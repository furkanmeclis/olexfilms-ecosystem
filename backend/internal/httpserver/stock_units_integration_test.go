package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type stockUnitRowView struct {
	UUID            string  `json:"uuid"`
	Barcode         string  `json:"barcode"`
	Status          string  `json:"status"`
	Quantity        int32   `json:"quantity"`
	RemainingMeters *string `json:"remaining_meters"`
	Product         struct {
		UUID string `json:"uuid"`
	} `json:"product"`
	Location *struct {
		Code string `json:"code"`
	} `json:"location"`
	PurchasePrice *struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
		Source   string `json:"source"`
	} `json:"purchase_price"`
}

type stockUnitRowPage struct {
	Total int64              `json:"total"`
	Items []stockUnitRowView `json:"items"`
}

func (p stockUnitRowPage) barcodes() map[string]stockUnitRowView {
	out := map[string]stockUnitRowView{}
	for _, i := range p.Items {
		out[i.Barcode] = i
	}
	return out
}

func (it *itest) orgUnits(token string, org db.Organization, query string) (int, stockUnitRowPage) {
	it.t.Helper()
	code, env := it.do("GET", "/v1/stock/organizations/"+org.Uuid.String()+"/units"+query, hostOlex, token, nil)
	var p stockUnitRowPage
	if code == http.StatusOK {
		if err := json.Unmarshal(env.Data, &p); err != nil {
			it.t.Fatal(err)
		}
	}
	return code, p
}

