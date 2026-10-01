package db_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEC-153: database-level guards of the stock ledger (migration 000046).
// Everything runs inside one transaction that is rolled back; each failing
// statement runs in its own savepoint.

type stockFixture struct {
	ctx      context.Context
	tx       pgx.Tx
	q        *db.Queries
	brandID  int64
	centerID int64
	roll     db.Product
	fixed    db.Product
	seq      int
}

func newStockFixture(t *testing.T) *stockFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("olex brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("olex center: %v", err)
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: "t153-cat",
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	product := func(sku, unitType string, fixed bool) db.Product {
		p, err := q.CreateProduct(ctx, db.CreateProductParams{
			OrganizationID: center.ID, BrandID: brand.ID, CategoryID: cat.ID,
			Sku: sku + "-" + suffix, Name: sku, Images: []byte("[]"),
			UnitType: unitType, UsesFixedBarcode: fixed, Active: true,
		})
		if err != nil {
			t.Fatalf("product %s: %v", sku, err)
		}
		return p
	}
	return &stockFixture{
		ctx: ctx, tx: tx, q: q, brandID: brand.ID, centerID: center.ID,
		roll:  product("t153-roll", "roll_meter", false),
		fixed: product("t153-fixed", "piece", true),
	}
}

func numeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("numeric %s: %v", s, err)
	}
	return n
}

func (f *stockFixture) unitParams(t *testing.T, p db.Product, initial, remaining string) db.CreateUnitParams {
	t.Helper()
	f.seq++
	arg := db.CreateUnitParams{
		OrganizationID: f.centerID, BrandID: f.brandID, ProductID: p.ID,
		Barcode:  fmt.Sprintf("T153-%d-%d", time.Now().UnixNano(), f.seq),
		UnitKind: "serial", Source: "generated", Status: "available",
	}
	if p.UsesFixedBarcode {
		arg.UnitKind = "fixed"
	}
	if initial != "" {
		arg.InitialMeters = numeric(t, initial)
		arg.RemainingMeters = numeric(t, remaining)
	}
	return arg
}

func (f *stockFixture) movement(t *testing.T, u db.Unit, key string) db.InsertStockMovementParams {
	t.Helper()
	return db.InsertStockMovementParams{
		OrganizationID: f.centerID, BrandID: f.brandID, UnitID: u.ID, ProductID: u.ProductID,
		Type: "entry", QuantityDelta: 1, MetersDelta: numeric(t, "0"),
		ToOwnerType: pgtype.Text{String: "organization", Valid: true},
		ToOwnerID:   pgtype.Int8{Int64: f.centerID, Valid: true},
		ToStatus:    pgtype.Text{String: "available", Valid: true},
		Metadata:    []byte("{}"), IdempotencyKey: key,
	}
}

// expectCode runs fn in a savepoint and requires a PgError with one of codes.
func (f *stockFixture) expectCode(t *testing.T, name string, fn func(sp pgx.Tx) error, codes ...string) {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = fn(sp)
	_ = sp.Rollback(f.ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("%s: want SQLSTATE %v, got %v", name, codes, err)
	}
	for _, c := range codes {
		if pgErr.Code == c {
			return
		}
	}
	t.Fatalf("%s: want SQLSTATE %v, got %s (%s)", name, codes, pgErr.Code, pgErr.Message)
}

