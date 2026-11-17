package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// unitCreditsOf reads the credit projection of a unit (-1: no balance).
func (it *itest) unitCreditsOf(unitID int64) (total, remaining, used int32) {
	it.t.Helper()
	err := it.pool.QueryRow(context.Background(), `SELECT credits_total, credits_remaining, used_count
		FROM unit_credit_balances WHERE unit_id = $1`, unitID).Scan(&total, &remaining, &used)
	if err != nil {
		return -1, -1, -1
	}
	return total, remaining, used
}

// unitStateOf reads the unit status and its current owner.
func (it *itest) unitStateOf(unitID int64) (status, ownerType string, ownerID int64) {
	it.t.Helper()
	if err := it.pool.QueryRow(context.Background(), `SELECT u.status, s.owner_type, s.owner_id
		FROM units u JOIN unit_current_state s ON s.unit_id = u.id WHERE u.id = $1`, unitID).
		Scan(&status, &ownerType, &ownerID); err != nil {
		it.t.Fatal(err)
	}
	return status, ownerType, ownerID
}

// TEC-1221 acceptance (kredili kod): a unit of a product with the default
// 5 credits is used in 5 services and refused in the 6th
// (UNIT_CREDITS_EXHAUSTED); the unit stays in the dealer's stock until its
// last credit is used; every service gets its own warranty; cancelling a
// completed service gives the credit back (and the unit back to stock);
// "ürünü bitirdim" ends the credits, removing the item gives them back; the
// super admin increases the credits as a ledger movement with an activity
// row.
func TestIntegrationUnitCredits(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t1221-dist", "distributor", center)
	dealer := it.org("t1221-dealer", "dealer", dist)
	owner, opw := it.user("t1221-owner")
	it.member(dealer, owner, "owner")

	piece := it.product(center, "T1221P")
	if piece.UsageCredits != 5 {
		t.Fatalf("product default credits = %d, want 5", piece.UsageCredits)
	}
	it.setWarrantyMonths(piece, 12)
	c := it.stockChain()
	cLoc := c.location(center, "C1221")
	dLoc := c.location(dist, "D1221")
	dealerOrg := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID}
	u1 := c.unit(center, piece, 12210)
	u2 := c.unit(center, piece, 12211)
	for _, u := range []db.Unit{u1, u2} {
		c.post(ledger.TypeEntry, u, c.nextRef(), cLoc)
		c.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
		c.ship(u, ledger.TypeOrderOut, ledger.TypeReceived, dealer, dealerOrg)
	}
	tok := it.loginOrg(owner, opw, dealer)

	newSvc := func(i int) serviceView {
		cust, veh := it.svcCustomer(dealer, fmt.Sprintf("t1221-cust-%d", i),
			fmt.Sprintf("34K%d%s", i, it.suffix[len(it.suffix)-4:]))
		s, _ := it.svcCall("POST", "/v1/services", tok,
			map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}, http.StatusCreated)
		return s
	}
	svcID := func(uuid string) int64 {
		var id int64
		if err := it.pool.QueryRow(ctx, `SELECT id FROM services WHERE uuid = $1`, uuid).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	// 1. Five services take one credit each; the picker shows the credits.
	svcs := make([]serviceView, 0, 6)
	for i := 1; i <= 6; i++ {
		svcs = append(svcs, newSvc(i))
	}
	page := it.stockUnits(tok, svcs[0].UUID, url.Values{"barcode": {u1.Barcode}}, http.StatusOK)
	if len(page.Items) != 1 || page.Items[0].Credits == nil || page.Items[0].Credits.Remaining != 5 {
		t.Fatalf("picker before use = %+v", page.Items)
	}
	for i := 0; i < 5; i++ {
		it.svcCall("POST", "/v1/services/"+svcs[i].UUID+"/items", tok, map[string]any{"barcode": u1.Barcode}, http.StatusCreated)
		if total, rem, used := it.unitCreditsOf(u1.ID); total != 5 || rem != int32(4-i) || used != int32(i+1) {
			t.Fatalf("after service %d: credits %d/%d used %d", i+1, rem, total, used)
		}
	}
	// 2. The 6th service is refused; the picker hides the exhausted unit.
	if _, ec := it.svcCall("POST", "/v1/services/"+svcs[5].UUID+"/items", tok,
		map[string]any{"barcode": u1.Barcode}, http.StatusConflict); ec != "UNIT_CREDITS_EXHAUSTED" {
		t.Fatalf("6th service = %s", ec)
	}
	if p := it.stockUnits(tok, svcs[5].UUID, url.Values{"barcode": {u1.Barcode}}, http.StatusOK); len(p.Items) != 0 {
		t.Fatalf("picker lists the exhausted unit: %+v", p.Items)
	}
	// Twice in the same service stays SERVICE_UNIT_IN_USE.
	if _, ec := it.svcCall("POST", "/v1/services/"+svcs[0].UUID+"/items", tok,
		map[string]any{"barcode": u1.Barcode}, http.StatusConflict); ec != "SERVICE_UNIT_IN_USE" {
		t.Fatalf("same unit twice = %s", ec)
	}

	// 3. Completing services 1-4 leaves the unit in the dealer's stock; the
	// 5th (last credit, no other open service) consumes it.
	complete := func(s serviceView) {
		done, _ := it.svcCall("POST", "/v1/services/"+s.UUID+"/transitions", tok, map[string]string{"status": "completed"}, http.StatusOK)
		if done.Status != "completed" {
			t.Fatalf("complete = %+v", done)
		}
		if err := it.srv.events.Publish(ctx, it.outboxEvent(events.ServiceCompleted, s.UUID)); err != nil {
			t.Fatalf("publish service.completed: %v", err)
		}
	}
	for i := 0; i < 4; i++ {
		complete(svcs[i])
		if st, ot, oid := it.unitStateOf(u1.ID); st != "available" || ot != "organization" || oid != dealer.ID {
			t.Fatalf("after completing service %d: %s %s %d", i+1, st, ot, oid)
		}
	}
	complete(svcs[4])
	if st, ot, oid := it.unitStateOf(u1.ID); st != "used" || ot != "service" || oid != svcID(svcs[4].UUID) {
		t.Fatalf("after the last credit: %s %s %d", st, ot, oid)
	}
	// Every service has its own warranty.
	for i := 0; i < 5; i++ {
		if ws := it.serviceWarranties(svcID(svcs[i].UUID), dealer.BrandID); len(ws) != 1 || ws[0].Status != "active" {
			t.Fatalf("service %d warranties = %+v", i+1, ws)
		}
	}

	// 4. Cancelling completed service 2 gives its credit back and the unit
	// comes back into stock; the 6th service can use it now.
	it.cancelCompleted(tok, svcs[1].UUID, map[string]any{"reason": "yanlış kayıt"}, http.StatusOK)
	if total, rem, used := it.unitCreditsOf(u1.ID); total != 5 || rem != 1 || used != 4 {
		t.Fatalf("after cancel: credits %d/%d used %d", rem, total, used)
	}
	if st, ot, oid := it.unitStateOf(u1.ID); st != "available" || ot != "organization" || oid != dealer.ID {
		t.Fatalf("after cancel: %s %s %d", st, ot, oid)
	}
	it.svcCall("POST", "/v1/services/"+svcs[5].UUID+"/items", tok, map[string]any{"barcode": u1.Barcode}, http.StatusCreated)

	// 5. "Ürünü bitirdim": the item takes every remaining credit; removing
	// the item gives them back.
	s7 := newSvc(7)
	v, _ := it.svcCall("POST", "/v1/services/"+s7.UUID+"/items", tok,
		map[string]any{"barcode": u2.Barcode, "finish_product": true}, http.StatusCreated)
	if total, rem, _ := it.unitCreditsOf(u2.ID); total != 5 || rem != 0 {
		t.Fatalf("finished: credits %d/%d", rem, total)
	}
	s8 := newSvc(8)
	if _, ec := it.svcCall("POST", "/v1/services/"+s8.UUID+"/items", tok,
		map[string]any{"barcode": u2.Barcode}, http.StatusConflict); ec != "UNIT_CREDITS_EXHAUSTED" {
		t.Fatalf("after finish = %s", ec)
	}
	it.svcCall("DELETE", "/v1/services/"+s7.UUID+"/items/"+v.Items[0].UUID, tok, nil, http.StatusOK)
	if total, rem, used := it.unitCreditsOf(u2.ID); total != 5 || rem != 5 || used != 0 {
		t.Fatalf("after remove: credits %d/%d used %d", rem, total, used)
	}

	// 6. Unit detail: the credit card with the services; the super admin
	// increases the credits (others 403).
	code, env := it.do("GET", "/v1/stock/units/by-barcode/"+u1.Barcode, hostOlex, tok, nil)
	if code != http.StatusOK {
		t.Fatalf("unit history = %d %s", code, errCode(env))
	}
	var hist struct {
		Credits *struct {
			Total     int32 `json:"credits_total"`
			Remaining int32 `json:"credits_remaining"`
			Services  []struct {
				ServiceNo string `json:"service_no"`
			} `json:"services"`
		} `json:"credits"`
	}
	if err := json.Unmarshal(env.Data, &hist); err != nil {
		t.Fatal(err)
	}
	if hist.Credits == nil || hist.Credits.Total != 5 || len(hist.Credits.Services) != 6 {
		t.Fatalf("unit credits card = %+v", hist.Credits)
	}
	path := "/v1/platform/stock/units/" + u1.Uuid.String() + "/credits"
	if code, _ := it.do("POST", path, hostOlex, tok, map[string]any{"amount": 3, "reason": "x"}); code != http.StatusForbidden {
		t.Fatalf("dealer increase = %d", code)
	}
	admin, apw := it.user("t1221-admin", rbac.RoleSuperAdmin)
	atp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": apw,
	}))
	if code, _ := it.do("POST", path, hostOlex, atp.AccessToken, map[string]any{"amount": 0, "reason": "x"}); code != http.StatusBadRequest &&
		code != http.StatusUnprocessableEntity {
		t.Fatalf("zero increase = %d", code)
	}
	if code, env := it.do("POST", path, hostOlex, atp.AccessToken, map[string]any{"amount": 3, "reason": "müşteri talebi"}); code != http.StatusOK {
		t.Fatalf("increase = %d %s", code, errCode(env))
	}
	if total, rem, _ := it.unitCreditsOf(u1.ID); total != 8 || rem != 3 {
		t.Fatalf("after increase: credits %d/%d", rem, total)
	}
	var acts int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM activity_events WHERE action = 'stock.unit_credits_increased'
		AND resource_uuid = $1`, u1.Uuid).Scan(&acts); err != nil {
		t.Fatal(err)
	}
	if acts != 1 {
		t.Fatalf("increase activity rows = %d", acts)
	}
}

// TEC-1221 acceptance: a barcode shipped on a plotter cut line with 3
// credits opens with 3 credits; credits need a plotter cut line.
func TestIntegrationPlotterLineCredits(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	f := it.newReceiveFixture("t1221p", 12220)

	// Credits on a line that is not plotter cut are refused.
	code, env := it.do("POST", "/v1/orders", hostOlex, f.distTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": f.piece.Uuid.String(), "quantity": 1, "plotter_cut": false, "plotter_credits": 3},
	}})
	if code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
		t.Fatalf("credits without plotter = %d %s", code, errCode(env))
	}

	o := it.openPreparing(f.distTok, f.staffTok, []any{
		map[string]any{"product_uuid": f.piece.Uuid.String(), "quantity": 1, "plotter_cut": true, "plotter_credits": 3},
	})
	it.shipOrder(f.staffTok, o, [][]map[string]any{{{"barcode": f.u1.Barcode}}})
	var def int32
	if err := it.pool.QueryRow(ctx, `SELECT unit_credit_default($1)`, f.u1.ID).Scan(&def); err != nil {
		t.Fatal(err)
	}
	if def != 3 {
		t.Fatalf("plotter unit credits = %d, want 3", def)
	}
	if err := it.pool.QueryRow(ctx, `SELECT unit_credit_default($1)`, f.u2.ID).Scan(&def); err != nil {
		t.Fatal(err)
	}
	if def != 5 {
		t.Fatalf("other unit credits = %d, want the product default 5", def)
	}
	var credits *int32
	if err := it.pool.QueryRow(ctx, `SELECT plotter_credits FROM order_items WHERE uuid = $1`, o.Items[0].UUID).Scan(&credits); err != nil {
		t.Fatal(err)
	}
	if credits == nil || *credits != 3 {
		t.Fatalf("line credits = %v", credits)
	}
	// Turning the mark off clears the credits.
	if code, env := it.do("PATCH", "/v1/orders/"+o.UUID+"/items/"+o.Items[0].UUID, hostOlex, f.staffTok,
		map[string]any{"plotter_cut": false}); code != http.StatusOK {
		t.Fatalf("unmark = %d %s", code, errCode(env))
	}
	if err := it.pool.QueryRow(ctx, `SELECT plotter_credits FROM order_items WHERE uuid = $1`, o.Items[0].UUID).Scan(&credits); err != nil {
		t.Fatal(err)
	}
	if credits != nil {
		t.Fatalf("credits after unmark = %d", *credits)
	}
	if code, env := it.do("PATCH", "/v1/orders/"+o.UUID+"/items/"+o.Items[0].UUID, hostOlex, f.staffTok,
		map[string]any{"plotter_cut": true, "plotter_credits": 4}); code != http.StatusOK {
		t.Fatalf("mark with credits = %d %s", code, errCode(env))
	}
	if err := it.pool.QueryRow(ctx, `SELECT unit_credit_default($1)`, f.u1.ID).Scan(&def); err != nil || def != 4 {
		t.Fatalf("credits after PATCH = %d %v", def, err)
	}
}
