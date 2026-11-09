package httpserver

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/google/uuid"
)

type parentReturnView struct {
	UUID                 string   `json:"uuid"`
	Status               string   `json:"status"`
	Role                 string   `json:"role"`
	OpenedByParent       bool     `json:"opened_by_parent"`
	AvailableTransitions []string `json:"available_transitions"`
	Sender               struct {
		UUID string `json:"uuid"`
	} `json:"sender"`
	Receiver struct {
		UUID string `json:"uuid"`
	} `json:"receiver"`
	Order *struct {
		UUID    string `json:"uuid"`
		OrderNo string `json:"order_no"`
	} `json:"order"`
	Items []struct {
		Barcode   string  `json:"barcode"`
		UnitPrice *string `json:"unit_price"`
	} `json:"items"`
}

type itemsOf[T any] struct {
	Items []T `json:"items"`
}

// TEC-1085 acceptance, on the F1 chain (the distributor sold the two rolls
// to the dealer on order B; the consumption is undone so the dealer holds
// them again):
//   - the distributor (parent, transfers.approve) lists its children, the
//     dealer's orders from it and order B's units;
//   - it opens a return in the dealer's name linked to order B: the return
//     starts approved, priced at order B's line, and the distributor ships
//     and receives it in one step; the stock moves to the distributor and
//     the stock_return reversal is booked on both cari;
//   - the dealer sees the return, the order filter finds it, the accounting
//     queue shows the order number;
//   - the dealer links its own return to order B; an order of someone else
//     is refused, a non-child is refused, the dealer cannot open a return
//     for another organization;
//   - a parent return can also be shipped by the parent and received after.
func TestIntegrationParentReturnLinkedToOrder(t *testing.T) {
	c := runF1Chain(t)
	it := c.it
	ctx := context.Background()

	chain := it.stockChain()
	to := ledger.Owner{Type: ledger.OwnerOrganization, ID: c.dealer.ID, OrgID: c.dealer.ID}
	for _, u := range c.rolls {
		tx, err := it.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		from := ledger.Owner{Type: ledger.OwnerService, ID: c.svcID, OrgID: c.dealer.ID}
		if _, err := chain.l.Post(ctx, tx, ledger.Movement{
			Type: ledger.TypeReturn, UnitID: u.ID, From: &from, To: &to, Centimeters: 2500,
			Source: "test", RefType: "t1085", RefID: c.svcID, Reason: "TEC-1085 consumption undone",
		}); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("return %s: %v", u.Barcode, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	dealerUUID := c.dealer.Uuid.String()
	call := func(method, path, tok string, body any, want int) parentReturnView {
		t.Helper()
		return decodeData[parentReturnView](t, it.accDo(method, path, tok, body, want))
	}
	move := func(tok, id, status string) parentReturnView {
		t.Helper()
		return call("POST", "/v1/stock-transfers/"+id+"/transitions", tok, map[string]any{"status": status}, http.StatusOK)
	}
	returnRows := func(id string) int {
		t.Helper()
		var n int
		if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM finance_entries
			WHERE source_type = 'stock_return' AND source_uuid = $1 AND reversal_of_id IS NULL`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// 1. Pickers: children, orders, order units.
	sources := decodeData[itemsOf[struct {
		UUID string `json:"uuid"`
	}]](t, it.accDo("GET", "/v1/stock-transfers/return-sources", c.distTok, nil, http.StatusOK))
	if !slices.ContainsFunc(sources.Items, func(o struct {
		UUID string `json:"uuid"`
	}) bool {
		return o.UUID == dealerUUID
	}) {
		t.Fatalf("return sources = %+v", sources)
	}
	it.accDo("GET", "/v1/stock-transfers/return-sources", c.dealerTok, nil, http.StatusForbidden)
	orders := decodeData[itemsOf[struct {
		UUID    string `json:"uuid"`
		OrderNo string `json:"order_no"`
	}]](t, it.accDo("GET", "/v1/stock-transfers/return-orders?from_org_uuid="+dealerUUID, c.distTok, nil, http.StatusOK))
	orderNo := ""
	for _, o := range orders.Items {
		if o.UUID == c.b.UUID {
			orderNo = o.OrderNo
		}
	}
	if orderNo == "" {
		t.Fatalf("return orders = %+v", orders)
	}
	// The dealer's own picker lists the same order (its parent sold it).
	own := decodeData[itemsOf[struct {
		UUID string `json:"uuid"`
	}]](t, it.accDo("GET", "/v1/stock-transfers/return-orders", c.dealerTok, nil, http.StatusOK))
	if len(own.Items) == 0 {
		t.Fatalf("dealer return orders = %+v", own)
	}
	units := decodeData[itemsOf[struct {
		Barcode    string `json:"barcode"`
		Returnable int64  `json:"returnable"`
	}]](t, it.accDo("GET", "/v1/stock-transfers/return-orders/"+c.b.UUID+"/units?from_org_uuid="+dealerUUID,
		c.distTok, nil, http.StatusOK))
	for _, r := range c.rolls {
		found := false
		for _, u := range units.Items {
			if u.Barcode == r.Barcode && u.Returnable == 1 {
				found = true
			}
		}
		if !found {
			t.Fatalf("order units = %+v, want %s returnable", units, r.Barcode)
		}
	}

	// 2. Refusals.
	it.accDo("POST", "/v1/stock-transfers", c.dealerTok, map[string]any{
		"kind": "return", "from_org_uuid": c.dist.Uuid.String(),
		"items": []map[string]any{{"barcode": c.rolls[0].Barcode}},
	}, http.StatusForbidden)
	if ec := errCode(it.accDo("POST", "/v1/stock-transfers", c.distTok, map[string]any{
		"kind": "return", "from_org_uuid": c.center.Uuid.String(),
		"items": []map[string]any{{"barcode": c.rolls[0].Barcode}},
	}, http.StatusUnprocessableEntity)); ec != "TRANSFER_NOT_CHILD" {
		t.Fatalf("return for a non-child = %s", ec)
	}
	if ec := errCode(it.accDo("POST", "/v1/stock-transfers", c.dealerTok, map[string]any{
		"kind": "return", "order_uuid": uuid.NewString(),
		"items": []map[string]any{{"barcode": c.rolls[0].Barcode}},
	}, http.StatusBadRequest)); ec != "VALIDATION_ERROR" {
		t.Fatalf("return with an unknown order = %s", ec)
	}

	// 3. Parent return of roll 0 linked to order B, received in one step.
	r1 := call("POST", "/v1/stock-transfers", c.distTok, map[string]any{
		"kind": "return", "from_org_uuid": dealerUUID, "order_uuid": c.b.UUID,
		"items": []map[string]any{{"barcode": c.rolls[0].Barcode}},
	}, http.StatusCreated)
	if r1.Status != "approved" || !r1.OpenedByParent || r1.Role != "parent" ||
		r1.Sender.UUID != dealerUUID || r1.Receiver.UUID != c.dist.Uuid.String() ||
		r1.Order == nil || r1.Order.UUID != c.b.UUID || r1.Order.OrderNo != orderNo ||
		strings.Join(r1.AvailableTransitions, ",") != "shipped,received,cancelled" {
		t.Fatalf("parent return = %+v", r1)
	}
	if len(r1.Items) != 1 || r1.Items[0].UnitPrice == nil {
		t.Fatalf("parent return items = %+v", r1.Items)
	}
	var orderItem *int64
	if err := it.pool.QueryRow(ctx, `SELECT i.order_item_id FROM stock_transfer_request_items i
		JOIN stock_transfer_requests r ON r.id = i.request_id WHERE r.uuid = $1`, r1.UUID).Scan(&orderItem); err != nil || orderItem == nil {
		t.Fatalf("order line of the return = %v, %v", orderItem, err)
	}
	if v := call("GET", "/v1/stock-transfers/"+r1.UUID, c.dealerTok, nil, http.StatusOK); v.Role != "sender" ||
		strings.Join(v.AvailableTransitions, ",") != "shipped,cancelled" {
		t.Fatalf("dealer view = %+v", v)
	}
	if v := move(c.distTok, r1.UUID, "received"); v.Status != "received" {
		t.Fatalf("one-step receipt = %+v", v)
	}
	st, err := it.q.GetUnitCurrentState(ctx, c.rolls[0].ID)
	if err != nil || st.HolderOrgID != c.dist.ID || st.Status != "available" {
		t.Fatalf("returned roll = %+v, %v", st, err)
	}
	if n := returnRows(r1.UUID); n != 2 {
		t.Fatalf("stock_return rows = %d, want 2", n)
	}

	// 4. The order filter and the accounting queue.
	linked := decodeData[itemsOf[parentReturnView]](t, it.accDo("GET",
		"/v1/stock-transfers?kind=return&order_uuid="+c.b.UUID, c.dealerTok, nil, http.StatusOK))
	if len(linked.Items) != 1 || linked.Items[0].UUID != r1.UUID {
		t.Fatalf("returns of order B = %+v", linked)
	}
	queue := decodeData[itemsOf[struct {
		UUID  string `json:"uuid"`
		Order *struct {
			OrderNo string `json:"order_no"`
		} `json:"order"`
	}]](t, it.accDo("GET", "/v1/accounting/queue/returns", c.distTok, nil, http.StatusOK))
	if len(queue.Items) != 1 || queue.Items[0].UUID != r1.UUID || queue.Items[0].Order == nil ||
		queue.Items[0].Order.OrderNo != orderNo {
		t.Fatalf("accounting queue = %+v", queue)
	}

	// 5. The dealer's own return linked to order B starts requested; the
	// parent cancels it and opens one itself, ships it, then receives it.
	r2 := call("POST", "/v1/stock-transfers", c.dealerTok, map[string]any{
		"kind": "return", "order_uuid": c.b.UUID,
		"items": []map[string]any{{"barcode": c.rolls[1].Barcode}},
	}, http.StatusCreated)
	if r2.Status != "requested" || r2.OpenedByParent || r2.Order == nil || r2.Order.UUID != c.b.UUID {
		t.Fatalf("dealer return = %+v", r2)
	}
	move(c.distTok, r2.UUID, "cancelled")
	r3 := call("POST", "/v1/stock-transfers", c.distTok, map[string]any{
		"from_org_uuid": dealerUUID, "items": []map[string]any{{"barcode": c.rolls[1].Barcode}},
	}, http.StatusCreated)
	if r3.Status != "approved" || !r3.OpenedByParent || r3.Order != nil {
		t.Fatalf("parent return without order = %+v", r3)
	}
	if v := move(c.distTok, r3.UUID, "shipped"); strings.Join(v.AvailableTransitions, ",") != "received,cancelled" {
		t.Fatalf("shipped parent return = %+v", v)
	}
	move(c.distTok, r3.UUID, "received")
	if n := returnRows(r3.UUID); n != 2 {
		t.Fatalf("stock_return rows of the shipped return = %d, want 2", n)
	}
	if got := c.stockOf(c.distTok, c.dist); got != 2 {
		t.Fatalf("distributor stock = %d, want 2", got)
	}
}
