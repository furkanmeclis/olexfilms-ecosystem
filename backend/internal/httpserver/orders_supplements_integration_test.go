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

// supplementLinks is the TEC-1222 part of an order view.
type supplementLinks struct {
	IsSupplement     bool `json:"is_supplement"`
	CanAddSupplement bool `json:"can_add_supplement"`
	ParentOrder      *struct {
		UUID    string `json:"uuid"`
		OrderNo string `json:"order_no"`
	} `json:"parent_order"`
	Supplements []struct {
		UUID string `json:"uuid"`
	} `json:"supplements"`
	Items []struct {
		IsGift bool `json:"is_gift"`
	} `json:"items"`
}

// TEC-1222 acceptance: a supplementary order opens on a shipped order of
// the center, copies its seller, buyer and warehouse, starts approved with
// its own lines (a gift line too) and both orders link to each other; the
// list marks it. It is refused before shipping, for the buyer and without
// orders.write / orders.ship. A supplement of a supplement hangs on the
// main order.
func TestIntegrationOrderSupplements(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	brand, err := it.q.GetBrandByID(ctx, center.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	if cur := strings.TrimSpace(brand.Currency); cur != "TRY" {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if err := it.q.UpsertExchangeRate(ctx, db.UpsertExchangeRateParams{
			RateDate: pgtype.Date{Time: today, Valid: true}, Base: cur, Quote: "TRY", Rate: "35", Source: "manual",
		}); err != nil {
			t.Fatalf("rate: %v", err)
		}
	}
	dealer := it.org("t1222-dealer", "dealer", center)
	staff, spw := it.user("t1222-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	acc, apw := it.user("t1222-center-accounting")
	it.member(center, acc, "staff", rbac.RoleCenterAccounting)
	wh, wpw := it.user("t1222-center-warehouse")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	dealerOwner, rpw := it.user("t1222-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	staffTok := it.loginOrg(staff, spw, center)
	accTok := it.loginOrg(acc, apw, center)
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)
	whTok := it.loginOrg(wh, wpw, center)

	film := it.product(center, "T1222F")
	plexi := it.product(center, "T1222P")
	chain := it.stockChain()
	loc := chain.location(center, "T1222")
	u1 := chain.unit(center, film, 12221)
	chain.post(ledger.TypeEntry, u1, chain.nextRef(), loc)

	main := it.orderCall("POST", "/v1/orders", staffTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(),
		"items":          []any{map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1}},
	}, http.StatusCreated)
	lines := []any{
		map[string]any{"product_uuid": plexi.Uuid.String(), "quantity": 2},
		map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1, "is_gift": true},
	}
	path := "/v1/orders/" + main.UUID + "/supplements"

	// Not before the goods left.
	if code, env := it.do("POST", path, hostOlex, staffTok, map[string]any{"items": lines}); code != http.StatusConflict ||
		errCode(env) != "ORDER_NOT_SUPPLEMENTABLE" {
		t.Fatalf("approved main = %d %s", code, errCode(env))
	}
	if code, ec := it.transition(staffTok, main.UUID, "preparing"); code != http.StatusOK {
		t.Fatalf("preparing = %d %s", code, ec)
	}
	prep := it.orderCall("GET", "/v1/orders/"+main.UUID, staffTok, nil, http.StatusOK)
	it.shipOrder(staffTok, prep, [][]map[string]any{{{"barcode": u1.Barcode}}})

	// The buyer and a user without orders.write / orders.ship cannot.
	if code, env := it.do("POST", path, hostOlex, dealerTok, map[string]any{"items": lines}); code != http.StatusForbidden {
		t.Fatalf("dealer = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", path, hostOlex, accTok, map[string]any{"items": lines}); code != http.StatusForbidden {
		t.Fatalf("accounting = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", path, hostOlex, staffTok, map[string]any{"items": []any{}}); code != http.StatusUnprocessableEntity &&
		code != http.StatusBadRequest {
		t.Fatalf("no lines = %d %s", code, errCode(env))
	}

	code, env := it.do("POST", path, hostOlex, staffTok, map[string]any{"items": lines, "note": "plexi set"})
	if code != http.StatusCreated {
		t.Fatalf("supplement = %d %s", code, errCode(env))
	}
	var sup orderView
	var supLinks supplementLinks
	if err := json.Unmarshal(env.Data, &sup); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(env.Data, &supLinks); err != nil {
		t.Fatal(err)
	}
	if sup.Status != "approved" || sup.Seller.UUID != center.Uuid.String() || sup.Buyer.UUID != dealer.Uuid.String() ||
		len(sup.Items) != 2 || sup.UUID == main.UUID {
		t.Fatalf("supplement = %+v", sup)
	}
	if !supLinks.IsSupplement || supLinks.ParentOrder == nil || supLinks.ParentOrder.UUID != main.UUID ||
		!supLinks.Items[1].IsGift || supLinks.CanAddSupplement {
		t.Fatalf("supplement links = %+v", supLinks)
	}
	mainRow, supRow := it.orderRow(main.UUID, center.BrandID), it.orderRow(sup.UUID, center.BrandID)
	if !supRow.ParentOrderID.Valid || supRow.ParentOrderID.Int64 != mainRow.ID ||
		supRow.SellerWarehouseID != mainRow.SellerWarehouseID || !supRow.CreatedBySeller {
		t.Fatalf("row parent %v warehouse %v/%v", supRow.ParentOrderID, supRow.SellerWarehouseID, mainRow.SellerWarehouseID)
	}

	// The main order lists it, for the seller and the buyer.
	for _, tok := range []string{staffTok, dealerTok} {
		code, env = it.do("GET", "/v1/orders/"+main.UUID, hostOlex, tok, nil)
		if code != http.StatusOK {
			t.Fatalf("main detail = %d %s", code, errCode(env))
		}
		var l supplementLinks
		if err := json.Unmarshal(env.Data, &l); err != nil {
			t.Fatal(err)
		}
		if l.IsSupplement || l.ParentOrder != nil || len(l.Supplements) != 1 || l.Supplements[0].UUID != sup.UUID {
			t.Fatalf("main links = %+v", l)
		}
		if tok == staffTok && !l.CanAddSupplement {
			t.Fatal("seller cannot add a supplement to the shipped main order")
		}
		if tok == dealerTok && l.CanAddSupplement {
			t.Fatal("buyer can add a supplement")
		}
	}

	// The list marks the supplement.
	code, env = it.do("GET", "/v1/orders?limit=100", hostOlex, staffTok, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d %s", code, errCode(env))
	}
	var page struct {
		Items []struct {
			UUID         string `json:"uuid"`
			IsSupplement bool   `json:"is_supplement"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	marks := map[string]bool{}
	for _, row := range page.Items {
		marks[row.UUID] = row.IsSupplement
	}
	if v, ok := marks[sup.UUID]; !ok || !v {
		t.Fatalf("list supplement mark = %v %v", v, ok)
	}
	if v, ok := marks[main.UUID]; !ok || v {
		t.Fatalf("list main mark = %v %v", v, ok)
	}

	// The warehouse (orders.ship, no prices) opens one too; a typed price
	// is ignored, the line takes the dealer's purchase price.
	typed := []any{map[string]any{"product_uuid": plexi.Uuid.String(), "quantity": 1, "unit_price": "999"}}
	byWh := it.orderCall("POST", path, whTok, map[string]any{"items": typed}, http.StatusCreated)
	if byWh.Status != "approved" || len(byWh.Items) != 1 {
		t.Fatalf("warehouse supplement = %+v", byWh)
	}
	if row := it.orderRow(byWh.UUID, center.BrandID); sameAmount(&byWh.Total, "999") || !row.ParentOrderID.Valid {
		t.Fatalf("warehouse supplement total %v parent %v", byWh.Total, row.ParentOrderID)
	}
	var whPrice string
	if err := it.pool.QueryRow(ctx, `SELECT oi.unit_price::text FROM order_items oi JOIN orders o ON o.id = oi.order_id WHERE o.uuid = $1`,
		byWh.UUID).Scan(&whPrice); err != nil || strings.HasPrefix(whPrice, "999") {
		t.Fatalf("warehouse typed price kept: %q %v", whPrice, err)
	}

	// A supplement of a shipped supplement hangs on the main order; an
	// approved one gets none yet.
	if code, env := it.do("POST", "/v1/orders/"+sup.UUID+"/supplements", hostOlex, staffTok, map[string]any{"items": lines}); code != http.StatusConflict {
		t.Fatalf("approved supplement = %d %s", code, errCode(env))
	}
	// Shortcut through the warehouse flow, tested above for the main order.
	if _, err := it.pool.Exec(ctx, `UPDATE orders SET status = 'shipped', shipped_at = NOW() WHERE id = $1`, supRow.ID); err != nil {
		t.Fatal(err)
	}
	second := it.orderCall("POST", "/v1/orders/"+sup.UUID+"/supplements", staffTok, map[string]any{"items": lines}, http.StatusCreated)
	if row := it.orderRow(second.UUID, center.BrandID); !row.ParentOrderID.Valid || row.ParentOrderID.Int64 != mainRow.ID {
		t.Fatalf("second supplement parent = %v, want %d", row.ParentOrderID, mainRow.ID)
	}
}
