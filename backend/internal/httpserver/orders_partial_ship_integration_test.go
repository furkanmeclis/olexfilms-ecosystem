package httpserver

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// partialView is the TEC-1227 part of an order view.
type partialView struct {
	Status         string  `json:"status"`
	Total          *string `json:"total"`
	TryRate        *string `json:"try_rate"`
	IsBackorder    bool    `json:"is_backorder"`
	CanPartialShip bool    `json:"can_partial_ship"`
	BackorderOf    *struct {
		UUID string `json:"uuid"`
	} `json:"backorder_of"`
	Backorders []struct {
		UUID    string `json:"uuid"`
		OrderNo string `json:"order_no"`
	} `json:"backorders"`
	Items []struct {
		Quantity  *int32  `json:"quantity"`
		UnitPrice *string `json:"unit_price"`
		LineTotal *string `json:"line_total"`
		Product   struct {
			UUID string `json:"uuid"`
		} `json:"product"`
	} `json:"items"`
	OpenDiscrepancies int `json:"open_discrepancies"`
	Discrepancies     []struct {
		UUID         string   `json:"uuid"`
		Kind         string   `json:"kind"`
		Status       string   `json:"status"`
		Barcode      string   `json:"barcode"`
		Resolution   *string  `json:"resolution"`
		CanResolveAs []string `json:"can_resolve_as"`
	} `json:"discrepancies"`
	Timeline []struct {
		Action string `json:"action"`
	} `json:"timeline"`
}

func (it *itest) partialCall(method, path, tok string, body any, want int) (partialView, json.RawMessage) {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, tok, body)
	if code != want {
		it.t.Fatalf("%s %s = %d %s", method, path, code, errCode(env))
	}
	var v partialView
	if err := json.Unmarshal(env.Data, &v); err != nil {
		it.t.Fatal(err)
	}
	return v, env.Data
}

func ratSum(t *testing.T, vals ...*string) *big.Rat {
	t.Helper()
	sum := new(big.Rat)
	for _, v := range vals {
		if v == nil {
			t.Fatal("missing amount")
		}
		r, ok := new(big.Rat).SetString(*v)
		if !ok {
			t.Fatalf("amount %q", *v)
		}
		sum.Add(sum, r)
	}
	return sum
}

