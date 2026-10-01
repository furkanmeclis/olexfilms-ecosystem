package httpserver

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type priceView struct {
	ProductUUID string `json:"product_uuid"`
	Viewer      string `json:"viewer"`
	Prices      []struct {
		Currency             string  `json:"currency"`
		PurchasePrice        *string `json:"purchase_price"`
		PurchasePriceSource  string  `json:"purchase_price_source"`
		SalePrice            *string `json:"sale_price"`
		RecommendedSalePrice *string `json:"recommended_sale_price"`
	} `json:"prices"`
}

// product creates a center product of the brand directly (catalog CRUD is
// TEC-145).
func (it *itest) product(center db.Organization, sku string) db.Product {
	it.t.Helper()
	ctx := context.Background()
	cat, err := it.q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: center.BrandID, Name: "t146-" + sku + "-" + it.suffix,
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		it.t.Fatalf("category: %v", err)
	}
	p, err := it.q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: center.ID, BrandID: center.BrandID, CategoryID: cat.ID,
		Sku: sku + "-" + it.suffix, Name: "Film " + sku, Images: []byte("[]"), UnitType: "piece", Active: true,
	})
	if err != nil {
		it.t.Fatalf("product: %v", err)
	}
	it.t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM products WHERE id = $1", p.ID)
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM product_categories WHERE id = $1", cat.ID)
	})
	return p
}

func (it *itest) priceView(access, productUUID string) (priceView, string) {
	it.t.Helper()
	code, env := it.do("GET", "/v1/tenant/pricing/products/"+productUUID, hostOlex, access, nil)
	if code != http.StatusOK {
		it.t.Fatalf("price view: %d %s", code, errCode(env))
	}
	var v priceView
	if err := json.Unmarshal(env.Data, &v); err != nil {
		it.t.Fatal(err)
	}
	return v, string(env.Data)
}

func sameAmount(got *string, want string) bool {
	if got == nil {
		return false
	}
	g, ok1 := new(big.Rat).SetString(*got)
	w, ok2 := new(big.Rat).SetString(want)
	return ok1 && ok2 && g.Cmp(w) == 0
}

