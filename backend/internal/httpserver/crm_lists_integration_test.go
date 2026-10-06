package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-371 (DT-BE-4): CRM list endpoints (customers, leads, vehicles) follow
// docs/list-contract.md: whitelisted single-field sort with an id tiebreak,
// CSV filters, date ranges, organization filters inside the caller's scope;
// the customer and lead exports carry the same filters and sort; leads get
// a scoped, undoable bulk endpoint.

func crmIntegration(t *testing.T) *itest {
	t.Helper()
	store := storage.NewMemory()
	mr := miniredis.RunT(t)
	qc := queue.NewClient(config.RedisConfig{Addr: mr.Addr()})
	t.Cleanup(func() { _ = qc.Close() })
	return newIntegrationWithDeps(t,
		func(c *config.Config) { c.Encryption.CustomerPIIKey = itCustomerPIIKey },
		func(d *Deps) { d.Storage = store; d.Queue = qc })
}

// exportJobQuery reads the stored query of an export job.
func (it *itest) exportJobQuery(jobUUID string) ioengine.ExportQuery {
	it.t.Helper()
	var raw []byte
	if err := it.pool.QueryRow(context.Background(), "SELECT query_json FROM export_jobs WHERE uuid = $1", jobUUID).Scan(&raw); err != nil {
		it.t.Fatal(err)
	}
	var q ioengine.ExportQuery
	if err := json.Unmarshal(raw, &q); err != nil {
		it.t.Fatal(err)
	}
	return q
}

