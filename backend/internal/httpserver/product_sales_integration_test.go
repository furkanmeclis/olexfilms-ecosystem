package httpserver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/jackc/pgx/v5/pgtype"
)

type dealerPriceResp struct {
	ProductUUID          string  `json:"product_uuid"`
	SalePrice            string  `json:"sale_price"`
	Currency             string  `json:"currency"`
	RecommendedSalePrice *string `json:"recommended_sale_price"`
}

type productSaleResp struct {
	UUID   string `json:"uuid"`
	Total  string `json:"total"`
	Profit string `json:"profit"`
	Voided bool   `json:"voided"`
	Lines  []struct {
		LineTotal        string  `json:"line_total"`
		PurchaseUnitCost *string `json:"purchase_unit_cost"`
		Profit           *string `json:"profit"`
	} `json:"lines"`
}

type supplierResp struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

func TestIntegrationProductSalesAndPurchases(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t344-dist", "distributor", center)
	dealer := it.org("t344-dealer", "dealer", dist)

	owner, opw := it.user("t344-owner")
	it.member(dealer, owner, "owner")
	staff, spw := it.user("t344-staff")
	it.member(dealer, staff, "staff")
	ownerTok := it.loginOrg(owner, opw, dealer)
	staffTok := it.loginOrg(staff, spw, dealer)

	product := it.product(center, "T344P")
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
	u1 := chain.unit(center, product, 34401)
	centerOwner := ledger.Owner{Type: ledger.OwnerOrganization, ID: center.ID, OrgID: center.ID}
	dealerOwner := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID}
	chain.post(ledger.TypeEntry, u1, chain.nextRef(), centerOwner)
	chain.ship(u1, ledger.TypeTransferOut, ledger.TypeTransferIn, dealer, dealerOwner)

	if code, _ := it.do("PUT", "/v1/dealer-prices", hostOlex, staffTok, map[string]any{
		"product_uuid": product.Uuid.String(), "sale_price": "1200",
	}); code != http.StatusForbidden {
		t.Fatalf("dealer_staff dealer price = %d, want 403", code)
	}
	price := decodeData[dealerPriceResp](t, mustDo(t, it, "PUT", "/v1/dealer-prices", ownerTok, map[string]any{
		"product_uuid": product.Uuid.String(), "sale_price": "1200",
	}, http.StatusOK))
	if price.SalePrice != "1200.00" || price.RecommendedSalePrice == nil || *price.RecommendedSalePrice != "1500.00" {
		t.Fatalf("dealer price = %+v", price)
	}

	stockBefore := orgProductQty(t, it, dealer.ID, product.ID)
	saleCode, saleEnv := it.do("POST", "/v1/product-sales", hostOlex, ownerTok, map[string]any{
		"payment_method": "cash",
		"lines": []map[string]any{{
			"barcode": u1.Barcode, "unit_price": "1200",
		}},
	})
	if saleCode != http.StatusCreated {
		var st, ot string
		var oid, holder int64
		_ = it.pool.QueryRow(ctx, `SELECT status, owner_type, owner_id, holder_org_id FROM unit_current_state WHERE unit_id = $1`, u1.ID).Scan(&st, &ot, &oid, &holder)
		msg := ""
		if saleEnv.Error != nil {
			msg = saleEnv.Error.Message
		}
		t.Fatalf("POST /v1/product-sales = %d %s %q, state=%s %s %d holder %d dealer %d barcode %s", saleCode, errCode(saleEnv), msg, st, ot, oid, holder, dealer.ID, u1.Barcode)
	}
	sale := decodeData[productSaleResp](t, saleEnv)
	if sale.Total != "1200.00" || sale.Profit != "400.00" || len(sale.Lines) != 1 ||
		sale.Lines[0].PurchaseUnitCost == nil || *sale.Lines[0].PurchaseUnitCost != "800.00" ||
		sale.Lines[0].Profit == nil || *sale.Lines[0].Profit != "400.00" {
		t.Fatalf("sale = %+v", sale)
	}
	if got := orgProductQty(t, it, dealer.ID, product.ID); got != stockBefore-1 {
		t.Fatalf("stock after sale = %d, want %d", got, stockBefore-1)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM finance_entries WHERE organization_id = $1 AND source_type = 'product_sale' AND source_uuid = $2`, dealer.ID, sale.UUID); n != 1 {
		t.Fatalf("product sale finance entries = %d, want 1", n)
	}
	if code, env := it.do("POST", "/v1/product-sales", hostOlex, ownerTok, map[string]any{
		"payment_method": "cash",
		"lines":          []map[string]any{{"barcode": u1.Barcode, "unit_price": "1200"}},
	}); code != http.StatusUnprocessableEntity || errCode(env) != "STOCK_UNAVAILABLE" {
		t.Fatalf("resell consumed barcode = %d %s, want 422 STOCK_UNAVAILABLE", code, errCode(env))
	}

	voided := decodeData[productSaleResp](t, mustDo(t, it, "POST", "/v1/product-sales/"+sale.UUID+"/void", ownerTok,
		map[string]any{"reason": "müşteri vazgeçti"}, http.StatusOK))
	if !voided.Voided {
		t.Fatalf("voided sale = %+v", voided)
	}
	if got := orgProductQty(t, it, dealer.ID, product.ID); got != stockBefore {
		t.Fatalf("stock after void = %d, want %d", got, stockBefore)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM finance_entries WHERE organization_id = $1 AND source_type = 'product_sale' AND reversal_of_id IS NOT NULL`, dealer.ID); n != 1 {
		t.Fatalf("product sale reversals = %d, want 1", n)
	}

	sale2 := decodeData[productSaleResp](t, mustDo(t, it, "POST", "/v1/product-sales", ownerTok, map[string]any{
		"payment_method": "cash",
		"lines":          []map[string]any{{"barcode": u1.Barcode, "unit_price": "1200"}},
	}, http.StatusCreated))
	if _, err := it.pool.Exec(ctx, `UPDATE product_sales SET sold_at = $1 WHERE uuid = $2`, time.Now().Add(-25*time.Hour), sale2.UUID); err != nil {
		t.Fatalf("age sale: %v", err)
	}
	if code, env := it.do("POST", "/v1/product-sales/"+sale2.UUID+"/void", hostOlex, ownerTok,
		map[string]any{"reason": "geç"}); code != http.StatusUnprocessableEntity || errCode(env) != "VOID_WINDOW_EXPIRED" {
		t.Fatalf("next day void = %d %s, want 422 VOID_WINDOW_EXPIRED", code, errCode(env))
	}

	supplier := decodeData[supplierResp](t, mustDo(t, it, "POST", "/v1/suppliers", ownerTok, map[string]any{
		"name": "Dış Tedarikçi " + it.suffix,
	}, http.StatusCreated))
	stockBeforePurchase := orgProductQty(t, it, dealer.ID, product.ID)
	purchase := decodeData[struct {
		UUID   string `json:"uuid"`
		Amount string `json:"amount"`
	}](t, mustDo(t, it, "POST", "/v1/purchases", ownerTok, map[string]any{
		"supplier_uuid": supplier.UUID, "amount": "250", "payment_method": "cash", "description": "Cam suyu",
	}, http.StatusCreated))
	if purchase.Amount != "250.00" {
		t.Fatalf("purchase = %+v", purchase)
	}
	if got := orgProductQty(t, it, dealer.ID, product.ID); got != stockBeforePurchase {
		t.Fatalf("stock after purchase = %d, want unchanged %d", got, stockBeforePurchase)
	}
	if n := it.countRows(`SELECT COUNT(*) FROM finance_entries WHERE organization_id = $1 AND source_type = 'purchase_external' AND source_uuid = $2 AND direction = 'expense'`, dealer.ID, purchase.UUID); n != 1 {
		t.Fatalf("purchase expense entries = %d, want 1", n)
	}
}

