package httpserver

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// shipOrder assigns the given barcodes (with quantities for fixed ones) to
// the lines of a preparing order and moves it to ready and shipped.
func (it *itest) shipOrder(sellerTok string, o orderView, assign [][]map[string]any) {
	it.t.Helper()
	for i, units := range assign {
		for _, body := range units {
			if code, ec, _ := it.assign(sellerTok, o.UUID, o.Items[i].UUID, body); code != http.StatusOK {
				it.t.Fatalf("assign %v = %d %s", body, code, ec)
			}
		}
	}
	for _, st := range []string{"ready", "shipped"} {
		if code, ec := it.transition(sellerTok, o.UUID, st); code != http.StatusOK {
			it.t.Fatalf("%s = %d %s", st, code, ec)
		}
	}
}

// TEC-168 acceptance: barcode ownership moves step by step along center ->
// distributor -> dealer. Each order is shipped (order_out) and received by
// the buyer (received): the unit is available at the buyer and the ledger
// history reads entry, order_out, received, order_out, received. A second
// receipt writes nothing; a received order cannot be cancelled. A cancel
// after shipping goes cancelling, then cancelled when the seller takes the
// goods back: order_cancel_restore returns every unit to the seller.
func TestIntegrationOrdersReceiveAndCancel(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	brand, err := it.q.GetBrandByID(ctx, center.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	cur := strings.TrimSpace(brand.Currency)
	if cur != "TRY" {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if err := it.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: today, Valid: true}, Base: cur, Quote: "TRY", Rate: "35", Source: "manual",
		}); err != nil {
			t.Fatalf("rate: %v", err)
		}
	}
	dist := it.org("t168-dist", "distributor", center)
	dealer := it.org("t168-dealer", "dealer", dist)
	staff, spw := it.user("t168-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t168-dist-owner")
	it.member(dist, distOwner, "owner")
	dealerOwner, rpw := it.user("t168-dealer-owner")
	it.member(dealer, dealerOwner, "owner")

	piece := it.product(center, "T168P")
	fixed := it.product(center, "T168F")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET uses_fixed_barcode = TRUE WHERE id = $1`, fixed.ID); err != nil {
		t.Fatalf("fixed product: %v", err)
	}
	fixed.UsesFixedBarcode = true
	it.setListPrice(piece, cur, "50")
	it.setListPrice(fixed, cur, "5")
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: piece.ID, BrandID: piece.BrandID, DistributorOrgID: dist.ID, Currency: cur, Price: "70",
	}); err != nil {
		t.Fatal(err)
	}

	chain := it.stockChain()
	loc := chain.location(center, "T168")
	u1 := chain.unit(center, piece, 1681)
	chain.post(ledger.TypeEntry, u1, chain.nextRef(), loc)
	u2 := chain.unit(center, piece, 1682)
	chain.post(ledger.TypeEntry, u2, chain.nextRef(), loc)
	fu := chain.fixedUnit(center, fixed, 1681, loc, 5)

	staffTok := it.loginOrg(staff, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)

	state := func(u db.Unit) db.UnitCurrentState {
		t.Helper()
		st, err := it.q.GetUnitCurrentState(ctx, u.ID)
		if err != nil {
			t.Fatalf("state %s: %v", u.Barcode, err)
		}
		return st
	}
	history := func(u db.Unit) string {
		t.Helper()
		rows, err := it.pool.Query(ctx, `SELECT type FROM stock_movements WHERE unit_id = $1 ORDER BY id`, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var types []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			types = append(types, s)
		}
		return strings.Join(types, ",")
	}
	countEvents := func(name, orderUUID string) int {
		t.Helper()
		var n int
		if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = $1 AND payload->'data'->>'order_uuid' = $2`,
			name, orderUUID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// 1. Center -> distributor: shipped, then received by the distributor.
	a := it.openPreparing(distTok, staffTok, []any{map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1}})
	it.shipOrder(staffTok, a, [][]map[string]any{{{"barcode": u1.Barcode}}})
	if v := it.orderCall("GET", "/v1/orders/"+a.UUID, distTok, nil, http.StatusOK); !strings.Contains(strings.Join(v.AvailableTransitions, ","), "received") {
		t.Fatalf("buyer transitions on shipped = %v", v.AvailableTransitions)
	}
	if code, _ := it.transition(staffTok, a.UUID, "received"); code != http.StatusForbidden {
		t.Fatalf("seller receive = %d", code)
	}
	if code, ec := it.transition(distTok, a.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive A = %d %s", code, ec)
	}
	if st := state(u1); st.Status != "available" || st.HolderOrgID != dist.ID || st.OwnerType != "organization" || st.OwnerID != dist.ID {
		t.Fatalf("u1 after A = %+v", st)
	}
	var keyed int
	if err := it.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM stock_movements m JOIN order_item_units oiu ON oiu.id = m.reference_id
		WHERE m.unit_id = $1 AND m.type = 'received' AND m.reference_type = 'order_item_unit'
		  AND m.idempotency_key = 'order:order_item_unit:' || oiu.id || ':received:' || $2`, u1.ID, u1.Barcode).Scan(&keyed); err != nil {
		t.Fatal(err)
	}
	if keyed != 1 {
		t.Fatalf("keyed received movements = %d", keyed)
	}
	// A second receipt is a no-op: no movement, no event.
	if code, ec := it.transition(distTok, a.UUID, "received"); code != http.StatusOK {
		t.Fatalf("second receive = %d %s", code, ec)
	}
	if h := history(u1); h != "entry,order_out,received" {
		t.Fatalf("u1 history after second receive = %s", h)
	}
	if n := countEvents("orders.received", a.UUID); n != 1 {
		t.Fatalf("orders.received events = %d", n)
	}
	// A received order cannot be cancelled (returns are a separate flow).
	for _, st := range []string{"cancelling", "cancelled"} {
		if code, ec := it.transition(distTok, a.UUID, st); code != http.StatusConflict || ec != "ORDER_INVALID_TRANSITION" {
			t.Fatalf("received -> %s = %d %s", st, code, ec)
		}
	}
	if v := it.orderCall("GET", "/v1/orders/"+a.UUID, distTok, nil, http.StatusOK); v.Status != "received" {
		t.Fatalf("A status = %s", v.Status)
	}

	// 2. Distributor -> dealer: the same unit is shipped and received again.
	b := it.openPreparing(dealerTok, distTok, []any{map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1}})
	it.shipOrder(distTok, b, [][]map[string]any{{{"barcode": u1.Barcode}}})
	if st := state(u1); st.Status != "in_transit" || st.HolderOrgID != dealer.ID {
		t.Fatalf("u1 shipped to dealer = %+v", st)
	}
	if code, ec := it.transition(dealerTok, b.UUID, "received"); code != http.StatusOK {
		t.Fatalf("receive B = %d %s", code, ec)
	}
	if st := state(u1); st.Status != "available" || st.HolderOrgID != dealer.ID || st.OwnerType != "organization" || st.OwnerID != dealer.ID {
		t.Fatalf("u1 after B = %+v", st)
	}
	if h := history(u1); h != "entry,order_out,received,order_out,received" {
		t.Fatalf("u1 history = %s", h)
	}

	// 3. Cancel after shipping: cancelling (the buyer asks), then cancelled
	// when the seller has the goods back; every unit returns to the seller.
	c := it.openPreparing(distTok, staffTok, []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": fixed.Uuid.String(), "quantity": 2},
	})
	it.shipOrder(staffTok, c, [][]map[string]any{{{"barcode": u2.Barcode}}, {{"barcode": fu.Barcode, "quantity": 2}}})
	holding := func() int32 {
		t.Helper()
		h, err := it.q.LockFixedBarcodeHolding(ctx, db.LockFixedBarcodeHoldingParams{UnitID: fu.ID, OwnerType: string(loc.Type), OwnerID: loc.ID})
		if err != nil {
			t.Fatal(err)
		}
		return h.QuantityOnHand
	}
	if q := holding(); q != 3 {
		t.Fatalf("fixed holding after ship = %d", q)
	}
	code, env := it.do("POST", "/v1/orders/"+c.UUID+"/transitions", hostOlex, distTok,
		map[string]string{"status": "cancelling", "reason": "wrong film"})
	if code != http.StatusOK {
		t.Fatalf("cancelling = %d %s", code, errCode(env))
	}
	if st := state(u2); st.Status != "in_transit" || st.HolderOrgID != dist.ID {
		t.Fatalf("u2 while cancelling = %+v", st)
	}
	// Only the seller closes the cancel; receiving is no longer possible.
	if code, _ := it.transition(distTok, c.UUID, "cancelled"); code != http.StatusForbidden {
		t.Fatalf("buyer cancelled = %d", code)
	}
	if code, ec := it.transition(distTok, c.UUID, "received"); code != http.StatusConflict || ec != "ORDER_INVALID_TRANSITION" {
		t.Fatalf("receive while cancelling = %d %s", code, ec)
	}
	if code, ec := it.transition(staffTok, c.UUID, "cancelled"); code != http.StatusOK {
		t.Fatalf("cancelled = %d %s", code, ec)
	}
	if st := state(u2); st.Status != "available" || st.HolderOrgID != center.ID || st.OwnerType != string(loc.Type) || st.OwnerID != loc.ID {
		t.Fatalf("u2 after cancel = %+v", st)
	}
	if q := holding(); q != 5 {
		t.Fatalf("fixed holding after cancel = %d", q)
	}
	if h := history(u2); h != "entry,order_out,order_cancel_restore" {
		t.Fatalf("u2 history = %s", h)
	}
	// A repeated cancel writes nothing.
	if code, ec := it.transition(staffTok, c.UUID, "cancelled"); code != http.StatusOK {
		t.Fatalf("second cancelled = %d %s", code, ec)
	}
	if h := history(u2); h != "entry,order_out,order_cancel_restore" {
		t.Fatalf("u2 history after second cancel = %s", h)
	}
	if q := holding(); q != 5 {
		t.Fatalf("fixed holding after second cancel = %d", q)
	}
	v := it.orderCall("GET", "/v1/orders/"+c.UUID, staffTok, nil, http.StatusOK)
	var steps []string
	for _, h := range v.History {
		steps = append(steps, h.ToStatus)
	}
	if v.Status != "cancelled" || !strings.HasSuffix(strings.Join(steps, ","), "shipped,cancelling,cancelled") {
		t.Fatalf("C = %s history %v", v.Status, steps)
	}
	if countEvents("orders.cancel_requested", c.UUID) != 1 || countEvents("orders.cancelled", c.UUID) != 1 {
		t.Fatalf("cancel events")
	}
	var reason string
	if err := it.pool.QueryRow(ctx, `SELECT COALESCE(cancel_reason, '') FROM orders WHERE uuid = $1`, c.UUID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "wrong film" {
		t.Fatalf("cancel reason = %q", reason)
	}
}
