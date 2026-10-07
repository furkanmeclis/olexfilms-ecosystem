package httpserver

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
)

type listPage[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type priceCatalogResp struct {
	ProductUUID          string  `json:"product_uuid"`
	Name                 string  `json:"name"`
	SalePrice            *string `json:"sale_price"`
	RecommendedSalePrice *string `json:"recommended_sale_price"`
	PurchasePrice        *string `json:"purchase_price"`
	EstimatedProfit      *string `json:"estimated_profit"`
}

type saleListResp struct {
	UUID     string  `json:"uuid"`
	Total    string  `json:"total"`
	Profit   *string `json:"profit"`
	Products string  `json:"products"`
	Voided   bool    `json:"voided"`
}

type purchaseListResp struct {
	UUID         string `json:"uuid"`
	SupplierName string `json:"supplier_name"`
	Amount       string `json:"amount"`
	Description  string `json:"description"`
}

func strv(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// TEC-348: list reads of the dealer sales screens (price catalog, barcode
// lookup, sales, suppliers, purchases) follow docs/list-contract.md.
func TestIntegrationDealerSalesLists(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t348-dist", "distributor", center)
	dealer := it.org("t348-dealer", "dealer", dist)

	owner, opw := it.user("t348-owner")
	it.member(dealer, owner, "owner")
	staff, spw := it.user("t348-staff")
	it.member(dealer, staff, "staff")
	ownerTok := it.loginOrg(owner, opw, dealer)
	staffTok := it.loginOrg(staff, spw, dealer)

	product := it.product(center, "T348P")
	it.setListPrice(product, dealer.Currency, "500")
	if _, err := it.q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: product.ID, BrandID: product.BrandID, Currency: dealer.Currency,
		PurchasePrice: txt("300"), SaleToDistributorPrice: txt("700"), RecommendedSalePrice: txt("1500"),
	}); err != nil {
		t.Fatalf("recommended price: %v", err)
	}
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: product.ID, BrandID: product.BrandID, DistributorOrgID: dist.ID, Currency: dealer.Currency, Price: "800",
	}); err != nil {
		t.Fatalf("dealer purchase price: %v", err)
	}
	chain := it.stockChain()
	u1 := chain.unit(center, product, 34801)
	centerOwner := ledger.Owner{Type: ledger.OwnerOrganization, ID: center.ID, OrgID: center.ID}
	dealerOwner := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID}
	chain.post(ledger.TypeEntry, u1, chain.nextRef(), centerOwner)
	chain.ship(u1, ledger.TypeTransferOut, ledger.TypeTransferIn, dealer, dealerOwner)

	catalogPath := "/v1/dealer-prices/catalog?q=" + url.QueryEscape(product.Sku)
	if code, _ := it.do("GET", catalogPath, hostOlex, staffTok, nil); code != http.StatusForbidden {
		t.Fatalf("dealer_staff catalog = %d, want 403", code)
	}
	if code, env := it.do("GET", "/v1/dealer-prices/catalog?sort=bogus", hostOlex, ownerTok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("bad catalog sort = %d %s, want 400", code, errCode(env))
	}
	cat := decodeData[listPage[priceCatalogResp]](t, mustDo(t, it, "GET", catalogPath, ownerTok, nil, http.StatusOK))
	if cat.Total != 1 || len(cat.Items) != 1 || cat.Items[0].SalePrice != nil ||
		strv(cat.Items[0].RecommendedSalePrice) != "1500.00" || strv(cat.Items[0].PurchasePrice) != "800.00" ||
		strv(cat.Items[0].EstimatedProfit) != "700.00" {
		t.Fatalf("unpriced catalog = %+v", cat)
	}
	if got := decodeData[listPage[priceCatalogResp]](t, mustDo(t, it, "GET", catalogPath+"&priced=true", ownerTok, nil, http.StatusOK)); got.Total != 0 {
		t.Fatalf("priced=true before pricing = %+v", got)
	}
	mustDo(t, it, "PUT", "/v1/dealer-prices", ownerTok, map[string]any{"product_uuid": product.Uuid.String(), "sale_price": "1200"}, http.StatusOK)
	cat = decodeData[listPage[priceCatalogResp]](t, mustDo(t, it, "GET", catalogPath+"&priced=true&sort=-sale_price", ownerTok, nil, http.StatusOK))
	if cat.Total != 1 || strv(cat.Items[0].SalePrice) != "1200.00" || strv(cat.Items[0].EstimatedProfit) != "400.00" {
		t.Fatalf("priced catalog = %+v", cat)
	}

	look := decodeData[struct {
		ProductUUID    string  `json:"product_uuid"`
		Barcode        string  `json:"barcode"`
		QuantityOnHand int32   `json:"quantity_on_hand"`
		SalePrice      *string `json:"sale_price"`
		PurchasePrice  *string `json:"purchase_price"`
	}](t, mustDo(t, it, "GET", "/v1/product-sales/lookup?barcode="+url.QueryEscape(u1.Barcode), ownerTok, nil, http.StatusOK))
	if look.ProductUUID != product.Uuid.String() || look.Barcode != u1.Barcode || look.QuantityOnHand != 1 ||
		strv(look.SalePrice) != "1200.00" || strv(look.PurchasePrice) != "800.00" {
		t.Fatalf("lookup = %+v sale=%s purchase=%s", look, strv(look.SalePrice), strv(look.PurchasePrice))
	}
	if code, env := it.do("GET", "/v1/product-sales/lookup?barcode=NOPE-"+it.suffix, hostOlex, ownerTok, nil); code != http.StatusUnprocessableEntity || errCode(env) != "STOCK_UNAVAILABLE" {
		t.Fatalf("unknown barcode lookup = %d %s, want 422 STOCK_UNAVAILABLE", code, errCode(env))
	}

	sale := decodeData[productSaleResp](t, mustDo(t, it, "POST", "/v1/product-sales", ownerTok, map[string]any{
		"payment_method": "cash", "lines": []map[string]any{{"barcode": u1.Barcode, "unit_price": "1200"}},
	}, http.StatusCreated))
	sales := decodeData[listPage[saleListResp]](t, mustDo(t, it, "GET", "/v1/product-sales?payment_method=cash&sort=-total", ownerTok, nil, http.StatusOK))
	if sales.Total != 1 || sales.Items[0].UUID != sale.UUID || sales.Items[0].Total != "1200.00" ||
		strv(sales.Items[0].Profit) != "400.00" || sales.Items[0].Products != product.Name || sales.Items[0].Voided {
		t.Fatalf("sales = %+v", sales)
	}
	if got := decodeData[listPage[saleListResp]](t, mustDo(t, it, "GET", "/v1/product-sales?payment_method=card,cari", ownerTok, nil, http.StatusOK)); got.Total != 0 {
		t.Fatalf("card sales = %+v", got)
	}
	if code, env := it.do("GET", "/v1/product-sales?payment_method=barter", hostOlex, ownerTok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("bad payment filter = %d %s, want 400", code, errCode(env))
	}
	mustDo(t, it, "POST", "/v1/product-sales/"+sale.UUID+"/void", ownerTok, map[string]any{"reason": "iade"}, http.StatusOK)
	if got := decodeData[listPage[saleListResp]](t, mustDo(t, it, "GET", "/v1/product-sales?voided=true&q="+url.QueryEscape(u1.Barcode), ownerTok, nil, http.StatusOK)); got.Total != 1 || !got.Items[0].Voided {
		t.Fatalf("voided sales = %+v", got)
	}

	supA := decodeData[supplierResp](t, mustDo(t, it, "POST", "/v1/suppliers", ownerTok, map[string]any{"name": "A Tedarik " + it.suffix}, http.StatusCreated))
	mustDo(t, it, "POST", "/v1/suppliers", ownerTok, map[string]any{"name": "B Tedarik " + it.suffix, "phone_e164": "+905321112233"}, http.StatusCreated)
	sups := decodeData[listPage[supplierResp]](t, mustDo(t, it, "GET", "/v1/suppliers?sort=-name&limit=1&q="+url.QueryEscape(it.suffix), ownerTok, nil, http.StatusOK))
	if sups.Total != 2 || len(sups.Items) != 1 || sups.Items[0].Name != "B Tedarik "+it.suffix {
		t.Fatalf("suppliers = %+v", sups)
	}
	if got := decodeData[listPage[supplierResp]](t, mustDo(t, it, "GET", "/v1/suppliers?q=905321112233", ownerTok, nil, http.StatusOK)); got.Total != 1 {
		t.Fatalf("supplier phone search = %+v", got)
	}

	mustDo(t, it, "POST", "/v1/purchases", ownerTok, map[string]any{
		"supplier_uuid": supA.UUID, "amount": "250", "payment_method": "card", "description": "Cam suyu",
	}, http.StatusCreated)
	purchases := decodeData[listPage[purchaseListResp]](t, mustDo(t, it, "GET",
		"/v1/purchases?supplier_uuid="+supA.UUID+"&payment_method=card&amount_min=100&q=cam", ownerTok, nil, http.StatusOK))
	if purchases.Total != 1 || purchases.Items[0].SupplierName != supA.Name || purchases.Items[0].Amount != "250.00" ||
		purchases.Items[0].Description != "Cam suyu" {
		t.Fatalf("purchases = %+v", purchases)
	}
	if got := decodeData[listPage[purchaseListResp]](t, mustDo(t, it, "GET", "/v1/purchases?payment_method=cash", ownerTok, nil, http.StatusOK)); got.Total != 0 {
		t.Fatalf("cash purchases = %+v", got)
	}
	if code, _ := it.do("GET", "/v1/purchases", hostOlex, staffTok, nil); code != http.StatusForbidden {
		t.Fatalf("dealer_staff purchases = %d, want 403", code)
	}
}
