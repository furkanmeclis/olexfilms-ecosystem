package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// fixedUnit creates a fixed barcode of p and enters qty of it at the owner.
func (c *stockChain) fixedUnit(center db.Organization, p db.Product, n int, at ledger.Owner, qty int32) db.Unit {
	c.it.t.Helper()
	ctx := context.Background()
	u, err := c.it.q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: center.ID, BrandID: center.BrandID, ProductID: p.ID,
		Barcode:  fmt.Sprintf("T167F-%s-%d", c.it.suffix, n),
		UnitKind: ledger.KindFixed, Source: "generated", Status: string(ledger.StatusPrinted),
	})
	if err != nil {
		c.it.t.Fatalf("fixed unit: %v", err)
	}
	tx, err := c.it.pool.Begin(ctx)
	if err != nil {
		c.it.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := c.l.Post(ctx, tx, ledger.Movement{
		Type: ledger.TypeEntry, UnitID: u.ID, To: &at, Quantity: qty, Source: "test", RefType: "t167", RefID: c.nextRef(),
	}); err != nil {
		c.it.t.Fatalf("fixed entry: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		c.it.t.Fatal(err)
	}
	return u
}

func (it *itest) assign(access, orderUUID, itemUUID string, body map[string]any) (int, string, orderView) {
	it.t.Helper()
	code, env := it.do("POST", "/v1/orders/"+orderUUID+"/items/"+itemUUID+"/units", hostOlex, access, body)
	var v orderView
	if code == http.StatusOK {
		v = it.orderCall("GET", "/v1/orders/"+orderUUID, access, nil, http.StatusOK)
	}
	return code, errCode(env), v
}

// openPreparing creates an order of buyerTok with lines, approved by the
// seller and moved to preparing.
func (it *itest) openPreparing(buyerTok, sellerTok string, items []any) orderView {
	it.t.Helper()
	o := it.orderCall("POST", "/v1/orders", buyerTok, map[string]any{"items": items}, http.StatusCreated)
	for _, step := range []struct{ tok, status string }{
		{buyerTok, "submitted"}, {sellerTok, "approved"}, {sellerTok, "preparing"},
	} {
		if code, ec := it.transition(step.tok, o.UUID, step.status); code != http.StatusOK {
			it.t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	return it.orderCall("GET", "/v1/orders/"+o.UUID, sellerTok, nil, http.StatusOK)
}

// TEC-167 acceptance: center -> distributor order; barcodes are assigned
// (reservations), the order goes ready and shipped; the ledger has one
// order_out per unit and the serial unit is in transit owned by the buyer.
// A unit cannot be reserved by two orders, ready with a missing assignment
// answers 409, a second ship writes no movement and a fixed barcode cannot
// be reserved above what the seller holds.
func TestIntegrationOrdersFulfillment(t *testing.T) {
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
	dist := it.org("t167-dist", "distributor", center)
	otherDist := it.org("t167-dist2", "distributor", center)
	staff, spw := it.user("t167-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t167-dist-owner")
	it.member(dist, distOwner, "owner")
	otherOwner, opw := it.user("t167-dist2-owner")
	it.member(otherDist, otherOwner, "owner")

	piece := it.product(center, "T167P")
	fixed := it.product(center, "T167F")
	// units_check_integrity: a fixed barcode needs a uses_fixed_barcode product.
	if _, err := it.pool.Exec(ctx, `UPDATE products SET uses_fixed_barcode = TRUE WHERE id = $1`, fixed.ID); err != nil {
		t.Fatalf("fixed product: %v", err)
	}
	fixed.UsesFixedBarcode = true
	it.setListPrice(piece, cur, "50")
	it.setListPrice(fixed, cur, "5")

	chain := it.stockChain()
	loc := chain.location(center, "T167")
	var serial [3]db.Unit
	for i := range serial {
		serial[i] = chain.unit(center, piece, 1670+i)
		chain.post(ledger.TypeEntry, serial[i], chain.nextRef(), loc)
	}
	fu := chain.fixedUnit(center, fixed, 1, loc, 5)

	staffTok := it.loginOrg(staff, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	otherTok := it.loginOrg(otherOwner, opw, otherDist)

	a := it.openPreparing(distTok, staffTok, []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 2},
		map[string]any{"product_uuid": fixed.Uuid.String(), "quantity": 3},
	})
	b := it.openPreparing(otherTok, staffTok, []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": fixed.Uuid.String(), "quantity": 3},
	})
	aPiece, aFixed := a.Items[0].UUID, a.Items[1].UUID
	bPiece, bFixed := b.Items[0].UUID, b.Items[1].UUID

	// 1. Nothing assigned: ready answers 409.
	if code, ec := it.transition(staffTok, a.UUID, "ready"); code != http.StatusConflict || ec != "ORDER_NOT_FULLY_ASSIGNED" {
		t.Fatalf("ready without units = %d %s", code, ec)
	}
	// The buyer cannot assign.
	if code, _, _ := it.assign(distTok, a.UUID, aPiece, map[string]any{"barcode": serial[0].Barcode}); code != http.StatusForbidden {
		t.Fatalf("buyer assign = %d", code)
	}

	// 2. A serial unit is reserved by one order only.
	code, ec, v := it.assign(staffTok, a.UUID, aPiece, map[string]any{"barcode": serial[0].Barcode})
	if code != http.StatusOK || v.Items[0].Assigned != "1" || len(v.Items[0].Units) != 1 || v.Items[0].Units[0].Shipped {
		t.Fatalf("assign = %d %s %+v", code, ec, v.Items)
	}
	if code, ec, _ := it.assign(staffTok, b.UUID, bPiece, map[string]any{"barcode": serial[0].Barcode}); code != http.StatusConflict || ec != "ORDER_UNIT_RESERVED" {
		t.Fatalf("same unit on a second order = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(staffTok, a.UUID, aPiece, map[string]any{"barcode": serial[0].Barcode}); code != http.StatusConflict || ec != "ORDER_UNIT_ALREADY_ASSIGNED" {
		t.Fatalf("same unit twice on the line = %d %s", code, ec)
	}
	// A unit of another product does not fit the line.
	if code, _, _ := it.assign(staffTok, a.UUID, aFixed, map[string]any{"barcode": serial[1].Barcode}); code != http.StatusBadRequest {
		t.Fatalf("wrong product = %d", code)
	}

	// 3. Fixed barcode: 5 on hand, 3 reserved by A; B cannot reserve 3 more.
	if code, ec, _ := it.assign(staffTok, a.UUID, aFixed, map[string]any{"barcode": fu.Barcode, "quantity": 3}); code != http.StatusOK {
		t.Fatalf("fixed assign = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(staffTok, b.UUID, bFixed, map[string]any{"barcode": fu.Barcode, "quantity": 3}); code != http.StatusConflict || ec != "ORDER_INSUFFICIENT_STOCK" {
		t.Fatalf("fixed over-reservation = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(staffTok, b.UUID, bFixed, map[string]any{"barcode": fu.Barcode, "quantity": 2}); code != http.StatusOK {
		t.Fatalf("fixed within stock = %d %s", code, ec)
	}

	// 4. One of two pieces assigned: still 409; the line is not exceeded.
	if code, ec := it.transition(staffTok, a.UUID, "ready"); code != http.StatusConflict || ec != "ORDER_NOT_FULLY_ASSIGNED" {
		t.Fatalf("ready with a missing unit = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(staffTok, a.UUID, aPiece, map[string]any{"unit_uuid": serial[1].Uuid.String()}); code != http.StatusOK {
		t.Fatalf("assign by uuid = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(staffTok, a.UUID, aPiece, map[string]any{"barcode": serial[2].Barcode}); code != http.StatusConflict || ec != "ORDER_ITEM_OVER_ASSIGNED" {
		t.Fatalf("over-assign = %d %s", code, ec)
	}
	// Unassign releases the reservation; the unit can be assigned again.
	if code, env := it.do("DELETE", "/v1/orders/"+a.UUID+"/items/"+aPiece+"/units/"+serial[1].Uuid.String(), hostOlex, staffTok, nil); code != http.StatusOK {
		t.Fatalf("unassign = %d %s", code, errCode(env))
	}
	if code, ec, _ := it.assign(staffTok, b.UUID, bPiece, map[string]any{"barcode": serial[1].Barcode}); code != http.StatusOK {
		t.Fatalf("released unit on B = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(staffTok, a.UUID, aPiece, map[string]any{"barcode": serial[2].Barcode}); code != http.StatusOK {
		t.Fatalf("assign third unit = %d %s", code, ec)
	}

	// 5. Ready, then no more assignment changes.
	if code, ec := it.transition(staffTok, a.UUID, "ready"); code != http.StatusOK {
		t.Fatalf("ready = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(staffTok, a.UUID, aPiece, map[string]any{"barcode": serial[1].Barcode}); code != http.StatusConflict || ec != "ORDER_NOT_ASSIGNABLE" {
		t.Fatalf("assign on ready = %d %s", code, ec)
	}
	// The buyer cannot ship.
	if code, _ := it.transition(distTok, a.UUID, "shipped"); code != http.StatusForbidden {
		t.Fatalf("buyer ship = %d", code)
	}

	// 6. Ship: one order_out per assigned unit, serial units in transit
	// owned by the buyer, the fixed barcode debited at the seller.
	if code, ec := it.transition(staffTok, a.UUID, "shipped"); code != http.StatusOK {
		t.Fatalf("ship = %d %s", code, ec)
	}
	countOut := func() int {
		var n int
		if err := it.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM stock_movements m
			JOIN order_item_units oiu ON oiu.id = m.reference_id AND m.reference_type = 'order_item_unit'
			JOIN order_items i ON i.id = oiu.order_item_id
			JOIN orders o ON o.id = i.order_id
			WHERE o.uuid = $1 AND m.type = 'order_out'
			  AND m.idempotency_key = 'order:order_item_unit:' || oiu.id || ':order_out:' || (SELECT barcode FROM units WHERE id = m.unit_id)`,
			a.UUID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := countOut(); n != 3 {
		t.Fatalf("order_out movements = %d, want 3", n)
	}
	for _, u := range []db.Unit{serial[0], serial[2]} {
		st, err := it.q.GetUnitCurrentState(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if st.Status != "in_transit" || st.HolderOrgID != dist.ID || st.OwnerType != "organization" || st.OwnerID != dist.ID {
			t.Fatalf("unit %s state = %+v", u.Barcode, st)
		}
	}
	h, err := it.q.LockFixedBarcodeHolding(ctx, db.LockFixedBarcodeHoldingParams{UnitID: fu.ID, OwnerType: string(loc.Type), OwnerID: loc.ID})
	if err != nil || h.QuantityOnHand != 2 {
		t.Fatalf("fixed holding = %+v %v", h, err)
	}
	var active, consumed int
	if err := it.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE r.status = 'active'), COUNT(*) FILTER (WHERE r.status = 'consumed')
		FROM stock_reservations r JOIN order_items i ON i.id = r.order_item_id JOIN orders o ON o.id = i.order_id
		WHERE o.uuid = $1`, a.UUID).Scan(&active, &consumed); err != nil {
		t.Fatal(err)
	}
	if active != 0 || consumed != 3 {
		t.Fatalf("reservations active=%d consumed=%d", active, consumed)
	}
	s := it.orderCall("GET", "/v1/orders/"+a.UUID, distTok, nil, http.StatusOK)
	if s.Status != "shipped" || s.ShippedAt == nil || !s.Items[0].Units[0].Shipped {
		t.Fatalf("shipped order = %+v", s)
	}

	// 7. A second ship is a no-op: no new movement.
	if code, ec := it.transition(staffTok, a.UUID, "shipped"); code != http.StatusOK {
		t.Fatalf("second ship = %d %s", code, ec)
	}
	if n := countOut(); n != 3 {
		t.Fatalf("order_out after second ship = %d, want 3", n)
	}
	var shippedEvents int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = 'orders.shipped' AND payload->'data'->>'order_uuid' = $1`,
		a.UUID).Scan(&shippedEvents); err != nil {
		t.Fatal(err)
	}
	if shippedEvents != 1 {
		t.Fatalf("orders.shipped events = %d", shippedEvents)
	}

	// 8. A shipped unit is no longer in the seller's stock; cancelling B
	// releases its reservations.
	if code, ec, _ := it.assign(staffTok, b.UUID, bPiece, map[string]any{"barcode": serial[0].Barcode}); code != http.StatusConflict ||
		ec != "ORDER_UNIT_NOT_AVAILABLE" {
		t.Fatalf("shipped unit on B = %d %s", code, ec)
	}
	if code, ec := it.transition(otherTok, b.UUID, "cancelled"); code != http.StatusOK {
		t.Fatalf("cancel B = %d %s", code, ec)
	}
	if err := it.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM stock_reservations r JOIN order_items i ON i.id = r.order_item_id JOIN orders o ON o.id = i.order_id
		WHERE o.uuid = $1 AND r.status = 'active'`, b.UUID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("B active reservations after cancel = %d", active)
	}
}
