package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type correctedItem struct {
	UUID       string `json:"uuid"`
	Barcode    string `json:"barcode"`
	Correction *struct {
		UUID               string  `json:"uuid"`
		Reason             string  `json:"reason"`
		ReplacementBarcode *string `json:"replacement_barcode"`
	} `json:"correction"`
}

// correct posts a consumption correction and returns the items of the
// service (with their corrections) and the error code.
func (it *itest) correct(token, svcUUID, itemUUID string, body map[string]any, want int) ([]correctedItem, string) {
	it.t.Helper()
	path := "/v1/services/" + svcUUID + "/items/" + itemUUID + "/consumption-correction"
	code, env := it.do("POST", path, hostOlex, token, body)
	if code != want {
		it.t.Fatalf("POST %s = %d %s, want %d", path, code, errCode(env), want)
	}
	var v struct {
		Items []correctedItem `json:"items"`
	}
	if code == http.StatusOK {
		if err := json.Unmarshal(env.Data, &v); err != nil {
			it.t.Fatal(err)
		}
	}
	return v.Items, errCode(env)
}

// svcItemsOf reads the items of a service.
func svcItemsOf(it *itest, svcUUID, token string) []correctedItem {
	it.t.Helper()
	code, env := it.do("GET", "/v1/services/"+svcUUID, hostOlex, token, nil)
	if code != http.StatusOK {
		it.t.Fatalf("GET service = %d %s", code, errCode(env))
	}
	var v struct {
		Items []correctedItem `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &v); err != nil {
		it.t.Fatal(err)
	}
	return v.Items
}

// TEC-230 acceptance: a center user corrects the consumption of a
// completed service. The wrong piece goes back into the dealer's stock
// with a ledger return movement and the correct piece is consumed instead;
// fixed pieces are credited back; the stock projections still equal the
// replayed ledger (rebuild check: no diff, no anomaly) and no accounting
// row is written. Refusals: the dealer (no services.cancel) 403, a partial
// cut 422, an item with an active warranty 422, a second correction 409,
// a replacement that is not held 409 / of another product 400, after the
// 24 hour window 422. A corrected item gets no warranty from the
// service.completed listener.
func TestIntegrationServiceConsumptionCorrection(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t230-dist", "distributor", center)
	dealer := it.org("t230-dealer", "dealer", dist)

	owner, opw := it.user("t230-owner")
	it.member(dealer, owner, "owner")
	staff, spw := it.user("t230-center")
	it.member(center, staff, "staff", rbac.RoleCenterStaff)
	cust, veh := it.svcCustomer(dealer, "t230-cust", "34T230"+it.suffix[len(it.suffix)-4:])

	piece := it.product(center, "T230P")
	roll := it.product(center, "T230R")
	fixed := it.product(center, "T230F")
	it.setWarrantyMonths(piece, 12)
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE products SET uses_fixed_barcode = TRUE WHERE id = $1`, fixed.ID); err != nil {
		t.Fatalf("fixed product: %v", err)
	}

	c := it.stockChain()
	cLoc := c.location(center, "C230")
	dLoc := c.location(dist, "D230")
	dealerOrg := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID}
	pa, pb, pc := c.unit(center, piece, 2301), c.unit(center, piece, 2302), c.unit(center, piece, 2303)
	ur := c.rollUnit(center, roll, 2304, "50.00")
	for _, u := range []db.Unit{pa, pb, pc, ur} {
		c.post(ledger.TypeEntry, u, c.nextRef(), cLoc)
		c.ship(u, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
		c.ship(u, ledger.TypeOrderOut, ledger.TypeReceived, dealer, dealerOrg)
	}
	uf := c.fixedUnit(center, fixed, 2305, cLoc, 10)
	ref := c.nextRef()
	c.postFixed(ledger.TypeOrderOut, uf, ref, &cLoc, nil, 4)
	c.postFixed(ledger.TypeReceived, uf, ref, nil, &dealerOrg, 4)

	oTok := it.loginOrg(owner, opw, dealer)
	sTok := it.loginOrg(staff, spw, center)
	s, _ := it.svcCall("POST", "/v1/services", oTok,
		map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}, http.StatusCreated)
	for _, item := range []map[string]any{
		{"barcode": pa.Barcode, "kind": "full"},
		{"barcode": pc.Barcode, "kind": "full"},
		{"barcode": uf.Barcode, "kind": "full", "quantity": 3},
		{"barcode": ur.Barcode, "kind": "partial", "meters": "10"},
	} {
		it.svcCall("POST", "/v1/services/"+s.UUID+"/items", oTok, item, http.StatusCreated)
	}
	done, _ := it.svcCall("POST", "/v1/services/"+s.UUID+"/transitions", oTok,
		map[string]string{"status": "completed"}, http.StatusOK)
	if done.Status != "completed" || len(done.Items) != 4 {
		t.Fatalf("completed = %+v", done)
	}
	itemOf := map[string]string{}
	for _, i := range done.Items {
		itemOf[i.Barcode] = i.UUID
	}
	var svcID int64
	if err := it.pool.QueryRow(ctx, `SELECT id FROM services WHERE uuid = $1`, s.UUID).Scan(&svcID); err != nil {
		t.Fatal(err)
	}
	financeRows := func() int {
		t.Helper()
		var n int
		if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM finance_entries
			WHERE organization_id = ANY($1::bigint[])`, []int64{center.ID, dist.ID, dealer.ID}).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	financeBefore := financeRows()
	state := func(u db.Unit) (status, ownerType string, ownerID int64, types string) {
		t.Helper()
		if err := it.pool.QueryRow(ctx, `SELECT u.status, s.owner_type, s.owner_id,
			(SELECT string_agg(m.type, ',' ORDER BY m.id) FROM stock_movements m WHERE m.unit_id = u.id)
			FROM units u JOIN unit_current_state s ON s.unit_id = u.id WHERE u.id = $1`, u.ID).
			Scan(&status, &ownerType, &ownerID, &types); err != nil {
			t.Fatal(err)
		}
		return
	}
	reason := map[string]any{"reason": "yanlış barkod okutuldu"}
	withReplacement := func(barcode string) map[string]any {
		return map[string]any{"reason": "yanlış barkod okutuldu", "replacement_barcode": barcode}
	}

	// 1. Refusals that write nothing.
	if _, ec := it.correct(oTok, s.UUID, itemOf[pa.Barcode], reason, http.StatusForbidden); ec == "" {
		t.Fatal("dealer correction: no error code")
	}
	if _, ec := it.correct(sTok, s.UUID, itemOf[ur.Barcode], reason, http.StatusUnprocessableEntity); ec != "SERVICE_CONSUMPTION_NOT_REVERSIBLE" {
		t.Fatalf("partial correction = %s", ec)
	}
	it.correct(sTok, s.UUID, itemOf[pa.Barcode], map[string]any{"reason": "  "}, http.StatusBadRequest)
	it.correct(sTok, s.UUID, itemOf[pa.Barcode], withReplacement(uf.Barcode), http.StatusBadRequest)
	if _, ec := it.correct(sTok, s.UUID, itemOf[pa.Barcode], withReplacement(pc.Barcode), http.StatusConflict); ec != "SERVICE_UNIT_NOT_AVAILABLE" {
		t.Fatalf("replacement in use = %s", ec)
	}
	if st, ot, oid, _ := state(pa); st != "used" || ot != "service" || oid != svcID {
		t.Fatalf("piece A after refusals = %s %s %d", st, ot, oid)
	}

	// 2. Piece A was the wrong unit: it comes back, piece B is consumed.
	items, _ := it.correct(sTok, s.UUID, itemOf[pa.Barcode], withReplacement(pb.Barcode), http.StatusOK)
	for _, i := range items {
		switch {
		case i.Barcode == pa.Barcode:
			if i.Correction == nil || i.Correction.ReplacementBarcode == nil || *i.Correction.ReplacementBarcode != pb.Barcode {
				t.Fatalf("corrected item = %+v", i)
			}
		case i.Correction != nil:
			t.Fatalf("item %s has a correction: %+v", i.Barcode, i.Correction)
		}
	}
	if st, ot, oid, types := state(pa); st != "available" || ot != "organization" || oid != dealer.ID ||
		types != "entry,transfer_out,transfer_in,order_out,received,consumption,return" {
		t.Fatalf("piece A = %s %s %d %s, want available at the dealer after a return", st, ot, oid, types)
	}
	if st, ot, oid, types := state(pb); st != "used" || ot != "service" || oid != svcID ||
		types != "entry,transfer_out,transfer_in,order_out,received,consumption" {
		t.Fatalf("piece B = %s %s %d %s, want used in the service", st, ot, oid, types)
	}
	var onHand int32
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(SUM(quantity), 0)::int FROM organization_product_stocks
		WHERE organization_id = $1 AND product_id = $2`, dealer.ID, piece.ID).Scan(&onHand); err != nil {
		t.Fatal(err)
	}
	if onHand != 1 {
		t.Fatalf("dealer piece stock = %d, want 1 (A back, B used)", onHand)
	}
	if _, ec := it.correct(sTok, s.UUID, itemOf[pa.Barcode], reason, http.StatusConflict); ec != "SERVICE_ITEM_ALREADY_CORRECTED" {
		t.Fatalf("second correction = %s", ec)
	}

	// 3. Warranty: the listener skips the corrected item; the other piece
	// gets a warranty and is refused while it is active.
	if err := it.srv.events.Publish(ctx, it.outboxEvent(events.ServiceCompleted, s.UUID)); err != nil {
		t.Fatalf("publish: %v", err)
	}
	ws := it.serviceWarranties(svcID, dealer.BrandID)
	if len(ws) != 1 || ws[0].UnitID != pc.ID || ws[0].Status != "active" {
		t.Fatalf("warranties = %+v, want one active on piece C", ws)
	}
	if _, ec := it.correct(sTok, s.UUID, itemOf[pc.Barcode], reason, http.StatusUnprocessableEntity); ec != "SERVICE_ITEM_WARRANTY_ACTIVE" {
		t.Fatalf("correction with an active warranty = %s", ec)
	}

	// 4. Fixed pieces: the 3 pieces are credited back to the dealer.
	it.correct(sTok, s.UUID, itemOf[uf.Barcode], reason, http.StatusOK)
	var fixedLeft int32
	if err := it.pool.QueryRow(ctx, `SELECT quantity_on_hand FROM fixed_barcode_holdings
		WHERE unit_id = $1 AND owner_type = 'organization' AND owner_id = $2`, uf.ID, dealer.ID).Scan(&fixedLeft); err != nil {
		t.Fatal(err)
	}
	if fixedLeft != 4 {
		t.Fatalf("fixed on hand = %d, want 4", fixedLeft)
	}

	// 5. Ledger replay: the projections equal the replayed ledger.
	rep, err := rebuild.New(it.pool, it.q).Check(ctx, dealer.ID)
	if err != nil {
		t.Fatalf("rebuild check: %v", err)
	}
	if rep.DiffCount != 0 || len(rep.Anomalies) != 0 || rep.UnitsScanned == 0 {
		t.Fatalf("rebuild check = %d diffs %v anomalies (%d units): %+v", rep.DiffCount, rep.Anomalies, rep.UnitsScanned, rep.Diffs)
	}

	// 6. No accounting row; the correction record is append-only.
	if got := financeRows(); got != financeBefore {
		t.Fatalf("finance entries %d -> %d", financeBefore, got)
	}
	var corrections int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM service_item_corrections WHERE service_id = $1`, svcID).Scan(&corrections); err != nil {
		t.Fatal(err)
	}
	if corrections != 2 {
		t.Fatalf("corrections = %d, want 2", corrections)
	}
	if _, err := it.pool.Exec(ctx, `UPDATE service_item_corrections SET reason = 'x' WHERE service_id = $1`, svcID); err == nil {
		t.Fatal("service_item_corrections accepted an update")
	}

	// 7. Time window: 25 hours after completion nothing can be corrected.
	if _, err := it.pool.Exec(ctx, `UPDATE services SET completed_at = NOW() - INTERVAL '25 hours' WHERE id = $1`, svcID); err != nil {
		t.Fatal(err)
	}
	if _, ec := it.correct(sTok, s.UUID, itemOf[pc.Barcode], reason, http.StatusUnprocessableEntity); ec != "SERVICE_CORRECTION_WINDOW_CLOSED" {
		t.Fatalf("correction after the window = %s", ec)
	}
}