// TEC-146 acceptance: the center sets the list price and a distributor
// price; the distributor sees its purchase price but not the center's; the
// distributor's dealer price is the only price its dealer sees; dealer staff
// without a pricing grant gets 403.
func TestIntegrationPricingVisibility(t *testing.T) {
	it := newIntegration(t)
	center := it.brandCenter("olex")
	dist := it.org("t146-dist", "distributor", center)
	otherDist := it.org("t146-dist2", "distributor", center)
	dealer := it.org("t146-dealer", "dealer", dist)

	acc, apw := it.user("t146-acc")
	it.member(center, acc, "staff", rbac.RoleCenterAccounting)
	distOwner, dpw := it.user("t146-dist-owner")
	it.member(dist, distOwner, "owner")
	otherOwner, opw := it.user("t146-dist2-owner")
	it.member(otherDist, otherOwner, "owner")
	dealerOwner, rpw := it.user("t146-dealer-owner")
	it.member(dealer, dealerOwner, "owner")
	dealerStaff, spw := it.user("t146-dealer-staff")
	it.member(dealer, dealerStaff, "staff")

	p := it.product(center, "T146")
	pid := p.Uuid.String()
	accTok := it.loginOrg(acc, apw, center)

	// 1. Center writes the list price; pricing.sale.write needs a step-up.
	listBody := map[string]any{"purchase_price": "50", "sale_to_distributor_price": "80", "recommended_sale_price": "150"}
	code, env := it.do("PUT", "/v1/tenant/pricing/products/"+pid+"/prices/TRY", hostOlex, accTok, listBody)
	if code != http.StatusForbidden || errCode(env) != "STEP_UP_REQUIRED" {
		t.Fatalf("list price without step-up = %d %s", code, errCode(env))
	}
	it.stepUp(acc.Uuid)
	if code, env = it.do("PUT", "/v1/tenant/pricing/products/"+pid+"/prices/TRY", hostOlex, accTok, listBody); code != http.StatusOK {
		t.Fatalf("list price = %d %s", code, errCode(env))
	}
	cv, _ := it.priceView(accTok, pid)
	if cv.Viewer != "center" || len(cv.Prices) != 1 || !sameAmount(cv.Prices[0].PurchasePrice, "50") ||
		!sameAmount(cv.Prices[0].SalePrice, "80") || !sameAmount(cv.Prices[0].RecommendedSalePrice, "150") {
		t.Fatalf("center view = %+v", cv)
	}

	// 2. Center gives the distributor a specific price.
	code, env = it.do("PUT", "/v1/tenant/pricing/products/"+pid+"/distributor-prices/"+dist.Uuid.String()+"/TRY",
		hostOlex, accTok, map[string]string{"price": "70"})
	if code != http.StatusOK {
		t.Fatalf("distributor price = %d %s", code, errCode(env))
	}
	// A dealer is not a valid distributor-price target.
	code, _ = it.do("PUT", "/v1/tenant/pricing/products/"+pid+"/distributor-prices/"+dealer.Uuid.String()+"/TRY",
		hostOlex, accTok, map[string]string{"price": "70"})
	if code != http.StatusNotFound {
		t.Fatalf("dealer as distributor target = %d", code)
	}
	code, env = it.do("GET", "/v1/tenant/pricing/distributor-prices?product_uuid="+pid, hostOlex, accTok, nil)
	var page struct {
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(env.Data, &page)
	if code != http.StatusOK || page.Total != 1 {
		t.Fatalf("distributor price list = %d total %d", code, page.Total)
	}

	// 3. The distributor sees its purchase price (the override), never the
	// center's purchase or recommended price.
	distTok := it.loginOrg(distOwner, dpw, dist)
	dv, raw := it.priceView(distTok, pid)
	if dv.Viewer != "distributor" || len(dv.Prices) != 1 || !sameAmount(dv.Prices[0].PurchasePrice, "70") ||
		dv.Prices[0].PurchasePriceSource != "override" || dv.Prices[0].SalePrice != nil {
		t.Fatalf("distributor view = %s", raw)
	}
	if strings.Contains(raw, "recommended") {
		t.Fatalf("distributor view leaks center prices: %s", raw)
	}
	// Another distributor without an override pays the list price.
	ov, raw := it.priceView(it.loginOrg(otherOwner, opw, otherDist), pid)
	if len(ov.Prices) != 1 || !sameAmount(ov.Prices[0].PurchasePrice, "80") || ov.Prices[0].PurchasePriceSource != "list" {
		t.Fatalf("other distributor view = %s", raw)
	}
	// The distributor cannot write center prices.
	it.stepUp(distOwner.Uuid)
	if code, env = it.do("PUT", "/v1/tenant/pricing/products/"+pid+"/prices/TRY", hostOlex, distTok, listBody); code != http.StatusForbidden ||
		errCode(env) != "FORBIDDEN" {
		t.Fatalf("distributor list price = %d %s", code, errCode(env))
	}

	// 4. The distributor writes its dealer price; the dealer sees only that.
	if code, env = it.do("PUT", "/v1/tenant/pricing/products/"+pid+"/dealer-prices/TRY", hostOlex, distTok,
		map[string]string{"price": "95"}); code != http.StatusOK {
		t.Fatalf("dealer price = %d %s", code, errCode(env))
	}
	dv, raw = it.priceView(distTok, pid)
	if !sameAmount(dv.Prices[0].SalePrice, "95") {
		t.Fatalf("distributor own sale price = %s", raw)
	}
	dealerTok := it.loginOrg(dealerOwner, rpw, dealer)
	rv, raw := it.priceView(dealerTok, pid)
	if rv.Viewer != "dealer" || len(rv.Prices) != 1 || !sameAmount(rv.Prices[0].PurchasePrice, "95") ||
		rv.Prices[0].PurchasePriceSource != "distributor" {
		t.Fatalf("dealer view = %s", raw)
	}
	for _, field := range []string{"\"sale_price\"", "\"recommended_sale_price\""} {
		if strings.Contains(raw, field) {
			t.Fatalf("dealer view leaks %s: %s", field, raw)
		}
	}
	it.stepUp(dealerOwner.Uuid)
	if code, _ = it.do("PUT", "/v1/tenant/pricing/products/"+pid+"/dealer-prices/TRY", hostOlex, dealerTok,
		map[string]string{"price": "1"}); code != http.StatusForbidden {
		t.Fatalf("dealer writes dealer price = %d", code)
	}

	// 5. Dealer staff holds no pricing grant.
	staffTok := it.loginOrg(dealerStaff, spw, dealer)
	for _, path := range []string{"/v1/tenant/pricing/products/" + pid, "/v1/tenant/pricing/products"} {
		if code, _ = it.do("GET", path, hostOlex, staffTok, nil); code != http.StatusForbidden {
			t.Fatalf("dealer staff GET %s = %d, want 403", path, code)
		}
	}

	// 6. The list endpoint returns the same masked view.
	code, env = it.do("GET", "/v1/tenant/pricing/products?q="+p.Sku, hostOlex, dealerTok, nil)
	if code != http.StatusOK || strings.Contains(string(env.Data), "sale_price") {
		t.Fatalf("dealer list = %d %s", code, env.Data)
	}
}
