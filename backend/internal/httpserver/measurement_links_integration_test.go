package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	measurementsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type serviceMeasurementsBody struct {
	Links []struct {
		Phase       string `json:"phase"`
		LinkSource  string `json:"link_source"`
		Confirmed   bool   `json:"confirmed"`
		ConfirmedBy *struct {
			UUID string `json:"uuid"`
		} `json:"confirmed_by"`
		Measurement struct {
			UUID string `json:"uuid"`
		} `json:"measurement"`
	} `json:"links"`
	Suggestions []struct {
		Phase       string `json:"phase"`
		Measurement struct {
			UUID string `json:"uuid"`
		} `json:"measurement"`
	} `json:"suggestions"`
	Candidates []struct {
		UUID string `json:"uuid"`
	} `json:"candidates"`
}

func (b serviceMeasurementsBody) link(phase string) (string, string, bool, bool) {
	for _, l := range b.Links {
		if l.Phase == phase {
			return l.Measurement.UUID, l.LinkSource, l.Confirmed, true
		}
	}
	return "", "", false, false
}

func (b serviceMeasurementsBody) mentions(id uuid.UUID) bool {
	s := id.String()
	for _, l := range b.Links {
		if l.Measurement.UUID == s {
			return true
		}
	}
	for _, x := range b.Suggestions {
		if x.Measurement.UUID == s {
			return true
		}
	}
	for _, c := range b.Candidates {
		if c.UUID == s {
			return true
		}
	}
	return false
}

