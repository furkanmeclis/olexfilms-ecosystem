package httpserver

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type warrantyRow struct {
	UUID       string  `json:"uuid"`
	PublicCode string  `json:"public_code"`
	Status     string  `json:"status"`
	VoidReason *string `json:"void_reason"`
	CanVoid    bool    `json:"can_void"`
	Product    struct {
		Name string `json:"name"`
	} `json:"product"`
	Organization struct {
		Name string `json:"name"`
	} `json:"organization"`
	Vehicle struct {
		Plate *string `json:"plate"`
	} `json:"vehicle"`
	Holder *struct {
		UUID string `json:"uuid"`
	} `json:"holder"`
}

type warrantyPage struct {
	Items []warrantyRow `json:"items"`
	Total int64         `json:"total"`
}

// listWarranties calls a warranty list and returns the public codes.
func (it *itest) listWarranties(path, token string) (warrantyPage, map[string]bool) {
	it.t.Helper()
	page := decodeData[warrantyPage](it.t, it.custDo("GET", path, token, nil, http.StatusOK))
	codes := map[string]bool{}
	for _, w := range page.Items {
		codes[w.PublicCode] = true
	}
	if int64(len(page.Items)) > page.Total {
		it.t.Fatalf("%s: %d items > total %d", path, len(page.Items), page.Total)
	}
	return page, codes
}

// warrantyFor opens one warranty through the service.completed listener
// for a service of org (seq makes the service number and unit unique).
func (it *itest) warrantyFor(org, center db.Organization, p db.Product, cust db.User, veh db.Vehicle, seq int) db.Warranty {
	it.t.Helper()
	u := it.stockChain().unit(center, p, 19100+seq)
	svc := it.directService(org, cust, veh, 191000+seq, db.CreateServiceItemParams{ProductID: p.ID, UnitID: u.ID, Kind: "full"})
	listener := warrantymodule.NewListener(it.pool, it.q, "http://localhost:3000", nil)
	if err := listener.HandleServiceCompleted(context.Background(), completedEvent(svc)); err != nil {
		it.t.Fatalf("listener: %v", err)
	}
	ws := it.serviceWarranties(svc.ID, svc.BrandID)
	if len(ws) != 1 {
		it.t.Fatalf("warranties = %d, want 1", len(ws))
	}
	return ws[0]
}