// TEC-1227 acceptance: with stock short, the center's warehouse staff ship
// the picked part of a preparing order; the rest becomes an approved
// backorder of the same buyer with the main order's prices and frozen rate
// (the totals add up), both orders link to each other and the warehouse
// role sees no price. The buyer receives with a missing and a damaged unit:
// the missing unit stays in transit, the seller sees the open record and
// is notified, and takes the unit back (returned). The backorder is
// cancelled like any order.
func TestIntegrationOrdersPartialShipAndDiscrepancies(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	f := it.newReceiveFixture("t1227", 12270)
	wh, wpw := it.user("t1227-center-warehouse")
	it.member(f.center, wh, "staff", rbac.RoleCenterWarehouse)
	whTok := it.loginOrg(wh, wpw, f.center)

	// Two pieces and three fixed ordered; one piece and two fixed in hand.
	o := it.openPreparing(f.distTok, f.staffTok, []any{
		map[string]any{"product_uuid": f.piece.Uuid.String(), "quantity": 2},
		map[string]any{"product_uuid": f.fixed.Uuid.String(), "quantity": 3},
	})
	path := "/v1/orders/" + o.UUID
	before, _ := it.partialCall("GET", path, f.staffTok, nil, http.StatusOK)
	if !before.CanPartialShip {
		t.Fatal("seller cannot ship partially")
	}

	// Nothing picked yet: refused.
	if code, env := it.do("POST", path+"/partial-ship", hostOlex, whTok, nil); code != http.StatusConflict ||
		errCode(env) != "ORDER_NOTHING_PICKED" {
		t.Fatalf("nothing picked = %d %s", code, errCode(env))
	}
	if code, ec, _ := it.assign(whTok, o.UUID, o.Items[0].UUID, map[string]any{"barcode": f.u1.Barcode}); code != http.StatusOK {
		t.Fatalf("assign u1 = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(whTok, o.UUID, o.Items[1].UUID, map[string]any{"barcode": f.fu.Barcode, "quantity": 2}); code != http.StatusOK {
		t.Fatalf("assign fixed = %d %s", code, ec)
	}
	// The buyer cannot split the seller's order.
	if code, env := it.do("POST", path+"/partial-ship", hostOlex, f.distTok, nil); code != http.StatusForbidden {
		t.Fatalf("buyer partial ship = %d %s", code, errCode(env))
	}
	// The default rule stays: ready needs every line picked.
	if code, ec := it.transition(whTok, o.UUID, "ready"); code != http.StatusConflict || ec != "ORDER_NOT_FULLY_ASSIGNED" {
		t.Fatalf("ready short = %d %s", code, ec)
	}

	main, raw := it.partialCall("POST", path+"/partial-ship", whTok, map[string]any{"reason": "stok eksik"}, http.StatusOK)
	if main.Status != "ready" || len(main.Backorders) != 1 || main.IsBackorder {
		t.Fatalf("main after split = %+v", main)
	}
	// The warehouse role sees no money.
	if main.Total != nil || main.TryRate != nil || strings.Contains(string(raw), `"unit_price":"`) ||
		strings.Contains(string(raw), `"line_total":"`) {
		t.Fatalf("prices leak to the warehouse role: %s", raw)
	}
	if *main.Items[0].Quantity != 1 || *main.Items[1].Quantity != 2 {
		t.Fatalf("main amounts = %d, %d", *main.Items[0].Quantity, *main.Items[1].Quantity)
	}
	backPath := "/v1/orders/" + main.Backorders[0].UUID
	back, raw := it.partialCall("GET", backPath, whTok, nil, http.StatusOK)
	if back.Status != "approved" || !back.IsBackorder || back.BackorderOf == nil || back.BackorderOf.UUID != o.UUID ||
		len(back.Items) != 2 || *back.Items[0].Quantity != 1 || *back.Items[1].Quantity != 1 || back.Total != nil ||
		strings.Contains(string(raw), `"unit_price":"`) {
		t.Fatalf("backorder = %+v %s", back, raw)
	}

	// The seller with prices: same unit prices and rate, totals add up.
	mainP, _ := it.partialCall("GET", path, f.staffTok, nil, http.StatusOK)
	backP, _ := it.partialCall("GET", backPath, f.staffTok, nil, http.StatusOK)
	if *mainP.TryRate != *backP.TryRate || *mainP.Items[0].UnitPrice != *backP.Items[0].UnitPrice {
		t.Fatalf("rate/price: %v/%v %v/%v", *mainP.TryRate, *backP.TryRate, *mainP.Items[0].UnitPrice, *backP.Items[0].UnitPrice)
	}
	if ratSum(t, mainP.Total, backP.Total).Cmp(ratSum(t, before.Total)) != 0 {
		t.Fatalf("totals %s + %s != %s", *mainP.Total, *backP.Total, *before.Total)
	}
	found := false
	for _, e := range mainP.Timeline {
		found = found || e.Action == "backorder_created"
	}
	if !found {
		t.Fatal("no backorder_created timeline step")
	}
	var warehouseNew int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = 'orders.warehouse_new'
		AND payload->'data'->>'order_uuid' = $1`, main.Backorders[0].UUID).Scan(&warehouseNew); err != nil {
		t.Fatal(err)
	}
	if warehouseNew != 1 {
		t.Fatalf("backorder warehouse_new events = %d", warehouseNew)
	}

	// Ship the picked part.
	if code, ec := it.transition(whTok, o.UUID, "shipped"); code != http.StatusOK {
		t.Fatalf("ship = %d %s", code, ec)
	}

	// The buyer receives: the piece is missing, the fixed barcode damaged.
	disc := []any{
		map[string]any{"unit_uuid": f.u1.Uuid.String(), "kind": "missing", "reason": "Kolide yok"},
		map[string]any{"unit_uuid": f.fu.Uuid.String(), "kind": "damaged", "reason": "Ezik"},
	}
	// Every unit missing is refused; discrepancies only go with received.
	allMissing := []any{
		map[string]any{"unit_uuid": f.u1.Uuid.String(), "kind": "missing", "reason": "Yok"},
		map[string]any{"unit_uuid": f.fu.Uuid.String(), "kind": "missing", "reason": "Yok"},
	}
	if code, env := it.do("POST", path+"/transitions", hostOlex, f.distTok, map[string]any{
		"status": "received", "discrepancies": allMissing,
	}); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("all missing = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", path+"/transitions", hostOlex, whTok, map[string]any{
		"status": "delivered", "discrepancies": disc,
	}); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("discrepancies on delivered = %d %s", code, errCode(env))
	}
	got, _ := it.partialCall("POST", path+"/transitions", f.distTok, map[string]any{
		"status": "received", "discrepancies": disc,
	}, http.StatusOK)
	if got.Status != "received" || got.OpenDiscrepancies != 2 || len(got.Discrepancies) != 2 {
		t.Fatalf("received = %+v", got)
	}
	if st, err := it.q.GetUnitCurrentState(ctx, f.u1.ID); err != nil || st.Status != "in_transit" {
		t.Fatalf("missing unit state = %+v %v", st, err)
	}
	var received int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM stock_movements WHERE unit_id = $1 AND type = 'received'`,
		f.fu.ID).Scan(&received); err != nil || received != 1 {
		t.Fatalf("damaged unit received movements = %d %v", received, err)
	}
	var notified int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE event_name = 'orders.receipt_discrepancy'
		AND payload->'data'->>'order_uuid' = $1 AND jsonb_array_length(payload->'data'->'notify_user_ids') > 0`,
		o.UUID).Scan(&notified); err != nil || notified != 1 {
		t.Fatalf("discrepancy events = %d %v", notified, err)
	}

	// The seller's warehouse sees the record (no price) and may take the
	// missing unit back; the buyer may mark it delivered.
	seller, raw := it.partialCall("GET", path, whTok, nil, http.StatusOK)
	if seller.OpenDiscrepancies != 2 || strings.Contains(string(raw), `"unit_price":"`) {
		t.Fatalf("seller record = %+v", seller)
	}
	var missing, damaged string
	for _, d := range seller.Discrepancies {
		switch d.Kind {
		case "missing":
			missing = d.UUID
			if strings.Join(d.CanResolveAs, ",") != "returned" || d.Barcode != f.u1.Barcode {
				t.Fatalf("seller missing actions = %+v", d)
			}
		case "damaged":
			damaged = d.UUID
		}
	}
	buyer, _ := it.partialCall("GET", path, f.distTok, nil, http.StatusOK)
	for _, d := range buyer.Discrepancies {
		if d.Kind == "missing" && strings.Join(d.CanResolveAs, ",") != "delivered" {
			t.Fatalf("buyer missing actions = %+v", d)
		}
	}
	if code, env := it.do("POST", path+"/discrepancies/"+missing+"/resolve", hostOlex, whTok,
		map[string]any{"resolution": "delivered"}); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("seller delivered = %d %s", code, errCode(env))
	}
	got, _ = it.partialCall("POST", path+"/discrepancies/"+missing+"/resolve", whTok,
		map[string]any{"resolution": "returned", "note": "Depoda bulundu"}, http.StatusOK)
	if got.OpenDiscrepancies != 1 {
		t.Fatalf("open after return = %d", got.OpenDiscrepancies)
	}
	if st, err := it.q.GetUnitCurrentState(ctx, f.u1.ID); err != nil || st.HolderOrgID != f.center.ID ||
		(st.Status != "placed" && st.Status != "available") {
		t.Fatalf("returned unit state = %+v %v", st, err)
	}
	if code, env := it.do("POST", path+"/discrepancies/"+missing+"/resolve", hostOlex, f.distTok,
		map[string]any{"resolution": "delivered"}); code != http.StatusConflict || errCode(env) != "ORDER_DISCREPANCY_CLOSED" {
		t.Fatalf("second resolve = %d %s", code, errCode(env))
	}
	got, _ = it.partialCall("POST", path+"/discrepancies/"+damaged+"/resolve", f.distTok,
		map[string]any{"resolution": "closed"}, http.StatusOK)
	if got.OpenDiscrepancies != 0 {
		t.Fatalf("open after close = %d", got.OpenDiscrepancies)
	}

	// The backorder is cancelled like any order.
	if code, ec := it.transition(whTok, main.Backorders[0].UUID, "preparing"); code != http.StatusOK {
		t.Fatalf("backorder preparing = %d %s", code, ec)
	}
	if code, ec := it.transition(f.staffTok, main.Backorders[0].UUID, "cancelled"); code != http.StatusOK {
		t.Fatalf("backorder cancel = %d %s", code, ec)
	}

	// A fully picked order is shipped whole, not split.
	o2 := it.openPreparing(f.distTok, f.staffTok, []any{map[string]any{"product_uuid": f.piece.Uuid.String(), "quantity": 1}})
	if code, ec, _ := it.assign(whTok, o2.UUID, o2.Items[0].UUID, map[string]any{"barcode": f.u2.Barcode}); code != http.StatusOK {
		t.Fatalf("assign u2 = %d %s", code, ec)
	}
	if code, env := it.do("POST", "/v1/orders/"+o2.UUID+"/partial-ship", hostOlex, whTok, nil); code != http.StatusConflict ||
		errCode(env) != "ORDER_FULLY_PICKED" {
		t.Fatalf("fully picked = %d %s", code, errCode(env))
	}
}
