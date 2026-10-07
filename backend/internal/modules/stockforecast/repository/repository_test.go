package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stockforecast/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	ctx    context.Context
	tx     pgx.Tx
	q      *db.Queries
	repo   *repository.Repository
	brand  int64
	center db.Organization
	cat    db.ProductCategory
	suffix string
}

func newFixture(t *testing.T) *fixture {
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
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	cat, err := q.CreateProductCategory(ctx, db.CreateProductCategoryParams{
		OrganizationID: center.ID, BrandID: brand.ID, Name: "t483-cat-" + suffix,
		AvailableParts: []byte("[]"), Active: true,
	})
	if err != nil {
		t.Fatalf("category: %v", err)
	}
	return &fixture{
		ctx: ctx, tx: tx, q: q, repo: repository.New(q),
		brand: brand.ID, center: center, cat: cat, suffix: suffix,
	}
}

func (f *fixture) product(t *testing.T, sku string) db.Product {
	t.Helper()
	p, err := f.q.CreateProduct(f.ctx, db.CreateProductParams{
		OrganizationID: f.center.ID, BrandID: f.brand, CategoryID: f.cat.ID,
		Sku: sku + "-" + f.suffix, Name: sku, Images: []byte("[]"),
		UnitType: "piece", UsesFixedBarcode: false, Active: true,
	})
	if err != nil {
		t.Fatalf("product %s: %v", sku, err)
	}
	return p
}

func num(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("numeric %s: %v", s, err)
	}
	return n
}

func date(y int, m time.Month, d int) pgtype.Date {
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

func (f *fixture) snapshot(t *testing.T, p db.Product, day pgtype.Date, latest bool, daysLeft pgtype.Numeric) db.StockForecast {
	t.Helper()
	row, err := f.repo.UpsertSnapshot(f.ctx, db.UpsertStockForecastSnapshotParams{
		OrganizationID:    f.center.ID,
		BrandID:           f.brand,
		ProductID:         p.ID,
		ComputedOn:        day,
		IsLatest:          latest,
		OnHandQty:         10,
		OnHandMeters:      num(t, "0"),
		AvgDaily30:        num(t, "1.0000"),
		AvgDaily90:        num(t, "1.0000"),
		SeasonalityFactor: num(t, "1.000"),
		DaysLeft:          daysLeft,
		DataDays:          90,
		Status:            "ok",
	})
	if err != nil {
		t.Fatalf("upsert snapshot: %v", err)
	}
	return row
}

func TestUpsertSnapshotKeepsSingleLatest(t *testing.T) {
	f := newFixture(t)
	p := f.product(t, "t483-latest")
	f.snapshot(t, p, date(2026, time.October, 1), true, num(t, "8.00"))
	second := f.snapshot(t, p, date(2026, time.October, 2), true, num(t, "7.00"))
	if !second.IsLatest {
		t.Fatal("second snapshot must be latest")
	}
	var latestCount int
	if err := f.tx.QueryRow(f.ctx, `
		SELECT COUNT(*) FROM stock_forecasts
		WHERE organization_id = $1 AND product_id = $2 AND is_latest`, f.center.ID, p.ID).Scan(&latestCount); err != nil {
		t.Fatal(err)
	}
	if latestCount != 1 {
		t.Fatalf("latest rows = %d, want 1", latestCount)
	}
	latest, err := f.q.GetLatestStockForecast(f.ctx, db.GetLatestStockForecastParams{
		OrganizationID: f.center.ID, ProductID: p.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !latest.ComputedOn.Time.Equal(second.ComputedOn.Time) {
		t.Fatalf("latest date = %v, want %v", latest.ComputedOn.Time, second.ComputedOn.Time)
	}
}

func TestListSortDaysLeftNullsLastAndTiebreak(t *testing.T) {
	f := newFixture(t)
	p1 := f.product(t, "t483-a")
	p2 := f.product(t, "t483-b")
	p3 := f.product(t, "t483-c")
	p4 := f.product(t, "t483-d")
	f.snapshot(t, p1, date(2026, time.October, 1), true, num(t, "2.00"))
	f.snapshot(t, p2, date(2026, time.October, 1), true, num(t, "2.00"))
	f.snapshot(t, p3, date(2026, time.October, 1), true, pgtype.Numeric{})
	f.snapshot(t, p4, date(2026, time.October, 1), true, num(t, "5.00"))

	arg := db.ListStockForecastsParams{
		OrganizationID: f.center.ID, BrandID: f.brand,
		SortKey: "days_left", LimitCount: 10,
	}
	asc, err := f.repo.List(f.ctx, arg)
	if err != nil {
		t.Fatal(err)
	}
	if got := []int64{asc[0].ProductID, asc[1].ProductID, asc[2].ProductID, asc[3].ProductID}; got[0] != p1.ID || got[1] != p2.ID || got[2] != p4.ID || got[3] != p3.ID {
		t.Fatalf("asc product order = %v; want [%d %d %d %d]", got, p1.ID, p2.ID, p4.ID, p3.ID)
	}
	arg.SortDesc = true
	desc, err := f.repo.List(f.ctx, arg)
	if err != nil {
		t.Fatal(err)
	}
	if got := []int64{desc[0].ProductID, desc[1].ProductID, desc[2].ProductID, desc[3].ProductID}; got[0] != p4.ID || got[1] != p2.ID || got[2] != p1.ID || got[3] != p3.ID {
		t.Fatalf("desc product order = %v; want [%d %d %d %d]", got, p4.ID, p2.ID, p1.ID, p3.ID)
	}
}