func TestIntegrationCRMListsCustomers(t *testing.T) {
	it := crmIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t371-dist", "distributor", center)
	dealerA := it.org("t371-a", "dealer", dist)
	dealerB := it.org("t371-b", "dealer", dist)
	var phones []string
	it.cleanupCustomers(&phones, dealerA, dealerB, dist)
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM export_jobs WHERE organization_id = ANY($1)`,
			[]int64{dist.ID, dealerA.ID, dealerB.ID})
	})
	ownerA, pwA := it.user("t371-owner-a")
	it.member(dealerA, ownerA, "owner")
	ownerB, pwB := it.user("t371-owner-b")
	it.member(dealerB, ownerB, "owner")
	distOwner, pwD := it.user("t371-dist-owner")
	it.member(dist, distOwner, "owner")
	tokA := it.loginOrg(ownerA, pwA, dealerA)
	tokB := it.loginOrg(ownerB, pwB, dealerB)
	tokD := it.loginOrg(distOwner, pwD, dist)

	tag := "crm" + it.suffix[len(it.suffix)-8:]
	create := func(tok, name string, extra map[string]any) string {
		ph := itPhone()
		phones = append(phones, ph)
		body := map[string]any{"phone": ph, "name": tag + "-" + name, "surname": "Liste"}
		for k, v := range extra {
			body[k] = v
		}
		return decodeData[custView](t, it.custDo("POST", "/v1/customers", tok, body, http.StatusCreated)).UUID
	}
	// c1 (A): individual, active, email z; c2 (A): corporate, pending, email a;
	// c3 (B): individual, disabled, no email, the only first service.
	c1 := create(tokA, "b", map[string]any{"email": tag + "-z@example.test"})
	c2 := create(tokA, "a", map[string]any{"email": tag + "-a@example.test", "type": "corporate", "company_name": "Firma " + tag})
	c3 := create(tokB, "c", nil)
	for i, row := range []struct {
		uuid, status, linked string
	}{{c1, "active", "2026-01-10T10:00:00Z"}, {c2, "pending", "2026-02-10T10:00:00Z"}, {c3, "disabled", "2026-03-10T10:00:00Z"}} {
		it.exec("UPDATE users SET status = $2 WHERE uuid = $1", row.uuid, row.status)
		it.exec(`UPDATE customer_organizations SET created_at = $2
			WHERE user_id = (SELECT id FROM users WHERE uuid = $1)`, row.uuid, row.linked)
		if i == 2 {
			it.exec(`UPDATE customer_organizations SET first_service_at = '2026-04-01T00:00:00Z'
				WHERE user_id = (SELECT id FROM users WHERE uuid = $1)`, row.uuid)
		}
	}

	c := listSortCase{it: it, admin: tokD, base: "/v1/customers", extra: url.Values{"q": {tag}}}
	c.expect(nil, c3, c2, c1) // default -linked_at
	c.expect(sortParam("linked_at"), c1, c2, c3)
	c.expect(sortParam("name"), c2, c1, c3)
	c.expect(sortParam("-name"), c3, c1, c2)
	c.expect(sortParam("email"), c2, c1, c3)  // empty e-mail last
	c.expect(sortParam("-email"), c1, c2, c3) // ... in both directions
	c.expect(sortParam("first_service_at"), c3, c1, c2)
	c.expect(sortParam("-first_service_at"), c3, c2, c1)
	c.expect(sortParam("status"), c1, c3, c2)
	c.expect(url.Values{"status": {"active,pending"}}, c2, c1)
	c.expect(url.Values{"status": {"disabled"}}, c3)
	c.expect(url.Values{"type": {"corporate"}}, c2)
	c.expect(url.Values{"type": {"individual"}}, c3, c1)
	c.expect(url.Values{"linked_from": {"2026-02-01"}, "linked_to": {"2026-02-28"}}, c2)
	c.expect(url.Values{"linked_to": {"2026-01-10"}}, c1) // a date covers the whole day
	c.expect(url.Values{"organization_uuid": {dealerB.Uuid.String()}}, c3)
	c.expect(url.Values{"organization_uuid": {dealerA.Uuid.String() + "," + dealerB.Uuid.String()}, "sort": {"name"}}, c2, c1, c3)
	c.expect400(sortParam("phone"), "sort")
	c.expect400(url.Values{"type": {"company"}}, "type")
	c.expect400(url.Values{"status": {"active,gone"}}, "status")
	c.expect400(url.Values{"organization_uuid": {"nope"}}, "organization_uuid")
	c.expect400(url.Values{"linked_from": {"2026-03-01"}, "linked_to": {"2026-02-01"}}, "linked_from")

	// The organization filter stays inside the scope: dealer A asking for
	// dealer B's customers gets nothing.
	a := listSortCase{it: it, admin: tokA, base: "/v1/customers", extra: url.Values{"q": {tag}}}
	a.expect(nil, c2, c1)
	a.expect(url.Values{"organization_uuid": {dealerB.Uuid.String()}})

	// Export: a bad list parameter is a 400 at request time; the job keeps
	// the filters and sort and the rows follow them.
	if code, env := it.do("POST", "/v1/customers/export", hostOlex, tokD, map[string]any{
		"format": "csv", "query": map[string]string{"sort": "phone"},
	}); code != http.StatusBadRequest {
		t.Fatalf("export with bad sort = %d %s", code, errCode(env))
	}
	job := decodeData[exportJob](t, it.custDo("POST", "/v1/customers/export", tokD, map[string]any{
		"format": "csv", "locale": "en", "query": map[string]string{"q": tag, "type": "individual", "sort": "-name"},
	}, http.StatusAccepted))
	stored := it.exportJobQuery(job.UUID)
	if stored["type"] != "individual" || stored["sort"] != "-name" || stored[customersusecase.QueryScope] == "" {
		t.Fatalf("stored export query = %v", stored)
	}
	stored[ioengine.QueryOrganizationID] = strconv.FormatInt(dist.ID, 10) // set by the worker
	adapter := customersusecase.NewListExportAdapter(customersusecase.New(it.pool, it.q, nil, nil))
	ds, err := adapter.Export(ctx, stored, i18n.Locale("en"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range ds.Rows {
		names = append(names, r["name"].(string))
	}
	if strings.Join(names, "|") != tag+"-c Liste|"+tag+"-b Liste" {
		t.Fatalf("export rows = %v", names)
	}
	_ = tokB
}

func TestIntegrationCRMListsVehicles(t *testing.T) {
	it := crmIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t371v-dist", "distributor", center)
	dealerA := it.org("t371v-a", "dealer", dist)
	dealerB := it.org("t371v-b", "dealer", dist)
	var phones []string
	it.cleanupCustomers(&phones, dealerA, dealerB, dist)
	ownerA, pwA := it.user("t371v-owner-a")
	it.member(dealerA, ownerA, "owner")
	ownerB, pwB := it.user("t371v-owner-b")
	it.member(dealerB, ownerB, "owner")
	distOwner, pwD := it.user("t371v-dist-owner")
	it.member(dist, distOwner, "owner")
	tokA := it.loginOrg(ownerA, pwA, dealerA)
	tokB := it.loginOrg(ownerB, pwB, dealerB)
	tokD := it.loginOrg(distOwner, pwD, dist)

	sfx := it.suffix[len(it.suffix)-8:]
	brandX, err := it.q.CreateCarBrand(ctx, db.CreateCarBrandParams{Name: "Alpha" + sfx, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	brandY, err := it.q.CreateCarBrand(ctx, db.CreateCarBrandParams{Name: "Beta" + sfx, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	modelX, err := it.q.CreateCarModel(ctx, db.CreateCarModelParams{CarBrandID: brandX.ID, Name: "Zeta", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	modelY, err := it.q.CreateCarModel(ctx, db.CreateCarModelParams{CarBrandID: brandY.ID, Name: "Eta", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = it.pool.Exec(ctx, "DELETE FROM vehicles WHERE car_brand_id = ANY($1)", []int64{brandX.ID, brandY.ID})
		_, _ = it.pool.Exec(ctx, "DELETE FROM car_models WHERE id = ANY($1)", []int64{modelX.ID, modelY.ID})
		_, _ = it.pool.Exec(ctx, "DELETE FROM car_brands WHERE id = ANY($1)", []int64{brandX.ID, brandY.ID})
	})

	customer := func(tok string) string {
		ph := itPhone()
		phones = append(phones, ph)
		return decodeData[custView](t, it.custDo("POST", "/v1/customers", tok, map[string]any{
			"phone": ph, "name": "Arac", "surname": sfx,
		}, http.StatusCreated)).UUID
	}
	cA, cB := customer(tokA), customer(tokB)
	vehicle := func(tok string, body map[string]any) string {
		return decodeData[vehView](t, it.custDo("POST", "/v1/vehicles", tok, body, http.StatusCreated)).UUID
	}
	v1 := vehicle(tokA, map[string]any{"customer_uuid": cA, "plate": "34 ABC 100",
		"car_brand_uuid": brandX.Uuid.String(), "car_model_uuid": modelX.Uuid.String(), "model_year": 2019})
	v2 := vehicle(tokA, map[string]any{"customer_uuid": cA, "plate": "34 ABC 050",
		"car_brand_uuid": brandY.Uuid.String(), "car_model_uuid": modelY.Uuid.String(), "model_year": 2022})
	v3 := vehicle(tokB, map[string]any{"customer_uuid": cB, "plate": "06 XYZ 200"})

	orgs := dealerA.Uuid.String() + "," + dealerB.Uuid.String()
	c := listSortCase{it: it, admin: tokD, base: "/v1/vehicles", extra: url.Values{"organization_uuid": {orgs}}}
	c.expect(nil, v3, v2, v1) // default -created_at
	c.expect(sortParam("created_at"), v1, v2, v3)
	c.expect(sortParam("plate"), v3, v2, v1)
	c.expect(sortParam("-plate"), v1, v2, v3)
	c.expect(sortParam("brand"), v1, v2, v3)  // no brand last
	c.expect(sortParam("-brand"), v2, v1, v3) // ... in both directions
	c.expect(sortParam("model"), v2, v1, v3)
	c.expect(sortParam("model_year"), v1, v2, v3)
	c.expect(sortParam("-model_year"), v2, v1, v3)
	c.expect(url.Values{"car_brand_uuid": {brandX.Uuid.String()}}, v1)
	c.expect(url.Values{"car_brand_uuid": {brandX.Uuid.String() + "," + brandY.Uuid.String()}}, v2, v1)
	c.expect(url.Values{"car_model_uuid": {modelY.Uuid.String()}}, v2)
	c.expect(url.Values{"organization_uuid": {dealerB.Uuid.String()}}, v3)
	c.expect(url.Values{"q": {"beta" + sfx}}, v2)          // car brand name
	c.expect(url.Values{"q": {"Beta" + sfx + " Eta"}}, v2) // brand + model
	c.expect(url.Values{"q": {"34abc"}, "sort": {"plate"}}, v2, v1)
	c.expect400(sortParam("vin"), "sort")
	c.expect400(url.Values{"car_brand_uuid": {"x"}}, "car_brand_uuid")
	c.expect400(url.Values{"car_model_uuid": {brandX.Uuid.String() + ",x"}}, "car_model_uuid")

	// Organization filter inside the scope: dealer A sees none of dealer B's.
	a := listSortCase{it: it, admin: tokA, base: "/v1/vehicles", extra: url.Values{"organization_uuid": {dealerB.Uuid.String()}}}
	a.expect(nil)
}

func TestIntegrationCRMListsLeads(t *testing.T) {
	it := crmIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealerA := it.org("t371l-a", "dealer", center)
	dealerB := it.org("t371l-b", "dealer", center)
	ownerA, pwA := it.user("t371l-owner-a")
	it.member(dealerA, ownerA, "owner")
	staffA, _ := it.user("t371l-staff-a")
	it.member(dealerA, staffA, "staff")
	ownerB, _ := it.user("t371l-owner-b")
	it.member(dealerB, ownerB, "owner")
	social, pwS := it.user("t371l-social")
	it.member(center, social, "staff", rbac.RoleCenterSocial)
	tokA := it.loginOrg(ownerA, pwA, dealerA)
	tokS := it.loginOrg(social, pwS, center)
	t.Cleanup(func() {
		ctx := context.Background()
		ids := []int64{dealerA.ID, dealerB.ID}
		_, _ = it.pool.Exec(ctx, "DELETE FROM export_jobs WHERE organization_id = ANY($1)", ids)
		_, _ = it.pool.Exec(ctx, "DELETE FROM bulk_operations WHERE organization_id = ANY($1)", ids)
		_, _ = it.pool.Exec(ctx, "DELETE FROM lead_events WHERE organization_id = ANY($1)", ids)
		_, _ = it.pool.Exec(ctx, "DELETE FROM leads WHERE organization_id = ANY($1)", ids)
	})

	tag := "crml" + it.suffix[len(it.suffix)-8:]
	type spec struct {
		org                  db.Organization
		company, contact     string
		status, temp, source string
		follow, created      string
		assignee             int64
	}
	mk := func(s spec) string {
		p := db.CreateLeadParams{
			OrganizationID: s.org.ID, BrandID: s.org.BrandID, TargetType: "customer",
			Source: s.source, Temperature: s.temp, Status: s.status, Notes: "",
		}
		if s.company != "" {
			p.CandidateCompanyName = pgtype.Text{String: tag + "-" + s.company, Valid: true}
		}
		if s.contact != "" {
			p.CandidateContactName = pgtype.Text{String: tag + "-" + s.contact, Valid: true}
		}
		if s.status == "lost" {
			p.LostReason = pgtype.Text{String: "price", Valid: true}
		}
		if s.follow != "" {
			ts, _ := time.Parse(time.RFC3339, s.follow)
			p.FollowUpDate = pgtype.Timestamptz{Time: ts, Valid: true}
		}
		if s.assignee != 0 {
			p.AssigneeUserID = pgtype.Int8{Int64: s.assignee, Valid: true}
		}
		row, err := it.q.CreateLead(ctx, p)
		if err != nil {
			t.Fatalf("lead: %v", err)
		}
		it.exec("UPDATE leads SET created_at = $2 WHERE id = $1", row.ID, s.created)
		return row.Uuid.String()
	}
	l1 := mk(spec{org: dealerA, contact: "d", status: "new", temp: "hot", source: "walk_in",
		follow: "2026-05-02T09:00:00Z", created: "2026-01-01T10:00:00Z", assignee: ownerA.ID})
	l2 := mk(spec{org: dealerA, company: "a", status: "quoted", temp: "cold", source: "website",
		created: "2026-01-02T10:00:00Z"})
	l3 := mk(spec{org: dealerA, contact: "c", status: "contacted", temp: "warm", source: "walk_in",
		follow: "2026-05-01T09:00:00Z", created: "2026-01-03T10:00:00Z"})
	l4 := mk(spec{org: dealerB, contact: "b", status: "lost", temp: "warm", source: "referral",
		created: "2026-01-04T10:00:00Z", assignee: ownerB.ID})

	c := listSortCase{it: it, admin: tokS, base: "/v1/leads", extra: url.Values{"q": {tag}}}
	c.expect(nil, l4, l3, l2, l1) // default -created_at
	c.expect(sortParam("created_at"), l1, l2, l3, l4)
	c.expect(sortParam("name"), l2, l4, l3, l1) // company or contact name
	c.expect(sortParam("status"), l1, l3, l2, l4)
	c.expect(sortParam("-status"), l4, l2, l3, l1)
	c.expect(sortParam("temperature"), l2, l3, l4, l1)
	c.expect(sortParam("-temperature"), l1, l4, l3, l2)
	c.expect(sortParam("follow_up_date"), l3, l1, l2, l4)  // empty last
	c.expect(sortParam("-follow_up_date"), l1, l3, l4, l2) // ... in both directions
	c.expect(url.Values{"status": {"new,contacted"}}, l3, l1)
	c.expect(url.Values{"source": {"walk_in,referral"}}, l4, l3, l1)
	c.expect(url.Values{"temperature": {"warm"}}, l4, l3)
	c.expect(url.Values{"target_type": {"dealer_candidate"}})
	c.expect(url.Values{"assignee_user_id": {strconv.FormatInt(ownerA.ID, 10)}}, l1)
	c.expect(url.Values{"assignee_user_id": {"none"}}, l3, l2)
	c.expect(url.Values{"assignee_user_id": {strconv.FormatInt(ownerA.ID, 10) + ",none"}}, l3, l2, l1)
	c.expect(url.Values{"created_from": {"2026-01-02"}, "created_to": {"2026-01-03"}}, l3, l2)
	c.expect400(sortParam("source"), "sort")
	c.expect400(url.Values{"status": {"new,open"}}, "status")
	c.expect400(url.Values{"temperature": {"boiling"}}, "temperature")
	c.expect400(url.Values{"source": {"fax"}}, "source")
	c.expect400(url.Values{"assignee_user_id": {"abc"}}, "assignee_user_id")
	a := listSortCase{it: it, admin: tokA, base: "/v1/leads", extra: url.Values{"q": {tag}}}
	a.expect(nil, l3, l2, l1) // managed scope: dealer A only

	// --- Export -------------------------------------------------------------
	if code, env := it.do("POST", "/v1/leads/export", hostOlex, tokA, map[string]any{
		"format": "csv", "query": map[string]string{"sort": "source"},
	}); code != http.StatusBadRequest {
		t.Fatalf("export with bad sort = %d %s", code, errCode(env))
	}
	if code, _ := it.do("POST", "/v1/leads/export", hostOlex, tokA, map[string]any{"format": "docx"}); code != http.StatusBadRequest {
		t.Fatalf("export with bad format = %d", code)
	}
	job := decodeData[exportJob](t, it.custDo("POST", "/v1/leads/export", tokA, map[string]any{
		"format": "csv", "locale": "en", "query": map[string]string{"q": tag, "status": "new,contacted", "sort": "name"},
	}, http.StatusAccepted))
	if job.Resource != leadsusecase.ResourceListExport {
		t.Fatalf("job = %+v", job)
	}
	stored := it.exportJobQuery(job.UUID)
	if stored[leadsusecase.QueryScope] != strconv.FormatInt(dealerA.ID, 10) || stored["sort"] != "name" {
		t.Fatalf("stored export query = %v", stored)
	}
	stored[ioengine.QueryOrganizationID] = strconv.FormatInt(dealerA.ID, 10) // set by the worker
	adapter := leadsusecase.NewListExportAdapter(leadsusecase.New(it.pool, it.q, nil))
	ds, err := adapter.Export(ctx, stored, i18n.Locale("en"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ds.Rows) != 2 || ds.Rows[0]["name"] != tag+"-c" || ds.Rows[1]["name"] != tag+"-d" ||
		ds.Rows[0]["status"] != "Contacted" || ds.Rows[1]["assignee"] == "" {
		t.Fatalf("export rows = %v", ds.Rows)
	}
	// The worker re-authorizes the stored scope: dealer B is outside a
	// dealer A job.
	bad := ioengine.ExportQuery{}
	for k, v := range stored {
		bad[k] = v
	}
	bad[leadsusecase.QueryScope] = strconv.FormatInt(dealerB.ID, 10)
	if _, err := adapter.Export(ctx, bad, i18n.Locale("en")); err == nil {
		t.Fatal("dealer A job with dealer B scope must fail")
	}

	// --- Bulk ---------------------------------------------------------------
	status := func(id string) (string, *string, *int64) {
		var s string
		var reason *string
		var assignee *int64
		if err := it.pool.QueryRow(ctx, "SELECT status, lost_reason, assignee_user_id FROM leads WHERE uuid = $1", id).
			Scan(&s, &reason, &assignee); err != nil {
			t.Fatal(err)
		}
		return s, reason, assignee
	}
	bulk := func(tok, action string, ids []string, params, query map[string]string) bulkSyncView {
		target := map[string]any{"scope": "ids", "ids": ids, "params": params}
		if query != nil {
			target = map[string]any{"scope": "query", "query": query, "params": params}
		}
		return it.bulkRun("/v1/leads/bulk", tok, map[string]any{"action": action, "target": target})
	}
	for _, tc := range []struct {
		action string
		params map[string]string
	}{
		{"set_status", map[string]string{"status": "open"}},
		{"set_status", map[string]string{"status": "lost"}}, // lost_reason missing
		{"assign", map[string]string{"assignee_user_id": "abc"}},
		{"archive", nil},
	} {
		if code, env := it.do("POST", "/v1/leads/bulk", hostOlex, tokA, map[string]any{
			"action": tc.action, "target": map[string]any{"scope": "ids", "ids": []string{l3}, "params": tc.params},
		}); code != http.StatusBadRequest {
			t.Fatalf("bulk %s %v = %d %s, want 400", tc.action, tc.params, code, errCode(env))
		}
	}

	// set_status: the pipeline rule fails new → quoted per item, and a lead
	// outside the caller's scope is "not found" even when the client sends
	// its own scope key.
	code, env := it.do("POST", "/v1/leads/bulk", hostOlex, tokA, map[string]any{
		"action": "set_status",
		"target": map[string]any{"scope": "ids", "ids": []string{l3, l1, l4},
			"params": map[string]string{"status": "quoted"}, "query": map[string]string{"_scope": "brand"}},
	})
	if code != http.StatusOK {
		t.Fatalf("bulk set_status = %d %s", code, errCode(env))
	}
	var run bulkSyncView
	if err := json.Unmarshal(env.Data, &run); err != nil || run.Operation == nil {
		t.Fatalf("bulk result %s: %v", env.Data, err)
	}
	if run.Summary.Succeeded != 1 || run.Summary.Failed != 2 {
		t.Fatalf("set_status summary = %+v", run.Summary)
	}
	if s, _, _ := status(l3); s != "quoted" {
		t.Fatalf("l3 status = %s", s)
	}
	if s, _, _ := status(l4); s != "lost" {
		t.Fatalf("dealer B lead changed: %s", s)
	}
	it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, tokA, http.StatusOK)
	if s, _, _ := status(l3); s != "contacted" {
		t.Fatalf("l3 after undo = %s", s)
	}

	// lost with a reason; undo restores status and clears the reason.
	run = bulk(tokA, "set_status", []string{l2}, map[string]string{"status": "lost", "lost_reason": "budget"}, nil)
	if s, reason, _ := status(l2); run.Summary.Succeeded != 1 || s != "lost" || reason == nil || *reason != "budget" {
		t.Fatalf("lost: %+v %s %v", run.Summary, s, reason)
	}
	it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, tokA, http.StatusOK)
	if s, reason, _ := status(l2); s != "quoted" || reason != nil {
		t.Fatalf("l2 after undo = %s %v", s, reason)
	}

	// assign: a member of the dealer succeeds (timeline event), a user of
	// another organization fails per item; undo restores the assignee.
	run = bulk(tokA, "assign", []string{l2, l3}, map[string]string{"assignee_user_id": strconv.FormatInt(staffA.ID, 10)}, nil)
	if _, _, as := status(l3); run.Summary.Succeeded != 2 || as == nil || *as != staffA.ID {
		t.Fatalf("assign: %+v %v", run.Summary, as)
	}
	var events int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM lead_events e JOIN leads l ON l.id = e.lead_id
		WHERE l.uuid = ANY($1::uuid[]) AND e.event_type = 'assigned'`, []string{l2, l3}).Scan(&events); err != nil || events != 2 {
		t.Fatalf("assigned events = %d %v", events, err)
	}
	it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, tokA, http.StatusOK)
	if _, _, as := status(l3); as != nil {
		t.Fatalf("l3 assignee after undo = %v", *as)
	}
	run = bulk(tokA, "assign", []string{l2}, map[string]string{"assignee_user_id": strconv.FormatInt(ownerB.ID, 10)}, nil)
	if run.Summary.Failed != 1 {
		t.Fatalf("foreign assignee: %+v", run.Summary)
	}

	// "Select all matching": the brand wide caller resolves the list filters.
	run = bulk(tokS, "set_status", nil, map[string]string{"status": "contacted"}, map[string]string{"q": tag, "status": "new"})
	if run.Summary.Total != 1 || run.Summary.Succeeded != 1 {
		t.Fatalf("query run = %+v", run.Summary)
	}
	if s, _, _ := status(l1); s != "contacted" {
		t.Fatalf("l1 = %s", s)
	}
}
