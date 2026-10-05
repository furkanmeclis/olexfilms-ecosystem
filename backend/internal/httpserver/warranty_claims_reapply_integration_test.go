package httpserver

import (
	"context"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-336: the HTTP server event bus must run the warranty claim listener too:
// approved claims open one re-application service, and completing it closes the
// claim.
func TestIntegrationWarrantyClaimReapplyServerEvents(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t336-dealer", "dealer", center)
	cust, veh := it.svcCustomer(dealer, "t336-cust", "34R336"+it.suffix[len(it.suffix)-4:])
	product := it.product(center, "T336R")
	if _, err := it.pool.Exec(ctx, `UPDATE product_categories SET available_parts = '["body_kaput"]'::jsonb WHERE id = $1`, product.CategoryID); err != nil {
		t.Fatalf("product parts: %v", err)
	}
	it.setWarrantyMonths(product, 12)
	unit := it.stockChain().unit(center, product, 33601)
	original := it.directService(dealer, cust, veh, 33601, db.CreateServiceItemParams{
		ProductID: product.ID, UnitID: unit.ID, Kind: "full", AppliedParts: []byte(`["body_kaput"]`),
	})
	listenerWarranty := it.serviceWarranties(original.ID, original.BrandID)
	if len(listenerWarranty) != 0 {
		t.Fatalf("fixture service unexpectedly has warranties before listener: %d", len(listenerWarranty))
	}
	if err := it.srv.events.Publish(ctx, serviceEvent(events.ServiceCompleted, original)); err != nil {
		t.Fatalf("publish original completed: %v", err)
	}
	warranties := it.serviceWarranties(original.ID, original.BrandID)
	if len(warranties) != 1 {
		t.Fatalf("warranties = %d, want 1", len(warranties))
	}
	items, err := it.q.ListServiceItems(ctx, original.ID)
	if err != nil {
		t.Fatalf("service items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("service items = %d, want 1", len(items))
	}
	claim, err := it.q.CreateWarrantyClaim(ctx, db.CreateWarrantyClaimParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, WarrantyID: warranties[0].ID,
		ServiceID: original.ID, VehicleID: original.VehicleID, CustomerUserID: original.CustomerUserID,
		Description: "TEC-336 yeniden uygulama", Status: "open", CoverageCheck: []byte(`{}`),
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
	if err != nil {
		t.Fatalf("reload claim: %v", err)
	}
	if got.Status != "reapplied" || !got.ReapplyServiceID.Valid {
		t.Fatalf("claim after approved event = %+v, want reapplied with service", got)
	}
	reapply, err := it.q.GetService(ctx, db.GetServiceParams{ID: got.ReapplyServiceID.Int64, BrandID: got.BrandID})
	if err != nil {
		t.Fatalf("reapply service: %v", err)
	}
	completed, err := it.q.CompleteService(ctx, db.CompleteServiceParams{ID: reapply.ID})
	if err != nil {
		t.Fatalf("complete reapply service: %v", err)
	}
	if err := it.srv.events.Publish(ctx, serviceEvent(events.ServiceCompleted, completed)); err != nil {
		t.Fatalf("publish reapply completed: %v", err)
	}
	closed, err := it.q.GetWarrantyClaimByID(ctx, db.GetWarrantyClaimByIDParams{ID: claim.ID, BrandID: claim.BrandID})
	if err != nil {
		t.Fatalf("reload closed claim: %v", err)
	}
	if closed.Status != "closed" {
		t.Fatalf("claim status = %s, want closed", closed.Status)
	}
}

func serviceEvent(name string, svc db.Service) events.Event {
	id, uid := svc.ID, svc.Uuid
	return events.New(name).WithTenant(svc.OrganizationID).
		WithEntity("service", &id, &uid).
		WithPayload(map[string]any{"service_id": svc.ID, "service_uuid": uid.String(), "brand_id": svc.BrandID})
}

func claimStatusEvent(claim db.WarrantyClaim, from, to string) events.Event {
	id, uid := claim.ID, claim.Uuid
	return events.New(events.WarrantyClaimStatusChanged).WithTenant(claim.OrganizationID).
		WithEntity("warranty_claim", &id, &uid).
		WithPayload(map[string]any{
			"claim_id": claim.ID, "claim_uuid": uid.String(), "brand_id": claim.BrandID,
			"from": from, "to": to,
		})
}
