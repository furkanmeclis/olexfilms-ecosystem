package httpserver

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

// TEC-261 (F2-FIX-4): a migrated hub or warehouse order sits in processing
// (the legacy "sent to the warehouse" step). It is not stuck: the seller
// moves it to preparing and on through ready, shipped and received, and
// either side can cancel it, which releases its reservations like any other
// cancel before shipping.
func TestIntegrationOrdersProcessingWayOut(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	f := it.newReceiveFixture("t261", 2610)
	piece, fixed := f.piece, f.fixed
	staffTok, distTok := f.staffTok, f.distTok

	// migrated puts an approved order in processing the way the migrator
	// writes it: the status column only, no transition of this API.
	migrated := func(o orderView) {
		t.Helper()
		if _, err := it.pool.Exec(ctx, `UPDATE orders SET status = 'processing' WHERE uuid = $1`, o.UUID); err != nil {
			t.Fatalf("processing: %v", err)
		}
	}
	activeReservations := func(orderUUID string) (active, released int) {
		t.Helper()
		if err := it.pool.QueryRow(ctx, `
			SELECT COUNT(*) FILTER (WHERE r.status = 'active'), COUNT(*) FILTER (WHERE r.status = 'released')
			FROM stock_reservations r JOIN order_items i ON i.id = r.order_item_id JOIN orders o ON o.id = i.order_id
			WHERE o.uuid = $1`, orderUUID).Scan(&active, &released); err != nil {
			t.Fatal(err)
		}
		return active, released
	}

	// 1. processing -> preparing -> ready -> shipped -> received.
	a := it.orderCall("POST", "/v1/orders", distTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
	}}, http.StatusCreated)
	for _, step := range []struct{ tok, status string }{{distTok, "submitted"}, {staffTok, "approved"}} {
		if code, ec := it.transition(step.tok, a.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	migrated(a)
	if v := it.orderCall("GET", "/v1/orders/"+a.UUID, staffTok, nil, http.StatusOK); v.Status != "processing" ||
		!reflect.DeepEqual(v.AvailableTransitions, []string{"preparing", "cancelled"}) {
		t.Fatalf("seller on processing = %s %v", v.Status, v.AvailableTransitions)
	}
	if v := it.orderCall("GET", "/v1/orders/"+a.UUID, distTok, nil, http.StatusOK); !reflect.DeepEqual(v.AvailableTransitions, []string{"cancelled"}) {
		t.Fatalf("buyer on processing = %v", v.AvailableTransitions)
	}
	// Only the seller starts preparing; processing does not skip to ready
	// or shipped.
	if code, _ := it.transition(distTok, a.UUID, "preparing"); code != http.StatusForbidden {
		t.Fatalf("buyer preparing = %d", code)
	}
	for _, st := range []string{"ready", "shipped"} {
		if code, ec := it.transition(staffTok, a.UUID, st); code != http.StatusConflict || ec != "ORDER_INVALID_TRANSITION" {
			t.Fatalf("processing -> %s = %d %s", st, code, ec)
		}
	}
	if code, ec := it.transition(staffTok, a.UUID, "preparing"); code != http.StatusOK {
		t.Fatalf("processing -> preparing = %d %s", code, ec)
	}
	a = it.orderCall("GET", "/v1/orders/"+a.UUID, staffTok, nil, http.StatusOK)
	it.shipOrder(staffTok, a, [][]map[string]any{{{"barcode": f.u1.Barcode}}})
	if code, ec := it.transition(distTok, a.UUID, "received"); code != http.StatusOK {
		t.Fatalf("received = %d %s", code, ec)
	}
	if st, err := it.q.GetUnitCurrentState(ctx, f.u1.ID); err != nil || st.Status != "available" || st.HolderOrgID != f.dist.ID {
		t.Fatalf("u1 after receipt = %+v %v", st, err)
	}
	v := it.orderCall("GET", "/v1/orders/"+a.UUID, staffTok, nil, http.StatusOK)
	if v.Status != "received" {
		t.Fatalf("A status = %s", v.Status)
	}
	var fromProcessing bool
	for _, h := range v.History {
		if h.FromStatus != nil && *h.FromStatus == "processing" && h.ToStatus == "preparing" {
			fromProcessing = true
		}
	}
	if !fromProcessing {
		t.Fatalf("history has no processing -> preparing: %+v", v.History)
	}

	// 2. A processing order holding reserved units is cancelled: every
	// reservation is released and the units can be assigned again.
	b := it.openPreparing(distTok, staffTok, []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": fixed.Uuid.String(), "quantity": 5},
	})
	for i, body := range []map[string]any{{"barcode": f.u2.Barcode}, {"barcode": f.fu.Barcode, "quantity": 5}} {
		if code, ec, _ := it.assign(staffTok, b.UUID, b.Items[i].UUID, body); code != http.StatusOK {
			t.Fatalf("assign %v = %d %s", body, code, ec)
		}
	}
	migrated(b)
	if active, _ := activeReservations(b.UUID); active != 2 {
		t.Fatalf("B active reservations before cancel = %d", active)
	}
	code, env := it.do("POST", "/v1/orders/"+b.UUID+"/transitions", hostOlex, staffTok,
		map[string]string{"status": "cancelled", "reason": "legacy order dropped"})
	if code != http.StatusOK {
		t.Fatalf("processing -> cancelled = %d %s", code, errCode(env))
	}
	if active, released := activeReservations(b.UUID); active != 0 || released != 2 {
		t.Fatalf("B reservations after cancel active=%d released=%d", active, released)
	}
	if v := it.orderCall("GET", "/v1/orders/"+b.UUID, distTok, nil, http.StatusOK); v.Status != "cancelled" || len(v.AvailableTransitions) != 0 {
		t.Fatalf("B = %s %v", v.Status, v.AvailableTransitions)
	}
	var events int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = 'orders.cancelled' AND payload->'data'->>'order_uuid' = $1`,
		b.UUID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("orders.cancelled events = %d", events)
	}
	// The freed units (the serial one and all 5 fixed pieces) fit a new order.
	c := it.openPreparing(distTok, staffTok, []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": fixed.Uuid.String(), "quantity": 5},
	})
	for i, body := range []map[string]any{{"barcode": f.u2.Barcode}, {"barcode": f.fu.Barcode, "quantity": 5}} {
		if code, ec, _ := it.assign(staffTok, c.UUID, c.Items[i].UUID, body); code != http.StatusOK {
			t.Fatalf("reassign freed %v = %d %s", body, code, ec)
		}
	}

	// 3. The buyer may cancel a processing order too.
	d := it.orderCall("POST", "/v1/orders", distTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
	}}, http.StatusCreated)
	for _, step := range []struct{ tok, status string }{{distTok, "submitted"}, {staffTok, "approved"}} {
		if code, ec := it.transition(step.tok, d.UUID, step.status); code != http.StatusOK {
			t.Fatalf("D %s = %d %s", step.status, code, ec)
		}
	}
	migrated(d)
	if code, ec := it.transition(distTok, d.UUID, "cancelled"); code != http.StatusOK {
		t.Fatalf("buyer processing -> cancelled = %d %s", code, ec)
	}
}