// TEC-216 acceptance: the distributor owner lists the units of its dealer
// (stock.read subtree) but not those of a sibling distributor's dealer;
// the distributor cannot split or reclassify a dealer-held unit (403, no
// ledger row); the purchase price is null without pricing.purchase.read.
func TestIntegrationStockUnitsSubtree(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t216-dist", "distributor", center)
	dealer := it.org("t216-dealer", "dealer", dist)
	otherDist := it.org("t216-dist2", "distributor", center)
	otherDealer := it.org("t216-dealer2", "dealer", otherDist)

	distOwner, dPw := it.user("t216-dist-owner")
	it.member(dist, distOwner, "owner")
	distWh, wPw := it.user("t216-dist-wh")
	it.member(dist, distWh, "staff", rbac.RoleDistributorWarehouseStaff)
	otherOwner, oPw := it.user("t216-dist2-owner")
	it.member(otherDist, otherOwner, "owner")
	dealerOwner, rPw := it.user("t216-dealer-owner")
	it.member(dealer, dealerOwner, "owner")

	p := it.product(center, "T216")
	roll := it.product(center, "T216R")
	if _, err := it.pool.Exec(ctx, `UPDATE products SET unit_type = 'roll_meter' WHERE id = $1`, roll.ID); err != nil {
		t.Fatalf("roll product: %v", err)
	}
	// K8: the distributor pays the center list price, the dealer pays the
	// distributor's dealer price.
	it.setListPrice(p, "TRY", "100")
	if _, err := it.q.UpsertDistributorDealerPrice(ctx, db.UpsertDistributorDealerPriceParams{
		ProductID: p.ID, BrandID: p.BrandID, DistributorOrgID: dist.ID, Currency: "TRY", Price: "120",
	}); err != nil {
		t.Fatal(err)
	}

	c := it.stockChain()
	cLoc := c.location(center, "C")
	dLoc := c.location(dist, "D")
	oLoc := c.location(otherDist, "O")
	dealerOrg := ledger.Owner{Type: ledger.OwnerOrganization, ID: dealer.ID, OrgID: dealer.ID}
	otherDealerOrg := ledger.Owner{Type: ledger.OwnerOrganization, ID: otherDealer.ID, OrgID: otherDealer.ID}

	// u1 (piece) and r1 (roll): center -> distributor -> dealer.
	u1 := c.unit(center, p, 2161)
	c.post(ledger.TypeEntry, u1, c.nextRef(), cLoc)
	c.ship(u1, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	c.ship(u1, ledger.TypeOrderOut, ledger.TypeReceived, dealer, dealerOrg)
	r1 := c.rollUnit(center, roll, 2162, "30.00")
	c.post(ledger.TypeEntry, r1, c.nextRef(), cLoc)
	c.ship(r1, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	c.ship(r1, ledger.TypeOrderOut, ledger.TypeReceived, dealer, dealerOrg)
	// u2 stays in the distributor's bin.
	u2 := c.unit(center, p, 2163)
	c.post(ledger.TypeEntry, u2, c.nextRef(), cLoc)
	c.ship(u2, ledger.TypeTransferOut, ledger.TypeTransferIn, dist, dLoc)
	// u3: the sibling distributor's dealer.
	u3 := c.unit(center, p, 2164)
	c.post(ledger.TypeEntry, u3, c.nextRef(), cLoc)
	c.ship(u3, ledger.TypeTransferOut, ledger.TypeTransferIn, otherDist, oLoc)
	c.ship(u3, ledger.TypeOrderOut, ledger.TypeReceived, otherDealer, otherDealerOrg)

	distTok := it.loginOrg(distOwner, dPw, dist)
	whTok := it.loginOrg(distWh, wPw, dist)
	otherTok := it.loginOrg(otherOwner, oPw, otherDist)
	dealerTok := it.loginOrg(dealerOwner, rPw, dealer)

	// 1. The distributor owner sees its dealer's units with the dealer's
	// purchase price (its own dealer price, K8).
	code, page := it.orgUnits(distTok, dealer, "")
	if code != http.StatusOK || page.Total != 2 {
		t.Fatalf("distributor on dealer units = %d total %d", code, page.Total)
	}
	rows := page.barcodes()
	row, ok := rows[u1.Barcode]
	if !ok || row.Status != "available" || row.Quantity != 1 || row.Product.UUID != p.Uuid.String() || row.Location != nil {
		t.Fatalf("dealer unit row = %+v", row)
	}
	if row.PurchasePrice == nil || !sameAmount(&row.PurchasePrice.Amount, "120") || row.PurchasePrice.Currency != "TRY" ||
		row.PurchasePrice.Source != "distributor" {
		t.Fatalf("dealer purchase price = %+v", row.PurchasePrice)
	}
	if r, ok := rows[r1.Barcode]; !ok || r.RemainingMeters == nil || *r.RemainingMeters != "30.00" {
		t.Fatalf("dealer roll row = %+v", r)
	}
	// Its own units carry the bin and the center list price.
	code, page = it.orgUnits(distTok, dist, "?product_uuid="+p.Uuid.String())
	if code != http.StatusOK || page.Total != 1 || page.Items[0].Barcode != u2.Barcode ||
		page.Items[0].Location == nil || page.Items[0].PurchasePrice == nil || !sameAmount(&page.Items[0].PurchasePrice.Amount, "100") ||
		page.Items[0].PurchasePrice.Source != "list" {
		t.Fatalf("distributor own units = %d %+v", code, page)
	}
	// Filters: exact barcode, status, q, paging.
	if _, page = it.orgUnits(distTok, dealer, "?barcode="+u1.Barcode); page.Total != 1 || page.Items[0].Barcode != u1.Barcode {
		t.Fatalf("barcode filter = %+v", page)
	}
	if _, page = it.orgUnits(distTok, dealer, "?status=in_transit"); page.Total != 0 {
		t.Fatalf("status filter = %+v", page)
	}
	if _, page = it.orgUnits(distTok, dealer, "?q="+r1.Barcode); page.Total != 1 || page.Items[0].Barcode != r1.Barcode {
		t.Fatalf("q filter = %+v", page)
	}
	if _, page = it.orgUnits(distTok, dealer, "?limit=1&offset=1"); page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("paging = %+v", page)
	}
	if code, _ = it.orgUnits(distTok, dealer, "?status=bogus"); code != http.StatusBadRequest {
		t.Fatalf("bad status = %d", code)
	}

	// 2. The sibling distributor's dealer is out of reach, both ways.
	if code, _ = it.orgUnits(distTok, otherDealer, ""); code != http.StatusNotFound {
		t.Fatalf("distributor on sibling dealer = %d, want 404", code)
	}
	if code, _ = it.orgUnits(otherTok, dealer, ""); code != http.StatusNotFound {
		t.Fatalf("sibling distributor on dealer = %d, want 404", code)
	}
	if code, page = it.orgUnits(otherTok, otherDealer, ""); code != http.StatusOK || page.Total != 1 || page.Items[0].Barcode != u3.Barcode {
		t.Fatalf("sibling distributor on its dealer = %d %+v", code, page)
	}

	// 3. The distributor cannot write dealer stock: split and
	// reclassification requests answer 403 and write nothing.
	before := it.countRows("SELECT COUNT(*) FROM stock_movements WHERE unit_id = ANY($1::bigint[])", []int64{u1.ID, r1.ID})
	if code, ec := it.status2("POST", "/v1/stock/splits", distTok, map[string]any{
		"barcode": r1.Barcode, "meters": "5", "idempotency_key": "t216-" + it.suffix,
	}); code != http.StatusForbidden {
		t.Fatalf("distributor split of a dealer roll = %d %s, want 403", code, ec)
	}
	if code, ec := it.status2("POST", "/v1/stock/reclassifications", distTok, map[string]any{
		"barcode": u1.Barcode, "to_product_uuid": roll.Uuid.String(), "reason": "wrong label",
	}); code != http.StatusForbidden {
		t.Fatalf("distributor reclassification of a dealer unit = %d %s, want 403", code, ec)
	}
	if after := it.countRows("SELECT COUNT(*) FROM stock_movements WHERE unit_id = ANY($1::bigint[])", []int64{u1.ID, r1.ID}); after != before {
		t.Fatalf("ledger rows %d -> %d after refused writes", before, after)
	}
	if n := it.countRows("SELECT COUNT(*) FROM stock_reclassifications WHERE unit_id = $1", u1.ID); n != 0 {
		t.Fatalf("reclassification rows = %d", n)
	}
	// The dealer owner holds no stock.write at all.
	if code, _ := it.status2("POST", "/v1/stock/splits", dealerTok, map[string]any{
		"barcode": r1.Barcode, "meters": "5", "idempotency_key": "t216b-" + it.suffix,
	}); code != http.StatusForbidden {
		t.Fatalf("dealer split = %d, want 403", code)
	}

	// 4. Without pricing.purchase.read the purchase price is null; the
	// warehouse staff still reaches the dealer's units (subtree).
	code, page = it.orgUnits(whTok, dealer, "")
	if code != http.StatusOK || page.Total != 2 {
		t.Fatalf("warehouse staff on dealer units = %d total %d", code, page.Total)
	}
	for _, r := range page.Items {
		if r.PurchasePrice != nil {
			t.Fatalf("warehouse staff sees a purchase price: %+v", r)
		}
	}
	if _, page = it.orgUnits(whTok, dist, ""); len(page.Items) == 0 || page.Items[0].PurchasePrice != nil {
		t.Fatalf("warehouse staff own units = %+v", page)
	}
	// The dealer owner (pricing.purchase.read managed) sees its own price.
	if code, page = it.orgUnits(dealerTok, dealer, "?barcode="+u1.Barcode); code != http.StatusOK ||
		page.Total != 1 || page.Items[0].PurchasePrice == nil || !sameAmount(&page.Items[0].PurchasePrice.Amount, "120") {
		t.Fatalf("dealer own units = %d %+v", code, page)
	}
}