func mustDo(t *testing.T, it *itest, method, path, token string, body any, want int) envelope {
	t.Helper()
	code, env := it.do(method, path, hostOlex, token, body)
	if code != want {
		msg := ""
		if env.Error != nil {
			msg = env.Error.Message
		}
		t.Fatalf("%s %s = %d %s %q, want %d", method, path, code, errCode(env), msg, want)
	}
	return env
}

func txt(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

func orgProductQty(t *testing.T, it *itest, orgID, productID int64) int32 {
	t.Helper()
	var qty int32
	if err := it.pool.QueryRow(context.Background(), `
		WITH serial AS (
			SELECT COUNT(*)::int AS qty
			FROM unit_current_state s
			JOIN units u ON u.id = s.unit_id
			WHERE s.holder_org_id = $1
			  AND u.product_id = $2
			  AND s.status IN ('available', 'placed')
			  AND s.owner_type IN ('organization', 'warehouse_location')
		), fixed AS (
			SELECT COALESCE(SUM(h.quantity_on_hand), 0)::int AS qty
			FROM fixed_barcode_holdings h
			JOIN units u ON u.id = h.unit_id
			WHERE h.holder_org_id = $1
			  AND u.product_id = $2
			  AND h.owner_type IN ('organization', 'warehouse_location')
		)
		SELECT serial.qty + fixed.qty FROM serial, fixed`, orgID, productID).Scan(&qty); err != nil {
		t.Fatalf("stock qty: %v", err)
	}
	return qty
}
