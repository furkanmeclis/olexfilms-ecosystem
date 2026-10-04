package db_test

import (
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-334: database-level guards of the warranty claim schema (migration
// 000092). Reuses the service fixture (center > distributor > dealer and a
// sibling dealer; rolled-back transaction, savepoint per failure).

func (f *serviceFixture) claimWarranty(t *testing.T) db.Warranty {
	t.Helper()
	piece := f.unit(t, f.piece, "")
	_, items := f.completedService(t, db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: piece.ID, Kind: "full"})
	w, err := f.q.CreateWarrantyForServiceItem(f.ctx, f.warrantyParams(items[0]))
	if err != nil {
		t.Fatalf("warranty: %v", err)
	}
	return w
}

func claimParams(org db.Organization, w db.Warranty) db.CreateWarrantyClaimParams {
	return db.CreateWarrantyClaimParams{
		OrganizationID: org.ID, BrandID: org.BrandID, WarrantyID: w.ID, ServiceID: w.ServiceID,
		VehicleID: w.VehicleID, CustomerUserID: w.HolderUserID, Description: "Film kalkti",
		Status: "open", CoverageCheck: []byte(`{}`),
	}
}

func (f *serviceFixture) createClaim(arg db.CreateWarrantyClaimParams) func(sp pgx.Tx) error {
	return func(sp pgx.Tx) error {
		_, err := db.New(sp).CreateWarrantyClaim(f.ctx, arg)
		return err
	}
}

func TestWarrantyClaimSchemaConstraints(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx

	t.Run("events are append-only", func(t *testing.T) {
		w := f.claimWarranty(t)
		c, err := f.q.CreateWarrantyClaim(ctx, claimParams(f.dealer, w))
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if _, err := f.q.SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
			ID: c.ID, BrandID: c.BrandID, FromStatus: "open", Status: "dealer_review",
		}); err != nil {
			t.Fatalf("status: %v", err)
		}
		events, err := f.q.ListWarrantyClaimEvents(ctx, c.ID)
		if err != nil || len(events) != 2 || events[0].EventType != "created" ||
			events[1].EventType != "status_changed" || events[1].FromStatus.String != "open" ||
			events[1].ToStatus.String != "dealer_review" {
			t.Fatalf("events = %+v, err %v", events, err)
		}
		f.expectCode(t, "update event", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE warranty_claim_events SET payload = '{}'::jsonb WHERE id = $1`, events[0].ID)
			return err
		}, "23001")
		f.expectCode(t, "delete event", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `DELETE FROM warranty_claim_events WHERE id = $1`, events[0].ID)
			return err
		}, "23001")
	})

	t.Run("one live claim per warranty", func(t *testing.T) {
		w := f.claimWarranty(t)
		first, err := f.q.CreateWarrantyClaim(ctx, claimParams(f.dealer, w))
		if err != nil {
			t.Fatalf("first claim: %v", err)
		}
		f.expectConstraint(t, "second open claim", "23505", "uq_warranty_claims_live_warranty",
			f.createClaim(claimParams(f.dealer, w)))
		// The parent distributor is blocked by the same index.
		f.expectConstraint(t, "second claim by the distributor", "23505", "uq_warranty_claims_live_warranty",
			f.createClaim(claimParams(f.dist, w)))
		f.expectConstraint(t, "rejected without reason", "23514", "chk_warranty_claims_rejection", func(sp pgx.Tx) error {
			_, err := db.New(sp).SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
				ID: first.ID, BrandID: first.BrandID, FromStatus: "open", Status: "rejected",
			})
			return err
		})
		if _, err := f.q.SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
			ID: first.ID, BrandID: first.BrandID, FromStatus: "open", Status: "rejected",
			RejectionReason: pgtype.Text{String: "Kapsam disi", Valid: true},
		}); err != nil {
			t.Fatalf("reject: %v", err)
		}
		second, err := f.q.CreateWarrantyClaim(ctx, claimParams(f.dealer, w))
		if err != nil {
			t.Fatalf("claim after rejection: %v", err)
		}
		if second.ClaimNo != first.ClaimNo+1 {
			t.Fatalf("claim_no = %d, want %d", second.ClaimNo, first.ClaimNo+1)
		}
	})

	t.Run("claim only by the service organization or its distributor", func(t *testing.T) {
		w := f.claimWarranty(t)
		otherDist := f.org(t, "dist-other", "distributor", f.centerID)
		for _, org := range []db.Organization{f.dealer2, otherDist} {
			f.expectTrigger(t, "claim by "+org.Name, f.createClaim(claimParams(org, w)))
		}
		arg := claimParams(f.dealer, w)
		arg.ServiceID = f.service(t, f.dealer).ID
		f.expectTrigger(t, "service differs from warranty", f.createClaim(arg))
		f.expectOK(t, "claim by the parent distributor", f.createClaim(claimParams(f.dist, w)))
		f.expectOK(t, "claim by the service organization", f.createClaim(claimParams(f.dealer, w)))
	})

	t.Run("re-application service link", func(t *testing.T) {
		w := f.claimWarranty(t)
		c, err := f.q.CreateWarrantyClaim(ctx, claimParams(f.dealer, w))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.q.SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
			ID: c.ID, BrandID: c.BrandID, FromStatus: "open", Status: "approved",
		}); err != nil {
			t.Fatalf("approve: %v", err)
		}
		svc := f.service(t, f.dealer)
		got, err := f.q.LinkWarrantyClaimReapplyService(ctx, db.LinkWarrantyClaimReapplyServiceParams{
			ID: c.ID, BrandID: c.BrandID, ReapplyServiceID: pgtype.Int8{Int64: svc.ID, Valid: true},
		})
		if err != nil || got.Status != "reapplied" || !got.DecidedAt.Valid {
			t.Fatalf("link: %+v %v", got, err)
		}
		if _, err := f.q.SetServiceWarrantyClaim(ctx, db.SetServiceWarrantyClaimParams{
			ID: svc.ID, BrandID: svc.BrandID, WarrantyClaimID: pgtype.Int8{Int64: c.ID, Valid: true},
		}); err != nil {
			t.Fatalf("service link: %v", err)
		}
		// A new claim is possible once the claim is closed.
		if _, err := f.q.SetWarrantyClaimStatus(ctx, db.SetWarrantyClaimStatusParams{
			ID: c.ID, BrandID: c.BrandID, FromStatus: "reapplied", Status: "closed",
		}); err != nil {
			t.Fatalf("close: %v", err)
		}
		f.expectTrigger(t, "closed is final", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE warranty_claims SET status = 'open', closed_at = NULL WHERE id = $1`, c.ID)
			return err
		})
		f.expectOK(t, "new claim after close", f.createClaim(claimParams(f.dealer, w)))
	})
}
