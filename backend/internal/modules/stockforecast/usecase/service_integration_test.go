package usecase

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeSettings struct {
	min, warn, crit, cover int
}

func (f fakeSettings) ForecastMinDays(context.Context) int            { return f.min }
func (f fakeSettings) ForecastDefaultWarningDays(context.Context) int { return f.warn }
func (f fakeSettings) ForecastCriticalDays(context.Context) int       { return f.crit }
func (f fakeSettings) ForecastDefaultCoverDays(context.Context) int   { return f.cover }

func TestService_RunOrganizationLedgerOrderCancelAndIdempotentDay(t *testing.T) {
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
	q := db.New(pool)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dist, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: "t484-dist-" + suffix, Name: "t484 dist", Status: "active",
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Type:           "distributor", ParentID: pgtype.Int8{Int64: center.ID, Valid: true},
		BrandID: brand.ID, Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("dist: %v", err)
	}
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: "t484-cat-" + suffix,
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	product, err := q.CreateProduct(ctx, db.CreateProductParams{
		OrganizationID: center.ID, BrandID: brand.ID, CategoryID: cat.ID,
		Sku: "T484-" + suffix, Name: "t484 product", Images: []byte("[]"),
		UnitType: "piece", UsesFixedBarcode: false, Active: true,
	})
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	loc, err := q.CreateWarehouseLocation(ctx, db.CreateWarehouseLocationParams{
		OrganizationID: center.ID, Code: "T484-" + suffix, Name: "forecast bin", Active: true,
	})
	if err != nil {
		t.Fatalf("location: %v", err)
	}
	unit, err := q.CreateUnit(ctx, db.CreateUnitParams{
		OrganizationID: center.ID, BrandID: brand.ID, ProductID: product.ID,
		Barcode: "T484-" + suffix, UnitKind: ledger.KindSerial, Source: "generated", Status: string(ledger.StatusPrinted),
	})
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	l := ledger.New(q, outbox.NewStore(pool, q))
	post := func(m ledger.Movement) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := l.Post(ctx, tx, m); err != nil {
			t.Fatalf("post %s: %v", m.Type, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	ref := time.Now().UnixNano()
	centerLoc := ledger.Owner{Type: ledger.OwnerWarehouseLocation, ID: loc.ID, OrgID: center.ID}
	distOrg := ledger.Owner{Type: ledger.OwnerOrganization, ID: dist.ID}
	post(ledger.Movement{Type: ledger.TypeEntry, UnitID: unit.ID, To: &centerLoc, Source: "t484", RefType: "fixture", RefID: ref})
	post(ledger.Movement{Type: ledger.TypeOrderOut, UnitID: unit.ID, To: &distOrg, Source: "t484", RefType: "fixture", RefID: ref + 1})
	post(ledger.Movement{Type: ledger.TypeOrderCancelRestore, UnitID: unit.ID, Source: "t484", RefType: "fixture", RefID: ref + 1})

	svc := New(pool, q, outbox.NewStore(pool, q), nil, fakeSettings{min: 1, warn: 14, crit: 7, cover: 30}, nil)
	day := time.Now()
	first, err := svc.ComputeProduct(ctx, center, product, day)
	if err != nil {
		t.Fatalf("compute first: %v", err)
	}
	second, err := svc.ComputeProduct(ctx, center, product, day)
	if err != nil {
		t.Fatalf("compute second: %v", err)
	}
	if numeric(first.Snapshot.AvgDaily30) != 0 || numeric(second.Snapshot.AvgDaily30) != 0 {
		t.Fatalf("avg_daily_30 first=%.4f second=%.4f, want cancelled order_out netted to zero",
			numeric(first.Snapshot.AvgDaily30), numeric(second.Snapshot.AvgDaily30))
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM stock_forecasts WHERE organization_id=$1 AND product_id=$2 AND computed_on=$3`,
		center.ID, product.ID, first.Snapshot.ComputedOn).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("forecast rows for org/product/day = %d, want 1", rows)
	}

	if _, err := q.UpsertStockForecastSnapshot(ctx, db.UpsertStockForecastSnapshotParams{
		OrganizationID:    dist.ID,
		BrandID:           brand.ID,
		ProductID:         product.ID,
		ComputedOn:        date(day),
		IsLatest:          true,
		OnHandQty:         20,
		OnHandMeters:      num(0),
		AvgDaily30:        num4(2),
		AvgDaily90:        num4(1),
		SeasonalityFactor: num3(1),
		DataDays:          400,
		Status:            StatusOK,
	}); err != nil {
		t.Fatalf("seed latest forecast: %v", err)
	}
	networkRows, err := svc.computeNetworkDemand(ctx, center, day)
	if err != nil {
		t.Fatalf("network demand: %v", err)
	}
	if networkRows != 3 {
		t.Fatalf("network rows = %d, want 3", networkRows)
	}
	var networkCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM network_demand_forecasts WHERE brand_id=$1 AND product_id=$2`,
		brand.ID, product.ID).Scan(&networkCount); err != nil {
		t.Fatal(err)
	}
	if networkCount != 3 {
		t.Fatalf("network demand rows = %d, want 3", networkCount)
	}
}
