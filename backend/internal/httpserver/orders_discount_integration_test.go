package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

type discountLine struct {
	Product struct {
		UUID string `json:"uuid"`
	} `json:"product"`
	UnitPrice     string  `json:"unit_price"`
	ListUnitPrice *string `json:"list_unit_price"`
	DiscountPct   *string `json:"discount_pct"`
	IsGift        bool    `json:"is_gift"`
	PriceMissing  bool    `json:"price_missing"`
	LineTotal     string  `json:"line_total"`
}

type discountOrder struct {
	UUID     string         `json:"uuid"`
	Status   string         `json:"status"`
	Currency string         `json:"currency"`
	Subtotal string         `json:"subtotal"`
	Total    string         `json:"total"`
	TryRate  *string        `json:"try_rate"`
	Items    []discountLine `json:"items"`
}

func (it *itest) discountOrder(method, path, access string, body any, want int) discountOrder {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, access, body)
	if code != want {
		it.t.Fatalf("%s %s = %d %s, want %d", method, path, code, errCode(env), want)
	}
	var v discountOrder
	if err := json.Unmarshal(env.Data, &v); err != nil {
		it.t.Fatal(err)
	}
	return v
}

func (o discountOrder) line(t *testing.T, productUUID string) discountLine {
	t.Helper()
	for _, l := range o.Items {
		if l.Product.UUID == productUUID {
			return l
		}
	}
	t.Fatalf("no line for %s in %+v", productUUID, o.Items)
	return discountLine{}
}

