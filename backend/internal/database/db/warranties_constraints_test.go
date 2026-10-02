package db_test

import (
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-185: database-level guards of the warranty schema (migration 000051).
// Reuses the service fixture (rolled-back transaction, savepoint per
// failure).

var publicCodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{12,32}$`)

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// completedService creates a service with the given items and completes it
// (items are locked afterwards, so they are written first).
func (f *serviceFixture) completedService(t *testing.T, items ...db.CreateServiceItemParams) (db.Service, []db.ServiceItem) {
	t.Helper()
	svc := f.service(t, f.dealer)
	out := make([]db.ServiceItem, 0, len(items))
	for _, arg := range items {
		arg.ServiceID = svc.ID
		if arg.AppliedParts == nil {
			arg.AppliedParts = []byte(`[]`)
		}
		it, err := f.q.CreateServiceItem(f.ctx, arg)
		if err != nil {
			t.Fatalf("service item: %v", err)
		}
		out = append(out, it)
	}
	svc, err := f.q.CompleteService(f.ctx, db.CompleteServiceParams{ID: svc.ID})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	return svc, out
}

func (f *serviceFixture) warrantyParams(item db.ServiceItem) db.CreateWarrantyForServiceItemParams {
	start := time.Now().Add(-time.Hour)
	return db.CreateWarrantyForServiceItemParams{
		ServiceItemID: item.ID, HolderUserID: f.customer.ID,
		StartAt: ts(start), EndAt: ts(start.AddDate(1, 0, 0)),
	}
}

func (f *serviceFixture) createWarranty(arg db.CreateWarrantyForServiceItemParams) func(sp pgx.Tx) error {
	return func(sp pgx.Tx) error {
		_, err := db.New(sp).CreateWarrantyForServiceItem(f.ctx, arg)
		return err
	}
}

func TestWarrantySchemaConstraints(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx

	t.Run("service_item_id unique", func(t *testing.T) {
		piece := f.unit(t, f.piece, "")
		_, items := f.completedService(t, db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: piece.ID, Kind: "full"})
		w, err := f.q.CreateWarrantyForServiceItem(ctx, f.warrantyParams(items[0]))
		if err != nil {
			t.Fatalf("warranty: %v", err)
		}
		if w.Status != "active" || w.ItemKind != "full" || w.VehicleID != f.vehicle.ID ||
			w.OrganizationID != f.dealer.ID || w.BrandID != f.dealer.BrandID || w.UnitID != piece.ID {
			t.Fatalf("warranty = %+v", w)
		}
		if !publicCodeRe.MatchString(w.PublicCode) {
			t.Fatalf("public_code %q is not URL-safe or shorter than 12", w.PublicCode)
		}
		// A replayed service.completed event creates nothing (idempotency).
		if _, err := f.q.CreateWarrantyForServiceItem(ctx, f.warrantyParams(items[0])); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("replay: want no row, got %v", err)
		}
		// A plain insert of the same item hits the unique constraint.
		f.expectConstraint(t, "duplicate service item", "23505", "uq_warranties_service_item", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO warranties
				(organization_id, brand_id, service_id, service_item_id, product_id, unit_id, item_kind,
				 vehicle_id, holder_user_id, start_at, end_at)
				VALUES ($1, $2, $3, $4, $5, $6, 'full', $7, $8, NOW(), NOW() + INTERVAL '1 year')`,
				w.OrganizationID, w.BrandID, w.ServiceID, w.ServiceItemID, w.ProductID, w.UnitID,
				w.VehicleID, w.HolderUserID)
			return err
		})
		// Duplicate public code.
		u := f.unit(t, f.piece, "")
		_, its := f.completedService(t, db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: u.ID, Kind: "full"})
		it := its[0]
		f.expectConstraint(t, "duplicate public code", "23505", "uq_warranties_public_code", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO warranties
				(public_code, organization_id, brand_id, service_id, service_item_id, product_id, unit_id, item_kind,
				 vehicle_id, holder_user_id, start_at, end_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, 'full', $8, $9, NOW(), NOW() + INTERVAL '1 year')`,
				w.PublicCode, it.OrganizationID, it.BrandID, it.ServiceID, it.ID, it.ProductID, it.UnitID,
				f.vehicle.ID, f.customer.ID)
			return err
		})
	})

	t.Run("second active warranty of a full unit", func(t *testing.T) {
		piece := f.unit(t, f.piece, "")
		full := db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: piece.ID, Kind: "full"}
		_, first := f.completedService(t, full)
		_, second := f.completedService(t, full)
		w, err := f.q.CreateWarrantyForServiceItem(ctx, f.warrantyParams(first[0]))
		if err != nil {
			t.Fatalf("first warranty: %v", err)
		}
		f.expectConstraint(t, "second active full", "23505", "uq_warranties_active_full_unit",
			f.createWarranty(f.warrantyParams(second[0])))
		// Once the first one is void, the unit may be covered again.
		if _, err := f.q.VoidWarranty(ctx, db.VoidWarrantyParams{ID: w.ID, BrandID: w.BrandID, VoidReason: text("test")}); err != nil {
			t.Fatalf("void: %v", err)
		}
		f.expectOK(t, "after void", f.createWarranty(f.warrantyParams(second[0])))
	})

	t.Run("partial cuts are outside the full-unit rule", func(t *testing.T) {
		roll := f.unit(t, f.film, "15")
		cut := db.CreateServiceItemParams{ProductID: f.film.ID, UnitID: roll.ID, Kind: "partial", Meters: numeric(t, "2")}
		_, items := f.completedService(t, cut, cut)
		_, more := f.completedService(t, cut)
		f.expectOK(t, "three active partial warranties of one roll", func(sp pgx.Tx) error {
			for _, it := range append(items, more...) {
				w, err := db.New(sp).CreateWarrantyForServiceItem(ctx, f.warrantyParams(it))
				if err != nil {
					return err
				}
				if w.ItemKind != "partial" {
					return fmt.Errorf("item_kind = %q", w.ItemKind)
				}
			}
			return nil
		})
	})

	t.Run("period and row checks", func(t *testing.T) {
		piece := f.unit(t, f.piece, "")
		_, items := f.completedService(t, db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: piece.ID, Kind: "full"})
		arg := f.warrantyParams(items[0])
		arg.EndAt = arg.StartAt
		f.expectConstraint(t, "end = start", "23514", "chk_warranties_period", f.createWarranty(arg))
		arg.EndAt = ts(arg.StartAt.Time.Add(-time.Hour))
		f.expectConstraint(t, "end < start", "23514", "chk_warranties_period", f.createWarranty(arg))

		// A draft service opens no warranty.
		draft := f.service(t, f.dealer)
		u := f.unit(t, f.piece, "")
		it, err := f.q.CreateServiceItem(ctx, db.CreateServiceItemParams{
			ServiceID: draft.ID, ProductID: f.piece.ID, UnitID: u.ID, Kind: "full", AppliedParts: []byte(`[]`),
		})
		if err != nil {
			t.Fatal(err)
		}
		f.expectTrigger(t, "service not completed", f.createWarranty(f.warrantyParams(it)))

		// Status and the covered item.
		w, err := f.q.CreateWarrantyForServiceItem(ctx, f.warrantyParams(items[0]))
		if err != nil {
			t.Fatal(err)
		}
		f.expectConstraint(t, "expired without expired_at", "23514", "chk_warranties_expired", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE warranties SET status = 'expired' WHERE id = $1`, w.ID)
			return err
		})
		f.expectTrigger(t, "start is immutable", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE warranties SET start_at = start_at - INTERVAL '1 day' WHERE id = $1`, w.ID)
			return err
		})
		if _, err := f.q.VoidWarranty(ctx, db.VoidWarrantyParams{ID: w.ID, BrandID: w.BrandID}); err != nil {
			t.Fatal(err)
		}
		f.expectTrigger(t, "void is final", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE warranties SET status = 'active', voided_at = NULL WHERE id = $1`, w.ID)
			return err
		})
	})

	t.Run("holder change and cron queries", func(t *testing.T) {
		piece := f.unit(t, f.piece, "")
		_, items := f.completedService(t, db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: piece.ID, Kind: "full"})
		now := time.Now()
		arg := f.warrantyParams(items[0])
		arg.StartAt, arg.EndAt = ts(now.AddDate(-1, 0, 0)), ts(now.Add(5*24*time.Hour))
		w, err := f.q.CreateWarrantyForServiceItem(ctx, arg)
		if err != nil {
			t.Fatal(err)
		}
		due7, err := f.q.ListWarrantiesDue7DayNotice(ctx, db.ListWarrantiesDue7DayNoticeParams{Now: ts(now), RowLimit: 1000})
		if err != nil || !containsWarranty(due7, w.ID) {
			t.Fatalf("7-day notice: %v (found=%v)", err, containsWarranty(due7, w.ID))
		}
		if n, err := f.q.MarkWarrantyNotified7(ctx, db.MarkWarrantyNotified7Params{ID: w.ID, Now: ts(now)}); err != nil || n != 1 {
			t.Fatalf("mark 7: %d %v", n, err)
		}
		if n, _ := f.q.MarkWarrantyNotified7(ctx, db.MarkWarrantyNotified7Params{ID: w.ID, Now: ts(now)}); n != 0 {
			t.Fatalf("second mark 7 = %d", n)
		}
		other, err := f.q.CreateUser(ctx, db.CreateUserParams{
			PasswordHash: "x", Name: "Yeni", Surname: "T185", Status: "active",
			Email: text(fmt.Sprintf("t185-new-%d@example.test", time.Now().UnixNano())),
		})
		if err != nil {
			t.Fatal(err)
		}
		moved, err := f.q.ChangeWarrantyHolderByVehicle(ctx, db.ChangeWarrantyHolderByVehicleParams{
			VehicleID: f.vehicle.ID, NewHolderUserID: other.ID,
		})
		if err != nil || !containsWarranty(moved, w.ID) {
			t.Fatalf("holder change: %v", err)
		}
		expired, err := f.q.ExpireDueWarranties(ctx, ts(now.Add(6*24*time.Hour)))
		if err != nil || !containsWarranty(expired, w.ID) {
			t.Fatalf("expire: %v", err)
		}
	})
}

