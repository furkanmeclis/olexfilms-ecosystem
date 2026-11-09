package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

func (it *itest) productDealerOrderable(uuid string) bool {
	it.t.Helper()
	var open bool
	if err := it.pool.QueryRow(context.Background(),
		"SELECT dealer_orderable FROM products WHERE uuid = $1", uuid).Scan(&open); err != nil {
		it.t.Fatalf("product %s: %v", uuid, err)
	}
	return open
}

// TEC-1061 acceptance: the center closes a product to dealer orders from
// the product form (PATCH) and the list bulk action (undoable); the center
// filters on it. A dealer / distributor never lists the closed product,
// whatever filter it sends, and its own order with it is refused with
// PRODUCT_NOT_ORDERABLE, while the center's order for the dealer accepts it.
func TestIntegrationProductDealerOrderable(t *testing.T) {
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
	dealer := it.org("t1061-dealer", "dealer", center)
	dist := it.org("t1061-dist", "distributor", center)

	staff, spw := it.user("t1061-center-staff")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	dealerOwner, rpw := it.user("t1061-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	distOwner, dpw := it.user("t1061-dist-owner")
	it.member(dist, distOwner, "owner")
	staffTok := it.loginOrg(staff, spw, center)
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)
	distTok := it.loginOrg(distOwner, dpw, dist)

	open := it.product(center, "T1061O")
	closed := it.product(center, "T1061C")
	it.setListPrice(open, cur, "50")
	it.setListPrice(closed, cur, "40")
	if !open.DealerOrderable || !closed.DealerOrderable {
		t.Fatal("a new product must be open to dealer orders by default")
	}

	// 1. The center closes the product from the form.
	code, env := it.do("PATCH", "/v1/catalog/products/"+closed.Uuid.String(), hostOlex, staffTok,
		map[string]any{"dealer_orderable": false})
	if code != http.StatusOK {
		t.Fatalf("patch = %d %s", code, errCode(env))
	}
	var view struct {
		DealerOrderable bool `json:"dealer_orderable"`
		Active          bool `json:"active"`
	}
	if err := json.Unmarshal(env.Data, &view); err != nil || view.DealerOrderable || !view.Active {
		t.Fatalf("patched view = %+v %v", view, err)
	}
	// A dealer cannot change it (catalog writes are center only).
	if code, env := it.do("PATCH", "/v1/catalog/products/"+open.Uuid.String(), hostOlex, dealerTok,
		map[string]any{"dealer_orderable": false}); code == http.StatusOK {
		t.Fatalf("dealer patch = %d %s", code, errCode(env))
	}

	// 2. Lists: the center sees both and filters; others see the open one.
	q := "/v1/catalog/products?active=true&sort=sku&q=" + url.QueryEscape("-"+it.suffix)
	skus := func(tok, extra string) []string {
		got, _, _ := it.listNames(q+extra, hostOlex, tok, http.StatusOK, true)
		return got
	}
	wantOrder(t, "center", skus(staffTok, ""), []string{closed.Sku, open.Sku})
	wantOrder(t, "center closed", skus(staffTok, "&dealer_orderable=false"), []string{closed.Sku})
	wantOrder(t, "center open", skus(staffTok, "&dealer_orderable=true"), []string{open.Sku})
	wantOrder(t, "center sort", skus(staffTok, "&sort=dealer_orderable"), []string{closed.Sku, open.Sku})
	wantOrder(t, "dealer", skus(dealerTok, ""), []string{open.Sku})
	wantOrder(t, "dealer asks closed", skus(dealerTok, "&dealer_orderable=false"), []string{})
	wantOrder(t, "distributor", skus(distTok, ""), []string{open.Sku})
	if _, _, ec := it.listNames(q+"&dealer_orderable=maybe", hostOlex, staffTok, http.StatusBadRequest, true); ec == "" {
		t.Fatal("bad dealer_orderable must be 400")
	}

	// 3. The dealer's own order with the closed product is refused.
	body := func(p db.Product) []byte {
		b, _ := json.Marshal(map[string]any{"items": []any{map[string]any{"product_uuid": p.Uuid.String(), "quantity": 1}}})
		return b
	}
	rec := it.raw("POST", "/v1/orders", dealerTok, "application/json", body(closed), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "PRODUCT_NOT_ORDERABLE") {
		t.Fatalf("dealer closed order = %d %s", rec.Code, rec.Body.String())
	}
	own := it.orderCall("POST", "/v1/orders", dealerTok,
		map[string]any{"items": []any{map[string]any{"product_uuid": open.Uuid.String(), "quantity": 1}}}, http.StatusCreated)
	if own.Status != "draft" {
		t.Fatalf("dealer own = %+v", own)
	}
	// Replacing the draft lines with the closed product is refused too.
	rec = it.raw("PUT", "/v1/orders/"+own.UUID+"/items", dealerTok, "application/json", body(closed), nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "PRODUCT_NOT_ORDERABLE") {
		t.Fatalf("dealer closed replace = %d %s", rec.Code, rec.Body.String())
	}

	// 4. The center's order for the dealer accepts the closed product.
	gift := it.orderCall("POST", "/v1/orders", staffTok, map[string]any{
		"buyer_org_uuid": dealer.Uuid.String(),
		"items":          []any{map[string]any{"product_uuid": closed.Uuid.String(), "quantity": 1}},
	}, http.StatusCreated)
	if gift.Status != "approved" || len(gift.Items) != 1 {
		t.Fatalf("center gift order = %+v", gift)
	}

	// 5. Bulk: open it again, then undo closes it back.
	run := it.bulkRun("/v1/catalog/products/bulk", staffTok, map[string]any{
		"action": "enable_dealer_order",
		"target": map[string]any{"scope": "ids", "ids": []string{closed.Uuid.String()}},
	})
	if run.Summary.Succeeded != 1 || !it.productDealerOrderable(closed.Uuid.String()) {
		t.Fatalf("enable run = %+v", run)
	}
	wantOrder(t, "dealer after enable", skus(dealerTok, ""), []string{closed.Sku, open.Sku})
	it.bulkUndo("/v1/tenant/bulk-operations/"+run.Operation.UUID.String()+"/undo", hostOlex, staffTok, http.StatusOK)
	if it.productDealerOrderable(closed.Uuid.String()) {
		t.Fatal("undo did not close the product again")
	}
	run = it.bulkRun("/v1/catalog/products/bulk", staffTok, map[string]any{
		"action": "disable_dealer_order",
		"target": map[string]any{"scope": "ids", "ids": []string{open.Uuid.String()}},
	})
	if run.Summary.Succeeded != 1 || it.productDealerOrderable(open.Uuid.String()) {
		t.Fatalf("disable run = %+v", run)
	}
	wantOrder(t, "dealer after disable", skus(dealerTok, ""), []string{})
}