// TEC-191 acceptance: scope (dealer own, distributor subtree, center
// brand), portal holder only, filters, void with permission + step-up,
// event and audit.
func TestIntegrationWarrantyList(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist1 := it.org("t191-dist1", "distributor", center)
	dist2 := it.org("t191-dist2", "distributor", center)
	dealerA := it.org("t191-dealer-a", "dealer", dist1)
	dealerB := it.org("t191-dealer-b", "dealer", dist1)
	dealerC := it.org("t191-dealer-c", "dealer", dist2)

	p := it.product(center, "T191A")
	it.setWarrantyMonths(p, 12)
	p2 := it.product(center, "T191B")
	it.setWarrantyMonths(p2, 24)

	custA, vehA := it.svcCustomer(dealerA, "t191-cust-a", "34TEC191")
	custB, vehB := it.svcCustomer(dealerB, "t191-cust-b", "06TEC192")
	custC, vehC := it.svcCustomer(dealerC, "t191-cust-c", "35TEC193")
	wA := it.warrantyFor(dealerA, center, p, custA, vehA, 1)
	wA2 := it.warrantyFor(dealerA, center, p2, custA, vehA, 2)
	wB := it.warrantyFor(dealerB, center, p, custB, vehB, 3)
	wC := it.warrantyFor(dealerC, center, p, custC, vehC, 4)

	// wA ends in 10 days (expiring soon), wB already expired.
	if _, err := it.pool.Exec(ctx, `UPDATE warranties SET end_at = NOW() + INTERVAL '10 days' WHERE id = $1`, wA.ID); err != nil {
		t.Fatalf("shorten: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE warranties SET status = 'expired', expired_at = NOW(), end_at = start_at + INTERVAL '1 second' WHERE id = $1`, wB.ID); err != nil {
		t.Fatalf("expire: %v", err)
	}

	login := func(org db.Organization, name, role string) (db.User, string) {
		u, pw := it.user(name)
		it.member(org, u, role)
		return u, it.loginOrg(u, pw, org)
	}
	_, tokA := login(dealerA, "t191-a-owner", "owner")
	_, tokB := login(dealerB, "t191-b-owner", "owner")
	_, tokD1 := login(dist1, "t191-d1-owner", "owner")
	centerUser, tokCenter := login(center, "t191-center", "staff")

	ours := func(codes map[string]bool) []string {
		out := []string{}
		for _, w := range []db.Warranty{wA, wA2, wB, wC} {
			if codes[w.PublicCode] {
				out = append(out, w.PublicCode)
			}
		}
		return out
	}
	want := func(label string, codes map[string]bool, ws ...db.Warranty) {
		t.Helper()
		got := ours(codes)
		if len(got) != len(ws) {
			t.Fatalf("%s: got %v, want %d warranties", label, got, len(ws))
		}
		for _, w := range ws {
			if !codes[w.PublicCode] {
				t.Fatalf("%s: %s missing (got %v)", label, w.PublicCode, got)
			}
		}
	}

	// 1. Scope: dealer A only its own, dealer B only its own, distributor 1
	// its subtree (A + B, not C), center the brand.
	_, codes := it.listWarranties("/v1/warranties?limit=100", tokA)
	want("dealer A", codes, wA, wA2)
	_, codes = it.listWarranties("/v1/warranties?limit=100", tokB)
	want("dealer B", codes, wB)
	_, codes = it.listWarranties("/v1/warranties?limit=100", tokD1)
	want("distributor 1", codes, wA, wA2, wB)
	_, codes = it.listWarranties("/v1/warranties?limit=100&q="+url.QueryEscape("TEC19"), tokCenter)
	want("center", codes, wA, wA2, wB, wC)

	// Detail: out of scope = 404, in scope = 200 with the holder.
	it.custDo("GET", "/v1/warranties/"+wB.Uuid.String(), tokA, nil, http.StatusNotFound)
	it.custDo("GET", "/v1/warranties/"+wC.Uuid.String(), tokD1, nil, http.StatusNotFound)
	det := decodeData[warrantyRow](t, it.custDo("GET", "/v1/warranties/"+wA.Uuid.String(), tokA, nil, http.StatusOK))
	if det.Holder == nil || det.Holder.UUID != custA.Uuid.String() || det.CanVoid || det.Vehicle.Plate == nil {
		t.Fatalf("dealer detail = %+v", det)
	}
	// Another brand's domain does not see it.
	if code, _ := it.do("GET", "/v1/warranties/"+wA.Uuid.String(), hostGlorian, tokA, nil); code == http.StatusOK {
		t.Fatalf("glorian host read the olex warranty")
	}

	// 2. Filters (center, narrowed to the test rows by plate / product).
	_, codes = it.listWarranties("/v1/warranties?limit=100&status=expired&q=TEC19", tokCenter)
	want("status expired", codes, wB)
	_, codes = it.listWarranties("/v1/warranties?limit=100&status=active&days_left_max=30&q=TEC19", tokCenter)
	want("ends within 30 days", codes, wA)
	_, codes = it.listWarranties("/v1/warranties?limit=100&days_left_min=30&q=TEC19", tokCenter)
	want("more than 30 days left", codes, wA2, wC)
	_, codes = it.listWarranties("/v1/warranties?limit=100&q="+url.QueryEscape("34 tec-191"), tokCenter)
	want("plate search", codes, wA, wA2)
	_, codes = it.listWarranties("/v1/warranties?limit=100&q="+url.QueryEscape(wC.PublicCode), tokCenter)
	want("code search", codes, wC)
	_, codes = it.listWarranties("/v1/warranties?limit=100&product_uuid="+p2.Uuid.String(), tokCenter)
	want("product", codes, wA2)
	for _, bad := range []string{"status=open", "days_left_max=x", "days_left_min=40&days_left_max=10", "product_uuid=nope"} {
		it.custDo("GET", "/v1/warranties?"+bad, tokCenter, nil, http.StatusBadRequest)
	}
	page, _ := it.listWarranties("/v1/warranties?limit=1&q=TEC19", tokCenter)
	if len(page.Items) != 1 || page.Total != 4 {
		t.Fatalf("paging: %d items, total %d", len(page.Items), page.Total)
	}

	// 3. Portal: the holder sees only its own warranties; a panel token is
	// refused on the portal route and a portal token on the panel route.
	portalTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
		UserID: custA.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
	})
	if err != nil {
		t.Fatal(err)
	}
	pp, codes := it.listWarranties("/v1/portal/warranties?limit=100", portalTok)
	want("portal", codes, wA, wA2)
	if pp.Total != 2 {
		t.Fatalf("portal total = %d", pp.Total)
	}
	for _, w := range pp.Items {
		if w.Holder != nil || w.CanVoid {
			t.Fatalf("portal row carries holder / can_void: %+v", w)
		}
	}
	it.custDo("GET", "/v1/portal/warranties/"+wA.Uuid.String(), portalTok, nil, http.StatusOK)
	it.custDo("GET", "/v1/portal/warranties/"+wB.Uuid.String(), portalTok, nil, http.StatusNotFound)
	if code, env := it.do("GET", "/v1/portal/warranties", hostOlex, tokA, nil); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("panel token on portal list: %d %s", code, errCode(env))
	}
	if code, _ := it.do("GET", "/v1/warranties", hostOlex, portalTok, nil); code != http.StatusForbidden {
		t.Fatalf("portal token on panel list: %d", code)
	}
	_, codes = it.listWarranties("/v1/portal/warranties?limit=100&status=active&days_left_max=30", portalTok)
	want("portal filter", codes, wA)

	// 4. Void: a dealer / distributor lacks warranties.void, the center needs
	// a fresh step-up and a reason; the void writes the event and the audit.
	voidPath := "/v1/warranties/" + wB.Uuid.String() + "/void"
	body := map[string]any{"reason": "Hatalı uygulama kaydı"}
	it.custDo("POST", voidPath, tokB, body, http.StatusForbidden)
	it.custDo("POST", voidPath, tokD1, body, http.StatusForbidden)
	if code, env := it.do("POST", voidPath, hostOlex, tokCenter, body); code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("void without step-up: %d %s", code, errCode(env))
	}
	it.stepUp(centerUser.Uuid)
	it.custDo("POST", voidPath, tokCenter, map[string]any{"reason": " x "}, http.StatusBadRequest)
	voided := decodeData[warrantyRow](t, it.custDo("POST", voidPath, tokCenter, body, http.StatusOK))
	if voided.Status != "void" || voided.VoidReason == nil || *voided.VoidReason != "Hatalı uygulama kaydı" || voided.CanVoid {
		t.Fatalf("voided = %+v", voided)
	}
	it.custDo("POST", voidPath, tokCenter, body, http.StatusConflict)

	var events, audits int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = 'warranty.voided' AND payload->>'entity_uuid' = $1`,
		wB.Uuid.String()).Scan(&events); err != nil {
		t.Fatalf("events: %v", err)
	}
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_events WHERE action = 'warranty.voided' AND resource_uuid = $1 AND actor_user_id = $2`,
		wB.Uuid, centerUser.ID).Scan(&audits); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if events != 1 || audits != 1 {
		t.Fatalf("void wrote %d events, %d audit rows", events, audits)
	}
	var status string
	var expiredAt *time.Time
	if err := it.pool.QueryRow(ctx, `SELECT status, expired_at FROM warranties WHERE id = $1`, wB.ID).Scan(&status, &expiredAt); err != nil {
		t.Fatal(err)
	}
	if status != "void" || expiredAt != nil {
		t.Fatalf("db row: status %s expired_at %v", status, expiredAt)
	}
	_, codes = it.listWarranties("/v1/warranties?limit=100&status=void&q=TEC19", tokD1)
	want("void filter", codes, wB)

	// The center lists can_void on active rows.
	det = decodeData[warrantyRow](t, it.custDo("GET", "/v1/warranties/"+wC.Uuid.String(), tokCenter, nil, http.StatusOK))
	if !det.CanVoid || !strings.HasPrefix(det.Product.Name, "Film ") || det.Organization.Name != dealerC.Name {
		t.Fatalf("center detail = %+v", det)
	}
}