func TestStockSchemaConstraints(t *testing.T) {
	f := newStockFixture(t)
	ctx := f.ctx

	unit, err := f.q.CreateUnit(ctx, f.unitParams(t, f.roll, "15", "15"))
	if err != nil {
		t.Fatalf("create unit: %v", err)
	}
	mv, err := f.q.InsertStockMovement(ctx, f.movement(t, unit, fmt.Sprintf("t153-%d-entry", unit.ID)))
	if err != nil {
		t.Fatalf("insert movement: %v", err)
	}
	stateArg := db.InsertUnitCurrentStateParams{
		UnitID: unit.ID, BrandID: f.brandID, OwnerType: "organization", OwnerID: f.centerID,
		HolderOrgID: f.centerID, Status: "available",
		LastMovementID: pgtype.Int8{Int64: mv.ID, Valid: true},
	}
	if _, err := f.q.InsertUnitCurrentState(ctx, stateArg); err != nil {
		t.Fatalf("insert state: %v", err)
	}

	t.Run("second active owner rejected", func(t *testing.T) {
		f.expectCode(t, "second state row", func(sp pgx.Tx) error {
			_, err := db.New(sp).InsertUnitCurrentState(ctx, stateArg)
			return err
		}, "23505")
	})

	t.Run("state locked for update", func(t *testing.T) {
		got, err := f.q.LockUnitCurrentState(ctx, unit.ID)
		if err != nil || got.OwnerID != f.centerID || got.OwnerOrganizationID.Int64 != f.centerID {
			t.Fatalf("lock state = %+v, %v", got, err)
		}
	})

	t.Run("negative meters rejected", func(t *testing.T) {
		f.expectCode(t, "remaining < 0 on update", func(sp pgx.Tx) error {
			_, err := db.New(sp).UpdateUnitRemainingMeters(ctx, db.UpdateUnitRemainingMetersParams{
				ID: unit.ID, RemainingMeters: numeric(t, "-1"),
			})
			return err
		}, "23514")
		f.expectCode(t, "remaining < 0 on insert", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateUnit(ctx, f.unitParams(t, f.roll, "15", "-0.5"))
			return err
		}, "23514")
		f.expectCode(t, "remaining > initial", func(sp pgx.Tx) error {
			_, err := db.New(sp).UpdateUnitRemainingMeters(ctx, db.UpdateUnitRemainingMetersParams{
				ID: unit.ID, RemainingMeters: numeric(t, "16"),
			})
			return err
		}, "23514")
		// 15 - 5 - 4 = 6 stays valid.
		got, err := f.q.UpdateUnitRemainingMeters(ctx, db.UpdateUnitRemainingMetersParams{
			ID: unit.ID, RemainingMeters: numeric(t, "6"),
		})
		if err != nil {
			t.Fatalf("remaining 6: %v", err)
		}
		v, _ := got.RemainingMeters.Float64Value()
		if v.Float64 != 6 {
			t.Fatalf("remaining = %v", v.Float64)
		}
	})

	t.Run("roll product requires meters", func(t *testing.T) {
		f.expectCode(t, "roll without meters", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateUnit(ctx, f.unitParams(t, f.roll, "", ""))
			return err
		}, "23514")
	})

	t.Run("movement update and delete rejected", func(t *testing.T) {
		f.expectCode(t, "update movement", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE stock_movements SET quantity_delta = 5 WHERE id = $1`, mv.ID)
			return err
		}, "23001")
		f.expectCode(t, "delete movement", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `DELETE FROM stock_movements WHERE id = $1`, mv.ID)
			return err
		}, "23001")
	})

	t.Run("duplicate idempotency key rejected", func(t *testing.T) {
		f.expectCode(t, "raw duplicate insert", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `
				INSERT INTO stock_movements (organization_id, brand_id, unit_id, product_id, type, idempotency_key)
				VALUES ($1, $2, $3, $4, 'entry', $5)`,
				f.centerID, f.brandID, unit.ID, unit.ProductID, mv.IdempotencyKey)
			return err
		}, "23505")
		// The ledger query is idempotent: a retried key writes nothing.
		if _, err := f.q.InsertStockMovement(ctx, f.movement(t, unit, mv.IdempotencyKey)); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("retried key: want ErrNoRows, got %v", err)
		}
		got, err := f.q.GetStockMovementByIdempotencyKey(ctx, mv.IdempotencyKey)
		if err != nil || got.ID != mv.ID {
			t.Fatalf("existing movement = %+v, %v", got, err)
		}
	})

	t.Run("fixed barcode quantity per owner", func(t *testing.T) {
		fu, err := f.q.CreateUnit(ctx, f.unitParams(t, f.fixed, "", ""))
		if err != nil {
			t.Fatalf("fixed unit: %v", err)
		}
		fmv, err := f.q.InsertStockMovement(ctx, f.movement(t, fu, fmt.Sprintf("t153-%d-entry", fu.ID)))
		if err != nil {
			t.Fatalf("fixed movement: %v", err)
		}
		// Fixed units have no single owner row.
		f.expectCode(t, "fixed unit state", func(sp pgx.Tx) error {
			_, err := db.New(sp).InsertUnitCurrentState(ctx, db.InsertUnitCurrentStateParams{
				UnitID: fu.ID, BrandID: f.brandID, OwnerType: "organization", OwnerID: f.centerID,
				HolderOrgID: f.centerID, Status: "available",
			})
			return err
		}, "23503")
		key := db.EnsureFixedBarcodeHoldingParams{
			UnitID: fu.ID, BrandID: f.brandID, OwnerType: "organization", OwnerID: f.centerID,
			HolderOrgID: f.centerID,
		}
		if err := f.q.EnsureFixedBarcodeHolding(ctx, key); err != nil {
			t.Fatalf("ensure holding: %v", err)
		}
		add := db.AddFixedBarcodeHoldingParams{
			UnitID: fu.ID, OwnerType: "organization", OwnerID: f.centerID,
			QuantityDelta: 10, LastMovementID: pgtype.Int8{Int64: fmv.ID, Valid: true},
		}
		h, err := f.q.AddFixedBarcodeHolding(ctx, add)
		if err != nil || h.QuantityOnHand != 10 {
			t.Fatalf("add 10 = %+v, %v", h, err)
		}
		add.QuantityDelta = -4
		if h, err = f.q.AddFixedBarcodeHolding(ctx, add); err != nil || h.QuantityOnHand != 6 {
			t.Fatalf("remove 4 = %+v, %v", h, err)
		}
		add.QuantityDelta = -7
		f.expectCode(t, "negative quantity", func(sp pgx.Tx) error {
			_, err := db.New(sp).AddFixedBarcodeHolding(ctx, add)
			return err
		}, "23514")
	})

	t.Run("projection never negative", func(t *testing.T) {
		key := db.EnsureOrganizationProductStockParams{
			OrganizationID: f.centerID, ProductID: f.roll.ID, BrandID: f.brandID,
		}
		if err := f.q.EnsureOrganizationProductStock(ctx, key); err != nil {
			t.Fatalf("ensure org stock: %v", err)
		}
		f.expectCode(t, "negative org stock", func(sp pgx.Tx) error {
			_, err := db.New(sp).AddOrganizationProductStock(ctx, db.AddOrganizationProductStockParams{
				OrganizationID: f.centerID, ProductID: f.roll.ID,
				QuantityDelta: -1, MetersDelta: numeric(t, "0"),
			})
			return err
		}, "23514")
	})
}
