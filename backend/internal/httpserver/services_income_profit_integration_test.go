package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type serviceIncomeResult struct {
	Service       serviceView `json:"service"`
	EntryUUID     string      `json:"entry_uuid"`
	ReversalUUIDs []string    `json:"reversal_uuids"`
	Warning       *string     `json:"warning"`
}

type serviceProfitView struct {
	Revenue     *string `json:"revenue"`
	Cost        *string `json:"cost"`
	GrossProfit *string `json:"gross_profit"`
	MarginPct   *string `json:"margin_pct"`
}

// TEC-343 acceptance: service income is posted once, customer cari is
// affected and reversed, profit subtracts the dealer purchase cost, partial
// roll consumption is proportional, and users without pricing.purchase.read
// see null cost fields.
func TestIntegrationServiceIncomeProfit(t *testing.T) {
	c := runF1Chain(t)
	ctx := context.Background()

	if _, err := c.it.pool.Exec(ctx, `UPDATE organizations SET currency = $2 WHERE id = $1`, c.dealer.ID, c.cur); err != nil {
		t.Fatalf("dealer currency: %v", err)
	}
	cash := decodeData[accAccount](t, c.it.accDo("POST", "/v1/accounting/accounts", c.dealerTok, map[string]any{
		"type": "cash", "name": "TEC-343 Kasa " + c.it.suffix,
	}, http.StatusCreated))

	income := decodeData[serviceIncomeResult](t, c.it.accDo("POST", "/v1/services/"+c.svcUUID+"/income", c.dealerTok, map[string]any{
		"amount": "15000", "payment_method": "cari", "description": "Kaplama hizmet geliri",
	}, http.StatusCreated))
	if income.EntryUUID == "" || income.Service.IncomeAmount == nil || !sameAmount(income.Service.IncomeAmount, "15000") {
		t.Fatalf("income result = %+v", income)
	}
	if _, ec := c.it.svcCall("POST", "/v1/services/"+c.svcUUID+"/income", c.dealerTok, map[string]any{
		"amount": "1", "payment_method": "cari",
	}, http.StatusConflict); ec != "SERVICE_INCOME_ALREADY_RECORDED" {
		t.Fatalf("second income error = %s", ec)
	}

	customerCari, err := c.it.q.GetCariAccountByCounterpartyUser(ctx, db.GetCariAccountByCounterpartyUserParams{
		OrganizationID: c.dealer.ID, CounterpartyUserID: c.it.serviceCustomerID(c.svcID),
	})
	if err != nil {
		t.Fatalf("customer cari: %v", err)
	}
	gotCari := decodeData[accCari](t, c.it.accDo("GET", "/v1/accounting/cari/"+customerCari.Uuid.String(), c.dealerTok, nil, http.StatusOK))
	if !sameAmount(&gotCari.Balance, "15000") {
		t.Fatalf("customer cari after service income = %+v", gotCari)
	}

	profit := decodeData[serviceProfitView](t, c.it.accDo("GET", "/v1/services/"+c.svcUUID+"/profit", c.dealerTok, nil, http.StatusOK))
	if !sameAmount(profit.Revenue, "15000") || !sameAmount(profit.Cost, "300") ||
		!sameAmount(profit.GrossProfit, "14700") || !sameAmount(profit.MarginPct, "98") {
		t.Fatalf("profit = %+v", profit)
	}
	page := c.it.svcList(c.dealerTok, "?limit=20")
	var listed *serviceView
	for i := range page.Items {
		if page.Items[i].UUID == c.svcUUID {
			listed = &page.Items[i]
			break
		}
	}
	if listed == nil || listed.IncomeAmount == nil || listed.Profit == nil || !sameAmount(listed.Profit.Cost, "300") {
		t.Fatalf("list service columns = %+v", listed)
	}

	staff, spw := c.it.user("t343-staff")
	c.it.member(c.dealer, staff, "staff", rbac.RoleDealerStaff)
	staffTok := c.it.loginOrg(staff, spw, c.dealer)
	masked := decodeData[serviceProfitView](t, c.it.accDo("GET", "/v1/services/"+c.svcUUID+"/profit", staffTok, nil, http.StatusOK))
	if masked.Cost != nil || masked.GrossProfit != nil || masked.MarginPct != nil {
		t.Fatalf("masked profit = %+v", masked)
	}

	deleted := decodeData[serviceIncomeResult](t, c.it.accDo("DELETE", "/v1/services/"+c.svcUUID+"/income", c.dealerTok,
		map[string]any{"reason": "Gelir iptali"}, http.StatusOK))
	if len(deleted.ReversalUUIDs) != 1 {
		t.Fatalf("income reversal = %+v", deleted)
	}
	gotCari = decodeData[accCari](t, c.it.accDo("GET", "/v1/accounting/cari/"+customerCari.Uuid.String(), c.dealerTok, nil, http.StatusOK))
	if !sameAmount(&gotCari.Balance, "0") {
		t.Fatalf("customer cari after delete income = %+v", gotCari)
	}
	ws := c.it.serviceWarranties(c.svcID, c.dealer.BrandID)
	if len(ws) == 0 {
		t.Fatal("service warranties missing")
	}
	claim, err := c.it.q.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
		OrganizationID: c.dealer.ID, BrandID: c.dealer.BrandID, WarrantyID: ws[0].ID,
		ServiceID: c.svcID, VehicleID: ws[0].VehicleID, CustomerUserID: c.it.serviceCustomerID(c.svcID),
		Description: "TEC-343 claim", Status: "open", CoverageCheck: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	warnSvc := c.it.completedWarningService(c.svcID, claim.ID)
	warned := decodeData[serviceIncomeResult](t, c.it.accDo("POST", "/v1/services/"+warnSvc+"/income", c.dealerTok, map[string]any{
		"amount": "1", "payment_method": "cash", "account_uuid": cash.UUID,
	}, http.StatusCreated))
	if warned.Warning == nil || *warned.Warning != "warranty_claim_service" {
		t.Fatalf("warranty-claim income warning = %+v", warned)
	}

	partialSvc := c.it.completedPartialRollService(c, cash.UUID)
	partialProfit := decodeData[serviceProfitView](t, c.it.accDo("GET", "/v1/services/"+partialSvc+"/profit", c.dealerTok, nil, http.StatusOK))
	if !sameAmount(partialProfit.Revenue, "150") || !sameAmount(partialProfit.Cost, "18") ||
		!sameAmount(partialProfit.GrossProfit, "132") || !sameAmount(partialProfit.MarginPct, "88") {
		t.Fatalf("partial profit = %+v", partialProfit)
	}
}

