package httpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-337: the HTTP server event bus books a closed claim (center product
// cost, warranty_cost) and GET /v1/warranty-claims/{uuid} shows the center
// its cost summary.
func TestIntegrationWarrantyClaimAccountingServer(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t337-dealer", "dealer", center)
	cust, veh := it.svcCustomer(dealer, "t337-cust", "34R337"+it.suffix[len(it.suffix)-4:])
	product := it.product(center, "T337R")
	if _, err := it.pool.Exec(ctx, `UPDATE product_categories SET available_parts = '["body_kaput"]'::jsonb WHERE id = $1`, product.CategoryID); err != nil {
		t.Fatalf("product parts: %v", err)
	}
	if _, err := it.q.UpsertProductPrice(ctx, db.UpsertProductPriceParams{
		ProductID: product.ID, BrandID: product.BrandID, Currency: "TRY",
		PurchasePrice: pgtype.Text{String: "123.45", Valid: true},
	}); err != nil {
		t.Fatalf("price: %v", err)
	}
	it.setWarrantyMonths(product, 12)
	unit := it.stockChain().unit(center, product, 33701)
	original := it.directService(dealer, cust, veh, 33701, db.CreateServiceItemParams{
		ProductID: product.ID, UnitID: unit.ID, Kind: "full", AppliedParts: []byte(`["body_kaput"]`),
	})
	if err := it.srv.events.Publish(ctx, serviceEvent(events.ServiceCompleted, original)); err != nil {
		t.Fatalf("publish original completed: %v", err)
	}
	warranties := it.serviceWarranties(original.ID, original.BrandID)
	items, err := it.q.ListServiceItems(ctx, original.ID)
	if err != nil || len(warranties) != 1 || len(items) != 1 {
		t.Fatalf("warranties %d, items %d, err %v", len(warranties), len(items), err)
	}
	claim, err := it.q.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, WarrantyID: warranties[0].ID,
		ServiceID: original.ID, VehicleID: original.VehicleID, CustomerUserID: original.CustomerUserID,
		Description: "TEC-337 gider", Status: "open", CoverageCheck: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := it.q.AddWarrantyClaimPart(ctx, db.AddWarrantyClaimPartParams{
		ClaimID: claim.ID, OrganizationID: claim.OrganizationID, BrandID: claim.BrandID,
		PartKey: "body_kaput", ServiceItemID: pgtype.Int8{Int64: items[0].ID, Valid: true},
		ProductID: pgtype.Int8{Int64: product.ID, Valid: true}, UnitID: pgtype.Int8{Int64: unit.ID, Valid: true},
	}); err != nil {
		t.Fatalf("claim part: %v", err)
	}
	approved, err := it.q.SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
		ID: claim.ID, BrandID: claim.BrandID, FromStatus: "open", Status: "approved",
	})
	if err != nil {
		t.Fatalf("approve claim: %v", err)
	}
	if err := it.srv.events.Publish(ctx, claimStatusEvent(approved, "open", "approved")); err != nil {
		t.Fatalf("publish claim approved: %v", err)
	}
	got, err := it.q.GetWarrantyClaimByID(ctx, db.GetWarrantyClaimByIDParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil || !got.ReapplyServiceID.Valid {
		t.Fatalf("claim after approved = %+v, %v", got, err)
	}
	completed, err := it.q.CompleteService(ctx, db.CompleteServiceParams{ID: got.ReapplyServiceID.Int64})
	if err != nil {
		t.Fatalf("complete reapply service: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := it.srv.events.Publish(ctx, serviceEvent(events.ServiceCompleted, completed)); err != nil {
			t.Fatalf("publish reapply completed: %v", err)
		}
	}
	var n int
	var amount string
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*), COALESCE(MAX(amount)::text, '') FROM finance_entries
WHERE source_type = 'warranty_claim' AND source_uuid = $1 AND organization_id = $2 AND category = 'warranty_cost'
  AND account_id IS NULL AND cari_id IS NULL`, claim.Uuid, center.ID).Scan(&n, &amount); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if n != 1 || amount != "123.45" {
		t.Fatalf("warranty_cost rows = %d (%s), want one of 123.45", n, amount)
	}

	staff, pw := it.user("t337-acc")
	it.member(center, staff, "staff", rbac.RoleCenterStaff, rbac.RoleCenterAccounting)
	code, env := it.do("GET", "/v1/warranty-claims/"+claim.Uuid.String(), hostOlex, it.loginOrg(staff, pw, center), nil)
	if code != 200 {
		t.Fatalf("GET claim: %d %s", code, errCode(env))
	}
	var view struct {
		Status      string `json:"status"`
		CostSummary *struct {
			ProductCost string `json:"product_cost"`
			Labor       string `json:"labor"`
			Currency    string `json:"currency"`
		} `json:"cost_summary"`
	}
	if err := json.Unmarshal(env.Data, &view); err != nil {
		t.Fatalf("claim payload: %s", env.Data)
	}
	if view.Status != "closed" || view.CostSummary == nil || view.CostSummary.ProductCost != "123.45" ||
		view.CostSummary.Labor != "0.00" || view.CostSummary.Currency != "TRY" {
		t.Fatalf("claim view = %s", env.Data)
	}

	// TEC-382: cancelling the completed re-application service reverses the
	// claim rows through the server bus (twice: one reversal per row) and the
	// cost summary nets to zero.
	cancelled, err := it.q.CancelCompletedService(ctx, db.CancelCompletedServiceParams{
		ID: completed.ID, CancelReason: pgtype.Text{String: "TEC-382 iptal", Valid: true},
	})
	if err != nil {
		t.Fatalf("cancel reapply service: %v", err)
	}
	cancelEv := serviceEvent(events.ServiceCancelled, cancelled)
	delete(cancelEv.Payload, "service_id") // the cancel-completed payload has none
	cancelEv.Payload["from_status"], cancelEv.Payload["reason"] = "completed", "TEC-382 iptal"
	for i := 0; i < 2; i++ {
		if err := it.srv.events.Publish(ctx, cancelEv); err != nil {
			t.Fatalf("publish reapply cancelled: %v", err)
		}
	}
	var originals, reversals int
	var net string
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE reversal_of_id IS NULL),
       COUNT(*) FILTER (WHERE reversal_of_id IS NOT NULL), SUM(amount)::text
FROM finance_entries WHERE source_type = 'warranty_claim' AND source_uuid = $1`, claim.Uuid).Scan(&originals, &reversals, &net); err != nil {
		t.Fatalf("reversal rows: %v", err)
	}
	if originals == 0 || reversals != originals || net != "0.00" {
		t.Fatalf("originals %d, reversals %d, net %s; want one reversal per row, net 0", originals, reversals, net)
	}
	code, env = it.do("GET", "/v1/warranty-claims/"+claim.Uuid.String(), hostOlex, it.loginOrg(staff, pw, center), nil)
	if code != 200 {
		t.Fatalf("GET claim after cancel: %d %s", code, errCode(env))
	}
	view.CostSummary = nil
	if err := json.Unmarshal(env.Data, &view); err != nil {
		t.Fatalf("claim payload: %s", env.Data)
	}
	if view.CostSummary == nil || view.CostSummary.ProductCost != "0.00" || view.CostSummary.Labor != "0.00" {
		t.Fatalf("claim view after cancel = %s", env.Data)
	}
}