func containsWarranty(ws []db.Warranty, id int64) bool {
	for _, w := range ws {
		if w.ID == id {
			return true
		}
	}
	return false
}

func TestOrganizationGoogleBusinessURL(t *testing.T) {
	f := newServiceFixture(t)
	set := func(v pgtype.Text) func(sp pgx.Tx) error {
		return func(sp pgx.Tx) error {
			_, err := db.New(sp).SetOrganizationGoogleBusinessURL(f.ctx, db.SetOrganizationGoogleBusinessURLParams{
				ID: f.dealer.ID, GoogleBusinessUrl: v,
			})
			return err
		}
	}
	f.expectOK(t, "https", set(text("https://g.page/r/CabcDEF123/review")))
	f.expectOK(t, "null", set(pgtype.Text{}))
	for _, bad := range []string{"http://g.page/r/abc", "g.page/r/abc", "https://", "https://exa mple.com", "javascript:alert(1)", ""} {
		f.expectConstraint(t, "url "+bad, "23514", "chk_organizations_google_business_url", set(text(bad)))
	}
}

func TestVehicleTransferConstraints(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx
	params := func() db.CreateVehicleTransferParams {
		return db.CreateVehicleTransferParams{
			OrganizationID: f.dealer.ID, BrandID: f.dealer.BrandID, VehicleID: f.vehicle.ID,
			FromUserID: f.customer.ID, ToPhone: "+905551112233",
			FromCodeHash: "hash-a", ToCodeHash: "hash-b", ExpiresAt: ts(time.Now().Add(15 * time.Minute)),
		}
	}
	create := func(arg db.CreateVehicleTransferParams) func(sp pgx.Tx) error {
		return func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateVehicleTransfer(ctx, arg)
			return err
		}
	}
	arg := params()
	arg.ToPhone = "05551112233"
	f.expectConstraint(t, "phone not E.164", "23514", "chk_vehicle_transfers_to_phone", create(arg))
	arg = params()
	arg.ToCodeHash = " "
	f.expectConstraint(t, "empty hash", "23514", "chk_vehicle_transfers_hashes", create(arg))
	arg = params()
	arg.ExpiresAt = ts(time.Now().Add(-time.Minute))
	f.expectConstraint(t, "already expired", "23514", "chk_vehicle_transfers_expires", create(arg))
	arg = params()
	arg.ToUserID = pgtype.Int8{Int64: f.customer.ID, Valid: true}
	f.expectConstraint(t, "transfer to self", "23514", "chk_vehicle_transfers_to_user", create(arg))

	other, err := f.q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Baska", Surname: "T185", Status: "active",
		Email: text(fmt.Sprintf("t185-other-%d@example.test", time.Now().UnixNano())),
	})
	if err != nil {
		t.Fatal(err)
	}
	arg = params()
	arg.FromUserID = other.ID
	f.expectTrigger(t, "not the owner", create(arg))

	tr, err := f.q.CreateVehicleTransfer(ctx, params())
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	f.expectConstraint(t, "second pending", "23505", "uq_vehicle_transfers_pending", create(params()))
	// Completion needs both codes verified.
	if _, err := f.q.CompleteVehicleTransfer(ctx, db.CompleteVehicleTransferParams{ID: tr.ID, ToUserID: pgtype.Int8{Int64: other.ID, Valid: true}}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("complete unverified: %v", err)
	}
	if _, err := f.q.SetVehicleTransferVerified(ctx, db.SetVehicleTransferVerifiedParams{ID: tr.ID, FromSide: true, ToSide: true}); err != nil {
		t.Fatal(err)
	}
	done, err := f.q.CompleteVehicleTransfer(ctx, db.CompleteVehicleTransferParams{ID: tr.ID, ToUserID: pgtype.Int8{Int64: other.ID, Valid: true}})
	if err != nil || done.Status != "completed" {
		t.Fatalf("complete: %+v %v", done, err)
	}
	f.expectTrigger(t, "completed is final", func(sp pgx.Tx) error {
		_, err := sp.Exec(ctx, `UPDATE vehicle_transfers SET status = 'cancelled', cancelled_at = NOW(), completed_at = NULL WHERE id = $1`, tr.ID)
		return err
	})
}