func (it *itest) serviceCustomerID(serviceID int64) int64 {
	it.t.Helper()
	var id int64
	if err := it.pool.QueryRow(context.Background(), `SELECT customer_user_id FROM services WHERE id = $1`, serviceID).Scan(&id); err != nil {
		it.t.Fatalf("service customer: %v", err)
	}
	return id
}

func (it *itest) completedWarningService(sourceServiceID, claimID int64) string {
	it.t.Helper()
	var out string
	if err := it.pool.QueryRow(context.Background(), `INSERT INTO services (
		service_no, organization_id, brand_id, customer_user_id, vehicle_id,
		car_brand_id, car_model_id, model_year, plate, plate_country, vin, km,
		package, notes, has_measurement, measurement_result_id, contract_id,
		status, completed_at, warranty_claim_id
	)
	SELECT 'DSW' || right(gen_random_uuid()::text, 8), organization_id, brand_id,
		customer_user_id, vehicle_id, car_brand_id, car_model_id, model_year,
		plate, plate_country, vin, km, package, notes, has_measurement,
		measurement_result_id, contract_id, 'completed', NOW(), $2
	FROM services WHERE id = $1
	RETURNING uuid::text`, sourceServiceID, claimID).Scan(&out); err != nil {
		it.t.Fatalf("warning service: %v", err)
	}
	return out
}

