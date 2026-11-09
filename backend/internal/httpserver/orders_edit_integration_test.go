package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

type editLineChange struct {
	Product string  `json:"product"`
	Before  *string `json:"before"`
	After   *string `json:"after"`
}

type editDetail struct {
	Status       string `json:"status"`
	Total        string `json:"total"`
	CanEditItems bool   `json:"can_edit_items"`
	Items        []struct {
		UUID      string `json:"uuid"`
		UnitPrice string `json:"unit_price"`
		LineTotal string `json:"line_total"`
		Quantity  *int32 `json:"quantity"`
		Assigned  string `json:"assigned"`
		Product   struct {
			UUID string `json:"uuid"`
		} `json:"product"`
	} `json:"items"`
	Timeline []struct {
		Action  string           `json:"action"`
		Lines   *int64           `json:"lines"`
		Changes []editLineChange `json:"changes"`
	} `json:"timeline"`
}

func (it *itest) editDetail(tok, orderUUID string) editDetail {
	it.t.Helper()
	code, env := it.do("GET", "/v1/orders/"+orderUUID, hostOlex, tok, nil)
	if code != http.StatusOK {
		it.t.Fatalf("get = %d %s", code, errCode(env))
	}
	var d editDetail
	if err := json.Unmarshal(env.Data, &d); err != nil {
		it.t.Fatal(err)
	}
	return d
}