// TEC-296 acceptance (F3-02d): VIN based before/after matching, dealer
// confirmation and manual selection.
func TestIntegrationServiceMeasurementMatching(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealerA := it.org("t296-a", "dealer", center)
	dealerB := it.org("t296-b", "dealer", center)
	t.Cleanup(func() {
		bg := context.Background()
		like := "T296-" + it.suffix[len(it.suffix)-10:] + "-%"
		_, _ = it.pool.Exec(bg, "DELETE FROM service_measurements WHERE service_id IN (SELECT id FROM services WHERE service_no LIKE $1)", like)
		_, _ = it.pool.Exec(bg, "DELETE FROM measurement_results WHERE organization_id IN ($1, $2) OR (organization_id = $3 AND vin = 'WVWZZZ1JZ3W296004')",
			dealerA.ID, dealerB.ID, center.ID)
	})
	ownerA, pwA := it.user("t296-owner-a")
	ownerB, pwB := it.user("t296-owner-b")
	it.member(dealerA, ownerA, "owner")
	it.member(dealerB, ownerB, "owner")
	tokA := it.loginOrg(ownerA, pwA, dealerA)
	tokB := it.loginOrg(ownerB, pwB, dealerB)

	const vin = "WVWZZZ1JZ3W296001"
	now := time.Now()
	seq := 0
	service := func(org db.Organization, vin string, hasMeasurement bool, status string, start time.Time) (db.Service, db.User) {
		t.Helper()
		seq++
		cust, veh := it.svcCustomer(org, fmt.Sprintf("t296-cust-%d", seq), fmt.Sprintf("34T296%d", seq))
		svc, err := it.q.CreateService(ctx, db.CreateServiceParams{
			ServiceNo:      fmt.Sprintf("T296-%s-%d", it.suffix[len(it.suffix)-10:], seq),
			OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: cust.ID, VehicleID: veh.ID,
			CarBrandID: veh.CarBrandID.Int64, CarModelID: veh.CarModelID.Int64, Status: "draft",
			Vin: pgtype.Text{String: vin, Valid: vin != ""}, HasMeasurement: hasMeasurement,
		})
		if err != nil {
			t.Fatalf("service: %v", err)
		}
		if _, err := it.pool.Exec(ctx, `UPDATE services SET created_at = $2, status = $3 WHERE id = $1`, svc.ID, start, status); err != nil {
			t.Fatalf("service status: %v", err)
		}
		svc.Status = status
		return svc, cust
	}
	measurement := func(org db.Organization, by db.User, vin string, at time.Time) db.MeasurementResult {
		t.Helper()
		status := "accepted"
		v := pgtype.Text{String: vin, Valid: vin != ""}
		if vin == "" {
			status = "vin_pending"
		}
		var row db.MeasurementResult
		if err := it.pool.QueryRow(ctx, `
			INSERT INTO measurement_results (organization_id, brand_id, vin, status, raw, source, created_by, measured_at)
			VALUES ($1, $2, $3, $4, '{"raw":{}}'::jsonb, 'mobile', $5, $6)
			RETURNING id, uuid`, org.ID, org.BrandID, v, status, by.ID, at).Scan(&row.ID, &row.Uuid); err != nil {
			t.Fatalf("measurement: %v", err)
		}
		return row
	}
	linker := measurementsmodule.NewLinker(it.pool, it.q, nil)
	get := func(tok string, svc db.Service, want int) serviceMeasurementsBody {
		t.Helper()
		env := it.accDo("GET", "/v1/services/"+svc.Uuid.String()+"/measurements", tok, nil, want)
		if want != http.StatusOK {
			return serviceMeasurementsBody{}
		}
		return decodeData[serviceMeasurementsBody](t, env)
	}
	post := func(tok string, svc db.Service, m db.MeasurementResult, phase string, want int) envelope {
		t.Helper()
		return it.accDo("POST", "/v1/services/"+svc.Uuid.String()+"/measurements", tok,
			map[string]any{"measurement_uuid": m.Uuid.String(), "phase": phase}, want)
	}

	// 1. One measurement before and one after the start: both linked
	// automatically (auto, waiting for confirmation); another VIN and
	// another organization's measurement are never offered.
	svc1, cust1 := service(dealerA, vin, true, "processing", now.Add(-time.Hour))
	before := measurement(dealerA, ownerA, vin, now.Add(-3*time.Hour))
	after := measurement(dealerA, ownerA, vin, now.Add(-10*time.Minute))
	otherVIN := measurement(dealerA, ownerA, "WVWZZZ1JZ3W296999", now.Add(-2*time.Hour))
	otherOrg := measurement(dealerB, ownerB, vin, now.Add(-2*time.Hour))
	id1 := svc1.ID
	if err := linker.HandleServiceEvent(ctx, events.New(events.ServiceUpdated).WithEntity("service", &id1, &svc1.Uuid)); err != nil {
		t.Fatalf("match: %v", err)
	}
	b := get(tokA, svc1, http.StatusOK)
	if m, src, ok, found := b.link("before"); !found || m != before.Uuid.String() || src != "auto" || ok {
		t.Fatalf("before link = %s %s confirmed=%v found=%v", m, src, ok, found)
	}
	if m, src, ok, found := b.link("after"); !found || m != after.Uuid.String() || src != "auto" || ok {
		t.Fatalf("after link = %s %s confirmed=%v found=%v", m, src, ok, found)
	}
	if b.mentions(otherVIN.Uuid) || b.mentions(otherOrg.Uuid) {
		t.Fatalf("foreign measurement offered: %+v", b)
	}
	var events1 int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE event_name = 'measurement.match_suggested'
		AND payload->>'entity_uuid' = $1`, svc1.Uuid.String()).Scan(&events1); err != nil || events1 != 1 {
		t.Fatalf("match_suggested events = %d (%v)", events1, err)
	}
	// Another organization does not see the service; its measurement is 404.
	get(tokB, svc1, http.StatusNotFound)
	post(tokA, svc1, otherOrg, "before", http.StatusNotFound)

	// 2. Two candidates for the same phase: nothing linked, both suggested.
	svc2, _ := service(dealerA, "WVWZZZ1JZ3W296002", true, "draft", now)
	c1 := measurement(dealerA, ownerA, "WVWZZZ1JZ3W296002", now.Add(-48*time.Hour))
	c2 := measurement(dealerA, ownerA, "WVWZZZ1JZ3W296002", now.Add(-24*time.Hour))
	if _, err := linker.MatchService(ctx, svc2.ID); err != nil {
		t.Fatalf("match 2: %v", err)
	}
	b = get(tokA, svc2, http.StatusOK)
	if len(b.Links) != 0 || len(b.Suggestions) != 2 || b.Suggestions[0].Measurement.UUID != c2.Uuid.String() ||
		b.Suggestions[1].Measurement.UUID != c1.Uuid.String() || b.Suggestions[0].Phase != "before" {
		t.Fatalf("two candidates = %+v", b)
	}

	// 3. Confirmation fills confirmed_by.
	b = decodeData[serviceMeasurementsBody](t, post(tokA, svc1, before, "before", http.StatusOK))
	if _, _, ok, _ := b.link("before"); !ok {
		t.Fatalf("not confirmed: %+v", b)
	}
	var confirmedBy pgtype.Int8
	if err := it.pool.QueryRow(ctx, `SELECT confirmed_by FROM service_measurements WHERE service_id = $1 AND phase = 'before'`,
		svc1.ID).Scan(&confirmedBy); err != nil || confirmedBy.Int64 != ownerA.ID {
		t.Fatalf("confirmed_by = %+v (%v)", confirmedBy, err)
	}

	// 4. A new measurement does not change the confirmed link; a manual
	// second before is 409.
	newer := measurement(dealerA, ownerA, vin, now.Add(-90*time.Minute))
	if _, err := linker.MatchService(ctx, svc1.ID); err != nil {
		t.Fatalf("match 4: %v", err)
	}
	if m, _, _, _ := get(tokA, svc1, http.StatusOK).link("before"); m != before.Uuid.String() {
		t.Fatalf("confirmed link changed to %s", m)
	}
	if env := post(tokA, svc1, newer, "before", http.StatusConflict); errCode(env) != "MEASUREMENT_PHASE_TAKEN" {
		t.Fatalf("second before = %s", errCode(env))
	}

	// 5. The unconfirmed auto after link is replaced by a manual choice.
	later := measurement(dealerA, ownerA, vin, now.Add(-5*time.Minute))
	b = decodeData[serviceMeasurementsBody](t, post(tokA, svc1, later, "after", http.StatusOK))
	if m, src, ok, _ := b.link("after"); m != later.Uuid.String() || src != "manual" || !ok {
		t.Fatalf("manual after = %s %s %v", m, src, ok)
	}

	// 6. A vin_pending measurement cannot be linked (422); a service
	// without a measurement answer neither.
	pending := measurement(dealerA, ownerA, "", now)
	if env := post(tokA, svc2, pending, "before", http.StatusUnprocessableEntity); errCode(env) != "MEASUREMENT_VIN_PENDING" {
		t.Fatalf("vin pending = %s", errCode(env))
	}
	svcNo, _ := service(dealerA, "WVWZZZ1JZ3W296003", false, "processing", now.Add(-time.Hour))
	if env := post(tokA, svcNo, c1, "before", http.StatusUnprocessableEntity); errCode(env) != "MEASUREMENT_NOT_EXPECTED" {
		t.Fatalf("not expected = %s", errCode(env))
	}
	if env := post(tokA, svc2, c1, "middle", http.StatusBadRequest); errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("bad phase = %s", errCode(env))
	}

	// 7. After completion the dealer cannot remove a link (403); the
	// center removes the link of its own completed service.
	if _, err := it.pool.Exec(ctx, `UPDATE services SET status = 'completed', completed_at = NOW() WHERE id = $1`, svc1.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	it.accDo("DELETE", "/v1/services/"+svc1.Uuid.String()+"/measurements/after", tokA, nil, http.StatusForbidden)
	centerUser, centerPW := it.user("t296-center")
	it.member(center, centerUser, "owner")
	tokC := it.loginOrg(centerUser, centerPW, center)
	svcC, _ := service(center, "WVWZZZ1JZ3W296004", true, "processing", now.Add(-time.Hour))
	mc := measurement(center, centerUser, "WVWZZZ1JZ3W296004", now.Add(-2*time.Hour))
	post(tokC, svcC, mc, "before", http.StatusOK)
	if _, err := it.pool.Exec(ctx, `UPDATE services SET status = 'completed', completed_at = NOW() WHERE id = $1`, svcC.ID); err != nil {
		t.Fatalf("complete center: %v", err)
	}
	it.accDo("DELETE", "/v1/services/"+svcC.Uuid.String()+"/measurements/before", tokC, nil, http.StatusNoContent)
	it.accDo("DELETE", "/v1/services/"+svcC.Uuid.String()+"/measurements/before", tokC, nil, http.StatusNotFound)
	// Before completion the dealer removes a link; a missing link is 404.
	post(tokA, svc2, c2, "before", http.StatusOK)
	it.accDo("DELETE", "/v1/services/"+svc2.Uuid.String()+"/measurements/before", tokA, nil, http.StatusNoContent)
	it.accDo("DELETE", "/v1/services/"+svc2.Uuid.String()+"/measurements/before", tokA, nil, http.StatusNotFound)

	// 8. The portal service detail carries no measurement field even with
	// linked measurements.
	if err := it.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: cust1.ID, Slug: rbac.RoleCustomer}); err != nil {
		t.Fatalf("customer role: %v", err)
	}
	portalTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{UserID: cust1.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal})
	if err != nil {
		t.Fatal(err)
	}
	env := it.accDo("GET", "/v1/portal/services/"+svc1.Uuid.String(), portalTok, nil, http.StatusOK)
	var raw any
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	jsonKeys(raw, keys)
	for _, k := range append(portalForbiddenKeys, "links", "suggestions", "candidates", "phase", "link_source") {
		if keys[k] {
			t.Fatalf("portal detail carries %q: %s", k, env.Data)
		}
	}
}

// TEC-296: an accepted mobile upload computes the matching right away; the
// single after candidate of a processing service is linked automatically.
func TestIntegrationMeasurementUploadMatches(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t296-up", "dealer", center)
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = it.pool.Exec(bg, "DELETE FROM service_measurements WHERE organization_id = $1", dealer.ID)
		_, _ = it.pool.Exec(bg, "DELETE FROM measurement_results WHERE organization_id = $1", dealer.ID)
	})
	owner, pw := it.user("t296-up-owner")
	it.member(dealer, owner, "owner")
	cust, veh := it.svcCustomer(dealer, "t296-up-cust", "34T296U")
	const vin = "WVWZZZ1JZ3W296010"
	svc, err := it.q.CreateService(ctx, db.CreateServiceParams{
		ServiceNo:      fmt.Sprintf("T296U-%s", it.suffix[len(it.suffix)-10:]),
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, CustomerUserID: cust.ID, VehicleID: veh.ID,
		CarBrandID: veh.CarBrandID.Int64, CarModelID: veh.CarModelID.Int64, Status: "draft",
		Vin: pgtype.Text{String: vin, Valid: true}, HasMeasurement: true,
	})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE services SET created_at = NOW() - interval '1 hour', status = 'processing' WHERE id = $1`, svc.ID); err != nil {
		t.Fatal(err)
	}
	tp := it.mobileLogin(owner.Email.String, pw, dealer.Slug, "t296-dev")
	code, env, _ := it.doMobileKey("POST", "/v1/mobile/measurements", tp.AccessToken, "t296-"+it.suffix,
		map[string]any{"vin": vin, "raw": map[string]any{"values": []int{100}}})
	if code != http.StatusAccepted {
		t.Fatalf("upload = %d %s", code, errCode(env))
	}
	var up struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(env.Data, &up); err != nil {
		t.Fatal(err)
	}
	var phase, source string
	var confirmed pgtype.Timestamptz
	if err := it.pool.QueryRow(ctx, `SELECT sm.phase, sm.link_source, sm.confirmed_at FROM service_measurements sm
		JOIN measurement_results mr ON mr.id = sm.measurement_result_id
		WHERE sm.service_id = $1 AND mr.uuid = $2`, svc.ID, up.UUID).Scan(&phase, &source, &confirmed); err != nil {
		t.Fatalf("auto link: %v", err)
	}
	if phase != "after" || source != "auto" || confirmed.Valid {
		t.Fatalf("auto link = %s %s %v", phase, source, confirmed)
	}
}