// TEC-1064 acceptance: the center's pricing.sale.write sets a dealer's
// standard and first-order discounts; a dealer of a distributor and a
// warehouse user are refused. The quote shows the list price, the default
// discount (first-order on the first order) and the current rate to TRY.
// The center's order for the dealer applies the first-order discount, a
// typed discount wins, a gift line is 0; the next order gets the standard
// discount. The dealer's own draft gets its discount automatically and sees
// neither the list price nor the rate.
func TestIntegrationOrdersDealerDiscounts(t *testing.T) {
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
	dealer := it.org("t1064-dealer", "dealer", center)
	dist := it.org("t1064-dist", "distributor", center)
	farDealer := it.org("t1064-far-dealer", "dealer", dist)

	sales, spw := it.user("t1064-center-sales")
	it.member(center, sales, "staff", rbac.RoleCenterStaff, rbac.RoleCenterAccounting)
	wh, wpw := it.user("t1064-center-warehouse")
	it.member(center, wh, "staff", rbac.RoleCenterWarehouse)
	dealerOwner, dpw := it.user("t1064-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	salesTok := it.loginOrg(sales, spw, center)
	whTok := it.loginOrg(wh, wpw, center)
	dealerTok := it.loginOrg(dealerOwner, dpw, dealer)

	film := it.product(center, "T1064F")
	plexi := it.product(center, "T1064P")
	it.setListPrice(film, cur, "100")
	it.setListPrice(plexi, cur, "40")

	discPath := "/v1/tenant/pricing/discounts/" + dealer.Uuid.String()
	// Only pricing.sale.write reads and writes the discounts.
	if code, env := it.do("GET", discPath, hostOlex, whTok, nil); code != http.StatusForbidden {
		t.Fatalf("warehouse read = %d %s", code, errCode(env))
	}
	it.stepUp(sales.Uuid)
	if code, env := it.do("PUT", "/v1/tenant/pricing/discounts/"+farDealer.Uuid.String(), hostOlex, salesTok,
		map[string]any{"standard_discount_pct": 10}); code != http.StatusNotFound {
		t.Fatalf("far dealer = %d %s", code, errCode(env))
	}
	if code, env := it.do("PUT", discPath, hostOlex, salesTok,
		map[string]any{"standard_discount_pct": 120}); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("120%% = %d %s", code, errCode(env))
	}
	code, env := it.do("PUT", discPath, hostOlex, salesTok,
		map[string]any{"standard_discount_pct": 15, "first_order_discount_pct": "20"})
	if code != http.StatusOK {
		t.Fatalf("set discounts = %d %s", code, errCode(env))
	}
	var dv struct {
		Standard   *string `json:"standard_discount_pct"`
		FirstOrder *string `json:"first_order_discount_pct"`
		IsFirst    bool    `json:"first_order"`
	}
	if err := json.Unmarshal(env.Data, &dv); err != nil {
		t.Fatal(err)
	}
	if !sameAmount(dv.Standard, "15") || !sameAmount(dv.FirstOrder, "20") || !dv.IsFirst {
		t.Fatalf("discounts = %+v", dv)
	}

	// The quote: list price, first-order discount, discounted price, TRY rate.
	code, env = it.do("POST", "/v1/orders/quote", hostOlex, salesTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(), "product_uuids": []string{film.Uuid.String(), plexi.Uuid.String()},
	})
	if code != http.StatusOK {
		t.Fatalf("quote = %d %s", code, errCode(env))
	}
	var qv struct {
		Currency      string  `json:"currency"`
		PricesVisible bool    `json:"prices_visible"`
		FirstOrder    bool    `json:"first_order"`
		DiscountPct   *string `json:"discount_pct"`
		TryRate       *string `json:"try_rate"`
		Items         []struct {
			ListUnitPrice *string `json:"list_unit_price"`
			UnitPrice     *string `json:"unit_price"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &qv); err != nil {
		t.Fatal(err)
	}
	if qv.Currency != cur || !qv.PricesVisible || !qv.FirstOrder || !sameAmount(qv.DiscountPct, "20") ||
		qv.TryRate == nil || len(qv.Items) != 2 || !sameAmount(qv.Items[0].ListUnitPrice, "100") ||
		!sameAmount(qv.Items[0].UnitPrice, "80") {
		t.Fatalf("quote = %s", env.Data)
	}

	// 1. First order: 20% on the film, the plexi set as a gift.
	first := it.discountOrder("POST", "/v1/orders", salesTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(),
		"items": []any{
			map[string]any{"product_uuid": film.Uuid.String(), "quantity": 2},
			map[string]any{"product_uuid": plexi.Uuid.String(), "quantity": 1, "is_gift": true, "discount_pct": 50},
		},
	}, http.StatusCreated)
	f := first.line(t, film.Uuid.String())
	if !sameAmount(f.ListUnitPrice, "100") || !sameAmount(f.DiscountPct, "20") || !sameAmount(&f.UnitPrice, "80") ||
		!sameAmount(&f.LineTotal, "160") || f.IsGift {
		t.Fatalf("film line = %+v", f)
	}
	g := first.line(t, plexi.Uuid.String())
	if !g.IsGift || g.DiscountPct != nil || !sameAmount(&g.UnitPrice, "0") || !sameAmount(&g.LineTotal, "0") ||
		g.PriceMissing || !sameAmount(g.ListUnitPrice, "40") {
		t.Fatalf("gift line = %+v", g)
	}
	if first.Status != "approved" || !sameAmount(&first.Total, "160") || first.TryRate == nil {
		t.Fatalf("first order = %+v", first)
	}

	// 2. The next order gets the standard discount; a typed one wins.
	second := it.discountOrder("POST", "/v1/orders", salesTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(),
		"items": []any{
			map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1},
			map[string]any{"product_uuid": plexi.Uuid.String(), "quantity": 3, "discount_pct": "12.5"},
		},
	}, http.StatusCreated)
	if l := second.line(t, film.Uuid.String()); !sameAmount(l.DiscountPct, "15") || !sameAmount(&l.UnitPrice, "85") {
		t.Fatalf("standard line = %+v", l)
	}
	if l := second.line(t, plexi.Uuid.String()); !sameAmount(l.DiscountPct, "12.5") || !sameAmount(&l.UnitPrice, "35") ||
		!sameAmount(&l.LineTotal, "105") {
		t.Fatalf("typed discount line = %+v", l)
	}
	if !sameAmount(&second.Total, "190") {
		t.Fatalf("second total = %s", second.Total)
	}
	// A bad typed discount is refused.
	if code, env := it.do("POST", "/v1/orders", hostOlex, salesTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(),
		"items":          []any{map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1, "discount_pct": 101}},
	}); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("101%% = %d %s", code, errCode(env))
	}

	// 3. The dealer's own draft: the standard discount, gift ignored; the
	// dealer sees the discounted price only.
	own := it.discountOrder("POST", "/v1/orders", dealerTok, map[string]any{
		"items": []any{map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1, "is_gift": true, "discount_pct": 90}},
	}, http.StatusCreated)
	l := own.line(t, film.Uuid.String())
	if own.Status != "draft" || l.IsGift || !sameAmount(&l.UnitPrice, "85") || l.ListUnitPrice != nil || l.DiscountPct != nil {
		t.Fatalf("dealer own = %+v", own)
	}
	code, env = it.do("POST", "/v1/orders/quote", hostOlex, dealerTok, map[string]any{
		"product_uuids": []string{film.Uuid.String()},
	})
	if code != http.StatusOK {
		t.Fatalf("dealer quote = %d %s", code, errCode(env))
	}
	var dq struct {
		DiscountPct *string `json:"discount_pct"`
		Items       []struct {
			ListUnitPrice *string `json:"list_unit_price"`
			UnitPrice     *string `json:"unit_price"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &dq); err != nil {
		t.Fatal(err)
	}
	if dq.DiscountPct != nil || len(dq.Items) != 1 || dq.Items[0].ListUnitPrice != nil || !sameAmount(dq.Items[0].UnitPrice, "85") {
		t.Fatalf("dealer quote = %s", env.Data)
	}
	// The seller sees the terms of the dealer's draft.
	sv := it.discountOrder("GET", "/v1/orders/"+own.UUID, salesTok, nil, http.StatusOK)
	if sl := sv.line(t, film.Uuid.String()); !sameAmount(sl.ListUnitPrice, "100") || !sameAmount(sl.DiscountPct, "15") {
		t.Fatalf("seller view of the draft = %+v", sl)
	}

	// 4. Without a discount nothing changes.
	if code, env := it.do("PUT", discPath, hostOlex, salesTok, map[string]any{
		"standard_discount_pct": nil, "first_order_discount_pct": nil,
	}); code != http.StatusOK {
		t.Fatalf("clear = %d %s", code, errCode(env))
	}
	plain := it.discountOrder("POST", "/v1/orders", salesTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(),
		"items":          []any{map[string]any{"product_uuid": film.Uuid.String(), "quantity": 1}},
	}, http.StatusCreated)
	if pl := plain.line(t, film.Uuid.String()); pl.DiscountPct != nil || !sameAmount(&pl.UnitPrice, "100") {
		t.Fatalf("no discount = %+v", pl)
	}
}
