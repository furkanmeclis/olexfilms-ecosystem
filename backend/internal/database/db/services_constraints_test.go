package db_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-178: database-level guards of the service schema (migration 000050).
// Reuses the order fixture (rolled-back transaction, savepoint per failure).

type serviceFixture struct {
	*orderFixture
	customer db.User
	vehicle  db.Vehicle
	carBrand int64
	carModel int64
	film     db.Product // roll_meter, category parts: body_kaput, body_tavan
	piece    db.Product // serial piece, same category
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	f := &serviceFixture{orderFixture: newOrderFixture(t)}
	ctx, q := f.ctx, f.q
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var err error
	f.customer, err = q.CreateUser(ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Musteri", Surname: "T178", Status: "active",
	})
	if err != nil {
		t.Fatalf("customer: %v", err)
	}
	if err := f.tx.QueryRow(ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, "T178 "+suffix).Scan(&f.carBrand); err != nil {
		t.Fatal(err)
	}
	if err := f.tx.QueryRow(ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, 'T178 Model') RETURNING id`, f.carBrand).Scan(&f.carModel); err != nil {
		t.Fatal(err)
	}
	f.vehicle, err = q.CreateVehicle(ctx, db.CreateVehicleParams{
		UserID: f.customer.ID, BrandID: f.brandID,
		CarBrandID: pgtype.Int8{Int64: f.carBrand, Valid: true},
		CarModelID: pgtype.Int8{Int64: f.carModel, Valid: true},
	})
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: f.centerID, BrandID: f.brandID, Name: "t178-cat-" + suffix,
		AvailableParts: []byte(`["body_kaput","body_tavan"]`), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product := func(sku, unitType string) db.Product {
		p, err := q.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: f.centerID, BrandID: f.brandID, CategoryID: cat.ID,
			Sku: sku + "-" + suffix, Name: sku, Images: []byte("[]"), UnitType: unitType, Active: true,
		})
		if err != nil {
			t.Fatalf("product %s: %v", sku, err)
		}
		return p
	}
	f.film = product("t178-film", "roll_meter")
	f.piece = product("t178-piece", "piece")
	return f
}

func (f *serviceFixture) serviceParams(org db.Organization) db.CreateServiceParams {
	f.seq++
	return db.CreateServiceParams{
		ServiceNo:      fmt.Sprintf("T178-%d-%d", time.Now().UnixNano(), f.seq),
		OrganizationID: org.ID, BrandID: org.BrandID,
		CustomerUserID: f.customer.ID, VehicleID: f.vehicle.ID,
		CarBrandID: f.carBrand, CarModelID: f.carModel, Status: "draft",
	}
}

func (f *serviceFixture) service(t *testing.T, org db.Organization) db.Service {
	t.Helper()
	s, err := f.q.CreateService(f.ctx, f.serviceParams(org))
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return s
}

func (f *serviceFixture) unit(t *testing.T, p db.Product, meters string) db.Unit {
	t.Helper()
	u, err := f.q.CreateUnit(f.ctx, f.unitParams(t, p, meters, meters))
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	return u
}

// expectTrigger runs fn in a savepoint and requires check_violation (23514)
// raised by a trigger (no constraint name).
func (f *serviceFixture) expectTrigger(t *testing.T, name string, fn func(sp pgx.Tx) error) {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = fn(sp)
	_ = sp.Rollback(f.ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("%s: want 23514 from a trigger, got %v", name, err)
	}
}

func TestServiceSchemaConstraints(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx
	dealer := f.dealer

	s := f.service(t, dealer)
	if s.Status != "draft" || s.CompletedAt.Valid || s.CancelledAt.Valid {
		t.Fatalf("new service = %+v", s)
	}

	create := func(arg db.CreateServiceParams) func(sp pgx.Tx) error {
		return func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateService(ctx, arg)
			return err
		}
	}

	t.Run("vin check", func(t *testing.T) {
		for _, vin := range []string{"WVWZZZ1JZXW000001", "1HGCM82633A004352"} {
			arg := f.serviceParams(dealer)
			arg.Vin = text(vin)
			arg.HasMeasurement = true
			sp, err := f.tx.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.New(sp).CreateService(ctx, arg); err != nil {
				t.Fatalf("valid vin %s: %v", vin, err)
			}
			_ = sp.Rollback(ctx)
		}
		for _, vin := range []string{"WVWZZZ1JZXW00000", "WVWZZZ1JZXI000001", "WVWZZZ1JZXO000001", "WVWZZZ1JZXQ000001", "wvwzzz1jzxw000001", ""} {
			arg := f.serviceParams(dealer)
			arg.Vin = text(vin)
			f.expectConstraint(t, "invalid vin "+vin, "23514", "chk_services_vin", create(arg))
		}
		// "Measurement: yes" needs a VIN.
		arg := f.serviceParams(dealer)
		arg.HasMeasurement = true
		f.expectConstraint(t, "measurement without vin", "23514", "chk_services_measurement_vin", create(arg))
	})

	t.Run("status and row checks", func(t *testing.T) {
		arg := f.serviceParams(dealer)
		arg.Status = "bogus"
		f.expectConstraint(t, "status bogus", "23514", "chk_services_status", create(arg))
		arg = f.serviceParams(dealer)
		arg.Status = "completed" // completed_at missing
		f.expectConstraint(t, "completed without completed_at", "23514", "chk_services_completed", create(arg))
		// brand must be the organization's brand (decision 2).
		arg = f.serviceParams(dealer)
		glorian := brandCenter(t, ctx, f.q, "glorian")
		arg.BrandID = glorian.BrandID
		f.expectTrigger(t, "brand mismatch", create(arg))
		// The vehicle belongs to the customer.
		other, err := f.q.CreateUser(ctx, db.CreateUserParams{PasswordHash: "x", Name: "Other", Surname: "T178", Status: "active"})
		if err != nil {
			t.Fatal(err)
		}
		arg = f.serviceParams(dealer)
		arg.CustomerUserID = other.ID
		f.expectTrigger(t, "vehicle of another user", create(arg))
		// Duplicate service number.
		arg = f.serviceParams(dealer)
		arg.ServiceNo = s.ServiceNo
		f.expectConstraint(t, "duplicate service_no", "23505", "uq_services_service_no", create(arg))
	})

	t.Run("items locked after completion", func(t *testing.T) {
		svc := f.service(t, dealer)
		roll := f.unit(t, f.film, "15")
		piece := f.unit(t, f.piece, "")
		item, err := f.q.CreateServiceItem(ctx, db.CreateServiceItemParams{
			ServiceID: svc.ID, ProductID: f.film.ID, UnitID: roll.ID, Kind: "partial",
			Meters: numeric(t, "2.50"), AppliedParts: []byte(`["body_kaput"]`),
		})
		if err != nil {
			t.Fatalf("partial item: %v", err)
		}
		if item.OrganizationID != dealer.ID || item.BrandID != dealer.BrandID {
			t.Fatalf("item scope = %d/%d", item.OrganizationID, item.BrandID)
		}
		// Editing a draft item is fine.
		if _, err := f.q.UpdateServiceItem(ctx, db.UpdateServiceItemParams{
			ID: item.ID, ServiceID: svc.ID, Kind: "partial", Meters: numeric(t, "3"),
			AppliedParts: []byte(`["body_kaput","body_tavan"]`),
		}); err != nil {
			t.Fatalf("draft update: %v", err)
		}

		if _, err := f.q.CompleteService(ctx, db.CompleteServiceParams{ID: svc.ID}); err != nil {
			t.Fatalf("complete: %v", err)
		}
		f.expectTrigger(t, "update after completed", func(sp pgx.Tx) error {
			_, err := db.New(sp).UpdateServiceItem(ctx, db.UpdateServiceItemParams{
				ID: item.ID, ServiceID: svc.ID, Kind: "partial", Meters: numeric(t, "1"), AppliedParts: []byte(`[]`),
			})
			return err
		})
		f.expectTrigger(t, "insert after completed", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateServiceItem(ctx, db.CreateServiceItemParams{
				ServiceID: svc.ID, ProductID: f.piece.ID, UnitID: piece.ID, Kind: "full", AppliedParts: []byte(`[]`),
			})
			return err
		})
		f.expectTrigger(t, "delete after completed", func(sp pgx.Tx) error {
			_, err := db.New(sp).DeleteServiceItem(ctx, db.DeleteServiceItemParams{ID: item.ID, ServiceID: svc.ID})
			return err
		})
		// completed is final.
		f.expectTrigger(t, "completed -> cancelled", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE services SET status = 'cancelled', cancelled_at = NOW(), completed_at = NULL WHERE id = $1`, svc.ID)
			return err
		})
		// The completed service itself stays editable for the center (notes).
		if _, err := f.tx.Exec(ctx, `UPDATE services SET notes = 'center note' WHERE id = $1`, svc.ID); err != nil {
			t.Fatalf("center edit of completed service: %v", err)
		}
	})

	t.Run("item shape and parts", func(t *testing.T) {
		svc := f.service(t, dealer)
		roll := f.unit(t, f.film, "15")
		piece := f.unit(t, f.piece, "")
		fixed := f.unit(t, f.fixed, "")
		item := func(arg db.CreateServiceItemParams) func(sp pgx.Tx) error {
			arg.ServiceID = svc.ID
			if arg.AppliedParts == nil {
				arg.AppliedParts = []byte(`[]`)
			}
			return func(sp pgx.Tx) error {
				_, err := db.New(sp).CreateServiceItem(ctx, arg)
				return err
			}
		}
		f.expectConstraint(t, "partial without meters", "23514", "chk_service_items_amount",
			item(db.CreateServiceItemParams{ProductID: f.film.ID, UnitID: roll.ID, Kind: "partial"}))
		f.expectConstraint(t, "bogus kind", "23514", "chk_service_items_kind",
			item(db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: piece.ID, Kind: "half"}))
		f.expectTrigger(t, "partial from a piece",
			item(db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: piece.ID, Kind: "partial", Meters: numeric(t, "1")}))
		f.expectTrigger(t, "fixed barcode without quantity",
			item(db.CreateServiceItemParams{ProductID: f.fixed.ID, UnitID: fixed.ID, Kind: "full"}))
		f.expectTrigger(t, "unit of another product",
			item(db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: roll.ID, Kind: "full"}))
		f.expectTrigger(t, "part not in available_parts",
			item(db.CreateServiceItemParams{ProductID: f.piece.ID, UnitID: piece.ID, Kind: "full", AppliedParts: []byte(`["window_on_cam"]`)}))
		f.expectOK(t, "fixed barcode with quantity",
			item(db.CreateServiceItemParams{ProductID: f.fixed.ID, UnitID: fixed.ID, Kind: "full", Quantity: pgtype.Int4{Int32: 2, Valid: true}}))
		f.expectOK(t, "two cuts of one roll",
			func(sp pgx.Tx) error {
				for i := 0; i < 2; i++ {
					if err := item(db.CreateServiceItemParams{ProductID: f.film.ID, UnitID: roll.ID, Kind: "partial", Meters: numeric(t, "1")})(sp); err != nil {
						return err
					}
				}
				return nil
			})
	})

	t.Run("status log append-only", func(t *testing.T) {
		log, err := f.q.InsertServiceStatusLog(ctx, db.InsertServiceStatusLogParams{
			ServiceID: s.ID, ToStatus: "draft", Metadata: []byte("{}"),
		})
		if err != nil {
			t.Fatalf("log: %v", err)
		}
		f.expectCode(t, "update log", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE service_status_logs SET note = 'x' WHERE id = $1`, log.ID)
			return err
		}, "23001")
		f.expectCode(t, "delete log", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `DELETE FROM service_status_logs WHERE id = $1`, log.ID)
			return err
		}, "23001")
	})
}

func (f *serviceFixture) expectOK(t *testing.T, name string, fn func(sp pgx.Tx) error) {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sp.Rollback(f.ctx) }()
	if err := fn(sp); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// owner_type 'service' is a real FK to services(id, organization_id); the
// holder organization must be the service organization.
func TestServiceStockOwnerFK(t *testing.T) {
	f := newServiceFixture(t)
	ctx := f.ctx
	dealer := f.dealer
	svc := f.service(t, dealer)

	state := func(owner, holder int64) func(sp pgx.Tx) error {
		u := f.unit(t, f.piece, "")
		return func(sp pgx.Tx) error {
			_, err := db.New(sp).InsertUnitCurrentState(ctx, db.InsertUnitCurrentStateParams{
				UnitID: u.ID, BrandID: f.brandID, OwnerType: "service", OwnerID: owner,
				HolderOrgID: holder, Status: "used",
			})
			return err
		}
	}
	f.expectConstraint(t, "unknown service", "23503", "fk_unit_current_state_owner_service",
		state(svc.ID+1_000_000_000, dealer.ID))
	f.expectConstraint(t, "holder is not the service organization", "23503", "fk_unit_current_state_owner_service",
		state(svc.ID, f.dist.ID))
	f.expectOK(t, "service owner", state(svc.ID, dealer.ID))

	sp, err := f.tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := state(svc.ID, dealer.ID)(sp); err != nil {
		t.Fatal(err)
	}
	var gen pgtype.Int8
	if err := sp.QueryRow(ctx, `SELECT owner_service_id FROM unit_current_state WHERE owner_type = 'service' AND owner_id = $1`, svc.ID).Scan(&gen); err != nil || gen.Int64 != svc.ID {
		t.Fatalf("owner_service_id = %v, %v", gen, err)
	}
	// The service cannot be deleted while it owns stock.
	_, err = sp.Exec(ctx, `DELETE FROM services WHERE id = $1`, svc.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || (pgErr.Code != "23001" && pgErr.Code != "23503") {
		t.Fatalf("delete owning service: %v", err)
	}
	_ = sp.Rollback(ctx)

	holding := func(owner, holder int64) func(sp pgx.Tx) error {
		u := f.unit(t, f.fixed, "")
		return func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO fixed_barcode_holdings
				(unit_id, brand_id, owner_type, owner_id, holder_org_id, quantity_on_hand)
				VALUES ($1, $2, 'service', $3, $4, 1)`, u.ID, f.brandID, owner, holder)
			return err
		}
	}
	f.expectConstraint(t, "holding: unknown service", "23503", "fk_fixed_barcode_holdings_owner_service",
		holding(svc.ID+1_000_000_000, dealer.ID))
	f.expectConstraint(t, "holding: holder mismatch", "23503", "fk_fixed_barcode_holdings_owner_service",
		holding(svc.ID, f.dist.ID))
	f.expectOK(t, "holding: service owner", holding(svc.ID, dealer.ID))
}