// TEC-1056 acceptance: the seller's sales (orders.write) and warehouse
// (orders.ship) staff edit an accepted order before it is ready — add a
// line (priced by the pricing module), change an amount (the locked unit
// price stays), remove a line (its assignment and reservation are
// released). An amount below the assigned units is 409; the buyer edits
// only its draft; a ready or shipped order is closed. Every edit is on the
// timeline with the before/after amounts.
func TestIntegrationOrdersSellerEdit(t *testing.T) {
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
	dist := it.org("t1056-dist", "distributor", center)
	sales, spw := it.user("t1056-sales")
	it.member(center, sales, "staff", rbac.RoleCenterStaff)
	picker, ppw := it.user("t1056-picker")
	it.member(center, picker, "staff", rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t1056-dist-owner")
	it.member(dist, distOwner, "owner")

	piece := it.product(center, "T1056P")
	sign := it.product(center, "T1056S")
	plexi := it.product(center, "T1056X")
	it.setListPrice(piece, cur, "50")
	it.setListPrice(sign, cur, "20")
	it.setListPrice(plexi, cur, "7")
	chain := it.stockChain()
	loc := chain.location(center, "T1056")
	su := chain.unit(center, piece, 10561)
	chain.post(ledger.TypeEntry, su, chain.nextRef(), loc)
	su2 := chain.unit(center, piece, 10562)
	chain.post(ledger.TypeEntry, su2, chain.nextRef(), loc)

	salesTok := it.loginOrg(sales, spw, center)
	pickerTok := it.loginOrg(picker, ppw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)
	put := func(tok, orderUUID string, items []any) (int, string) {
		t.Helper()
		code, env := it.do("PUT", "/v1/orders/"+orderUUID+"/items", hostOlex, tok, map[string]any{"items": items})
		return code, errCode(env)
	}

	o := it.orderCall("POST", "/v1/orders", distTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 2},
	}}, http.StatusCreated)
	for _, step := range []struct{ tok, status string }{{distTok, "submitted"}, {salesTok, "approved"}} {
		if code, ec := it.transition(step.tok, o.UUID, step.status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.status, code, ec)
		}
	}
	before := it.editDetail(salesTok, o.UUID)
	pieceLine := before.Items[0].UUID
	if !before.CanEditItems {
		t.Fatal("sales staff cannot edit an approved order")
	}
	// The buyer edits only its draft.
	if d := it.editDetail(distTok, o.UUID); d.CanEditItems {
		t.Fatal("buyer may edit an approved order")
	}
	if code, ec := put(distTok, o.UUID, []any{map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1}}); code != http.StatusConflict || ec != "ORDER_NOT_EDITABLE" {
		t.Fatalf("buyer edit = %d %s", code, ec)
	}

	// Sales adds the sign set to the approved order: priced by the pricing
	// module, the piece line keeps its uuid; a typed price is ignored.
	if code, ec := put(salesTok, o.UUID, []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 2},
		map[string]any{"product_uuid": sign.Uuid.String(), "quantity": 3, "unit_price": "1"},
	}); code != http.StatusOK {
		t.Fatalf("sales add = %d %s", code, ec)
	}
	d := it.editDetail(salesTok, o.UUID)
	if len(d.Items) != 2 || d.Items[0].UUID != pieceLine || d.Items[1].LineTotal != "60.00" || d.Total == before.Total {
		t.Fatalf("after add = %+v", d)
	}

	// Warehouse staff (orders.ship only) starts preparing, assigns one piece
	// and adds the plexiglass set; the response carries the prices (hiding
	// them from the warehouse is TEC-1050 / #511).
	if code, ec := it.transition(pickerTok, o.UUID, "preparing"); code != http.StatusOK {
		t.Fatalf("preparing = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(pickerTok, o.UUID, pieceLine, map[string]any{"barcode": su.Barcode}); code != http.StatusOK {
		t.Fatalf("assign = %d %s", code, ec)
	}
	if code, ec, _ := it.assign(pickerTok, o.UUID, pieceLine, map[string]any{"barcode": su2.Barcode}); code != http.StatusOK {
		t.Fatalf("assign 2 = %d %s", code, ec)
	}
	// Below the assigned amount: refused, nothing changes.
	if code, ec := put(pickerTok, o.UUID, []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
		map[string]any{"product_uuid": sign.Uuid.String(), "quantity": 3},
	}); code != http.StatusConflict || ec != "ORDER_LINE_BELOW_ASSIGNED" {
		t.Fatalf("below assigned = %d %s", code, ec)
	}
	// Remove the assigned piece line, raise the sign set, add plexiglass.
	if code, ec := put(pickerTok, o.UUID, []any{
		map[string]any{"product_uuid": sign.Uuid.String(), "quantity": 4},
		map[string]any{"product_uuid": plexi.Uuid.String(), "quantity": 1},
	}); code != http.StatusOK {
		t.Fatalf("warehouse edit = %d %s", code, ec)
	}
	// TEC-1050: the picker sees no prices; the money is read as sales.
	if pd := it.editDetail(pickerTok, o.UUID); len(pd.Items) != 2 || pd.Total != "" || pd.Items[0].LineTotal != "" {
		t.Fatalf("picker sees prices = %+v", pd)
	}
	d = it.editDetail(salesTok, o.UUID)
	if len(d.Items) != 2 || d.Items[0].LineTotal != "80.00" || d.Items[1].UnitPrice == "" || d.Total != "87.00" {
		t.Fatalf("after warehouse edit = %+v", d)
	}
	var active int
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM stock_reservations WHERE unit_id = ANY($1) AND status = 'active'`,
		[]int64{su.ID, su2.ID}).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("active reservations of the removed line = %d", active)
	}
	// The released unit can go to another order.
	o2 := it.orderCall("POST", "/v1/orders", distTok, map[string]any{"items": []any{
		map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1},
	}}, http.StatusCreated)
	for _, step := range []struct{ tok, status string }{{distTok, "submitted"}, {salesTok, "approved"}, {pickerTok, "preparing"}} {
		if code, ec := it.transition(step.tok, o2.UUID, step.status); code != http.StatusOK {
			t.Fatalf("o2 %s = %d %s", step.status, code, ec)
		}
	}
	o2d := it.editDetail(pickerTok, o2.UUID)
	if code, ec, _ := it.assign(pickerTok, o2.UUID, o2d.Items[0].UUID, map[string]any{"barcode": su.Barcode}); code != http.StatusOK {
		t.Fatalf("reassign released unit = %d %s", code, ec)
	}

	// Timeline: the edits with before/after; the unassignments of the
	// removed line.
	var edits [][]editLineChange
	unassigned := 0
	for _, e := range d.Timeline {
		switch e.Action {
		case "updated":
			edits = append(edits, e.Changes)
		case "unit_unassigned":
			unassigned++
		}
	}
	if len(edits) != 2 || unassigned != 2 {
		raw, _ := json.Marshal(d.Timeline)
		t.Fatalf("timeline = %s", raw)
	}
	if c := edits[0]; len(c) != 1 || c[0].Before != nil || c[0].After == nil || *c[0].After != "3" {
		t.Fatalf("add change = %+v", c)
	}
	if c := edits[1]; len(c) != 3 || c[0].After != nil || *c[0].Before != "2" ||
		*c[1].Before != "3" || *c[1].After != "4" || c[2].Before != nil {
		raw, _ := json.Marshal(c)
		t.Fatalf("warehouse change = %s", raw)
	}

	// A no-op edit writes nothing.
	if code, ec := put(salesTok, o.UUID, []any{
		map[string]any{"product_uuid": sign.Uuid.String(), "quantity": 4},
		map[string]any{"product_uuid": plexi.Uuid.String(), "quantity": 1},
	}); code != http.StatusOK {
		t.Fatalf("no-op = %d %s", code, ec)
	}
	if n := len(it.editDetail(salesTok, o.UUID).Timeline); n != len(d.Timeline) {
		t.Fatalf("no-op wrote a step: %d -> %d", len(d.Timeline), n)
	}
}

// TEC-1056: a ready order is reopened first; a shipped order is closed.
func TestIntegrationOrdersSellerEditClosed(t *testing.T) {
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
	dist := it.org("t1056c-dist", "distributor", center)
	sales, spw := it.user("t1056c-sales")
	it.member(center, sales, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	distOwner, dpw := it.user("t1056c-dist-owner")
	it.member(dist, distOwner, "owner")
	piece := it.product(center, "T1056CP")
	it.setListPrice(piece, cur, "50")
	chain := it.stockChain()
	loc := chain.location(center, "T1056C")
	su := chain.unit(center, piece, 10563)
	chain.post(ledger.TypeEntry, su, chain.nextRef(), loc)
	salesTok := it.loginOrg(sales, spw, center)
	distTok := it.loginOrg(distOwner, dpw, dist)

	o := it.openPreparing(distTok, salesTok, []any{map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 1}})
	if code, ec, _ := it.assign(salesTok, o.UUID, o.Items[0].UUID, map[string]any{"barcode": su.Barcode}); code != http.StatusOK {
		t.Fatalf("assign = %d %s", code, ec)
	}
	body := map[string]any{"items": []any{map[string]any{"product_uuid": piece.Uuid.String(), "quantity": 2}}}
	for _, status := range []string{"ready", "shipped"} {
		if code, ec := it.transition(salesTok, o.UUID, status); code != http.StatusOK {
			t.Fatalf("%s = %d %s", status, code, ec)
		}
		if d := it.editDetail(salesTok, o.UUID); d.CanEditItems {
			t.Fatalf("%s: can_edit_items", status)
		}
		code, env := it.do("PUT", "/v1/orders/"+o.UUID+"/items", hostOlex, salesTok, body)
		if code != http.StatusConflict || errCode(env) != "ORDER_NOT_EDITABLE" {
			t.Fatalf("%s edit = %d %s", status, code, errCode(env))
		}
	}
}