func (it *itest) completedPartialRollService(c *f1Chain, cashUUID string) string {
	it.t.Helper()
	ctx := context.Background()
	productUUID, err := uuid.Parse(c.item.UUID)
	if err != nil {
		it.t.Fatalf("product uuid: %v", err)
	}
	product, err := it.q.GetProductByUUIDAnyBrand(ctx, productUUID)
	if err != nil {
		it.t.Fatalf("product: %v", err)
	}
	unit := it.dealerRollFixture(c, product)
	cust, veh := it.svcCustomer(c.dealer, "t343-partial-cust", "34P343"+it.suffix[len(it.suffix)-4:])
	s, _ := it.svcCall("POST", "/v1/services", c.dealerTok,
		map[string]any{"customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}, http.StatusCreated)
	it.svcCall("POST", "/v1/services/"+s.UUID+"/items", c.dealerTok,
		map[string]any{"barcode": unit.Barcode, "kind": "partial", "meters": "3"}, http.StatusCreated)
	done, _ := it.svcCall("POST", "/v1/services/"+s.UUID+"/transitions", c.dealerTok,
		map[string]string{"status": "completed"}, http.StatusOK)
	if done.Status != "completed" {
		it.t.Fatalf("partial service = %+v", done)
	}
	decodeData[serviceIncomeResult](it.t, it.accDo("POST", "/v1/services/"+s.UUID+"/income", c.dealerTok, map[string]any{
		"amount": "150", "payment_method": "cash", "account_uuid": cashUUID,
	}, http.StatusCreated))
	return s.UUID
}

func (it *itest) dealerRollFixture(c *f1Chain, product db.Product) db.Unit {
	it.t.Helper()
	ctx := context.Background()
	var meters pgtype.Numeric
	if err := meters.Scan("25.00"); err != nil {
		it.t.Fatal(err)
	}
	unit, err := it.q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: c.center.ID, BrandID: c.center.BrandID, ProductID: product.ID,
		Barcode: "T343-" + it.suffix, UnitKind: ledger.KindSerial, Source: "generated",
		Status: string(ledger.StatusPrinted), InitialMeters: meters, RemainingMeters: meters,
	})
	if err != nil {
		it.t.Fatalf("partial unit: %v", err)
	}
	sc := it.stockChain()
	centerLoc := sc.location(c.center, "C343")
	distLoc := sc.location(c.dist, "D343")
	sc.post(ledger.TypeEntry, unit, sc.nextRef(), centerLoc)
	sc.ship(unit, ledger.TypeTransferOut, ledger.TypeTransferIn, c.dist, distLoc)
	sc.ship(unit, ledger.TypeOrderOut, ledger.TypeReceived, c.dealer, ledger.Owner{Type: ledger.OwnerOrganization, ID: c.dealer.ID, OrgID: c.dealer.ID})

	var orderID, itemID int64
	if err := it.pool.QueryRow(ctx, `INSERT INTO orders (
		order_no, organization_id, brand_id, seller_org_id, buyer_org_id, status, currency, subtotal, total
	) VALUES ($1, $2, $3, $2, $4, 'received', $5, 150.00, 150.00) RETURNING id`,
		"TEC343-"+it.suffix, c.dist.ID, c.dealer.BrandID, c.dealer.ID, c.cur).Scan(&orderID); err != nil {
		it.t.Fatalf("partial order: %v", err)
	}
	if err := it.pool.QueryRow(ctx, `INSERT INTO order_items (
		order_id, organization_id, brand_id, product_id, meters, unit_price, price_source, line_total
	) VALUES ($1, $2, $3, $4, 25.00, 6.00, 'distributor_dealer', 150.00) RETURNING id`,
		orderID, c.dist.ID, c.dealer.BrandID, product.ID).Scan(&itemID); err != nil {
		it.t.Fatalf("partial order item: %v", err)
	}
	if _, err := it.pool.Exec(ctx, `INSERT INTO order_item_units (
		order_item_id, organization_id, brand_id, unit_id, meters
	) VALUES ($1, $2, $3, $4, 25.00)`, itemID, c.dist.ID, c.dealer.BrandID, unit.ID); err != nil {
		it.t.Fatalf("partial order unit: %v", err)
	}
	return unit
}
