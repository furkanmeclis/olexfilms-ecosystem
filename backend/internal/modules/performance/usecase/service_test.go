package usecase

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type featureMap map[string]bool

func (f featureMap) Enabled(_ context.Context, _ int64, key string) (bool, error) {
	if v, ok := f[key]; ok {
		return v, nil
	}
	return true, nil
}

type perfFixture struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	prefix  string
	seq     int
	brandID int64
	center  db.Organization
	dist    db.Organization
	dealer1 db.Organization
	dealer2 db.Organization
	carB    int64
	carM    int64
	cat     int64
	product int64
}

func newPerfFixture(t *testing.T) *perfFixture {
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
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	q := db.New(tx)
	brand, err := q.GetBrandBySlug(ctx, "olex")
	if err != nil {
		t.Fatalf("brand: %v", err)
	}
	center, err := q.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	f := &perfFixture{
		ctx: ctx, tx: tx, q: q, prefix: fmt.Sprintf("t491-%d", time.Now().UnixNano()),
		brandID: brand.ID, center: center,
	}
	f.dist = f.org(t, "distributor", "dist", center.ID)
	f.dealer1 = f.org(t, "dealer", "dealer-one", f.dist.ID)
	f.dealer2 = f.org(t, "dealer", "dealer-two", f.dist.ID)
	f.catalog(t)
	return f
}

func (f *perfFixture) org(t *testing.T, typ, name string, parent int64) db.Organization {
	t.Helper()
	f.seq++
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("%s-%s-%d", f.prefix, typ, f.seq), Name: f.prefix + " " + name,
		Status: "active", Type: typ, ParentID: pgtype.Int8{Int64: parent, Valid: true}, BrandID: f.brandID,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("org: %v", err)
	}
	_, err = f.tx.Exec(f.ctx, `UPDATE organizations SET contract_valid_until = DATE '2026-10-20' WHERE id = $1`, o.ID)
	if err != nil {
		t.Fatalf("contract date: %v", err)
	}
	o.ContractValidUntil = pgtype.Date{Time: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), Valid: true}
	return o
}

func (f *perfFixture) user(t *testing.T, name string) int64 {
	t.Helper()
	f.seq++
	var id int64
	err := f.tx.QueryRow(f.ctx, `
INSERT INTO users (password_hash, name, surname, status, email)
VALUES ('x', $1, 'T491', 'active', $2)
RETURNING id`, name, fmt.Sprintf("%s-%s-%d@example.test", f.prefix, name, f.seq)).Scan(&id)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return id
}

func (f *perfFixture) catalog(t *testing.T) {
	t.Helper()
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, f.prefix+" car").Scan(&f.carB); err != nil {
		t.Fatalf("car brand: %v", err)
	}
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_models (car_brand_id, name, body_type) VALUES ($1, $2, 'sedan') RETURNING id`, f.carB, f.prefix+" model").Scan(&f.carM); err != nil {
		t.Fatalf("car model: %v", err)
	}
	if err := f.tx.QueryRow(f.ctx, `
INSERT INTO product_categories (organization_id, brand_id, name, available_parts)
VALUES ($1, $2, $3, '["hood"]') RETURNING id`, f.center.ID, f.brandID, f.prefix+" cat").Scan(&f.cat); err != nil {
		t.Fatalf("category: %v", err)
	}
	if err := f.tx.QueryRow(f.ctx, `
INSERT INTO products (organization_id, brand_id, category_id, sku, name, unit_type)
VALUES ($1, $2, $3, $4, $5, 'piece') RETURNING id`, f.center.ID, f.brandID, f.cat, f.prefix+"-sku", f.prefix+" film").Scan(&f.product); err != nil {
		t.Fatalf("product: %v", err)
	}
}

func (f *perfFixture) service(t *testing.T, org db.Organization, completed time.Time, measurement bool, rating int, warranty bool, staff int64) int64 {
	t.Helper()
	customer := f.user(t, "cust")
	var vehicle int64
	err := f.tx.QueryRow(f.ctx, `
INSERT INTO vehicles (user_id, organization_id, brand_id, car_brand_id, car_model_id, vin)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, customer, org.ID, f.brandID, f.carB, f.carM, fmt.Sprintf("A491ABCDEFG%06d", f.seq)).Scan(&vehicle)
	if err != nil {
		t.Fatalf("vehicle: %v", err)
	}
	var svc int64
	vin := pgtype.Text{}
	if measurement {
		vin = pgtype.Text{String: fmt.Sprintf("B491ABCDEFG%06d", f.seq), Valid: true}
	}
	err = f.tx.QueryRow(f.ctx, `
INSERT INTO services (service_no, organization_id, brand_id, customer_user_id, vehicle_id,
 car_brand_id, car_model_id, vin, has_measurement, status, completed_at, created_by_user_id,
 completed_by_user_id, performed_by_user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'draft', NULL, $10, $10, $10)
RETURNING id`, fmt.Sprintf("S-%s-%d", f.prefix, f.seq), org.ID, f.brandID, customer, vehicle,
		f.carB, f.carM, vin, measurement, staff).Scan(&svc)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	var unit int64
	err = f.tx.QueryRow(f.ctx, `
INSERT INTO units (organization_id, brand_id, product_id, barcode, status)
VALUES ($1, $2, $3, $4, 'used') RETURNING id`, f.center.ID, f.brandID, f.product, fmt.Sprintf("BC-%s-%d", f.prefix, f.seq)).Scan(&unit)
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	var item int64
	err = f.tx.QueryRow(f.ctx, `
INSERT INTO service_items (service_id, organization_id, brand_id, product_id, unit_id, kind)
VALUES ($1, $2, $3, $4, $5, 'full') RETURNING id`, svc, org.ID, f.brandID, f.product, unit).Scan(&item)
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if _, err = f.tx.Exec(f.ctx, `UPDATE services SET status = 'completed', completed_at = $2 WHERE id = $1`, svc, completed); err != nil {
		t.Fatalf("complete service: %v", err)
	}
	if rating > 0 {
		_, err = f.tx.Exec(f.ctx, `
INSERT INTO service_reviews (organization_id, brand_id, service_id, customer_user_id, platform_rating, product_rating, created_at)
VALUES ($1, $2, $3, $4, $5, $5, $6)`, org.ID, f.brandID, svc, customer, rating, completed.Add(time.Hour))
		if err != nil {
			t.Fatalf("review: %v", err)
		}
	}
	if warranty {
		_, err = f.tx.Exec(f.ctx, `
INSERT INTO warranties (organization_id, brand_id, service_id, service_item_id, product_id, unit_id, item_kind, vehicle_id, holder_user_id, start_at, end_at)
VALUES ($1, $2, $3, $4, $5, $6, 'full', $7, $8, $9, $10)`,
			org.ID, f.brandID, svc, item, f.product, unit, vehicle, customer, completed, completed.AddDate(1, 0, 0))
		if err != nil {
			t.Fatalf("warranty: %v", err)
		}
	}
	_, err = f.tx.Exec(f.ctx, `
INSERT INTO efficiency_facts (organization_id, brand_id, service_id, service_item_id, unit_id, product_id,
 dealer_org_id, staff_user_id, part_key, actual_meters, expected_meters, service_date)
VALUES ($1, $2, $3, $4, $5, $6, $1, $7, 'hood', $8, 10, $9)`,
		org.ID, f.brandID, svc, item, unit, f.product, staff, 10+f.seq, completed)
	if err != nil {
		t.Fatalf("efficiency fact: %v", err)
	}
	return svc
}

func (f *perfFixture) stockAndAccounting(t *testing.T, org db.Organization, at time.Time) {
	t.Helper()
	var unit int64
	err := f.tx.QueryRow(f.ctx, `
INSERT INTO units (organization_id, brand_id, product_id, barcode, status)
VALUES ($1, $2, $3, $4, 'used') RETURNING id`, f.center.ID, f.brandID, f.product, "ST-"+f.prefix).Scan(&unit)
	if err != nil {
		t.Fatalf("stock unit: %v", err)
	}
	_, err = f.tx.Exec(f.ctx, `
INSERT INTO stock_movements (organization_id, brand_id, unit_id, product_id, type, quantity_delta,
 from_owner_type, from_owner_id, to_owner_type, to_owner_id, from_status, to_status, idempotency_key, created_at)
VALUES ($1, $2, $3, $4, 'consumption', -4, 'organization', $1, 'service', 1, 'available', 'used', $5, $6)`,
		org.ID, f.brandID, unit, f.product, "stock-"+f.prefix, at.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("stock movement: %v", err)
	}
	_, err = f.tx.Exec(f.ctx, `
INSERT INTO organization_product_stocks (organization_id, brand_id, product_id, quantity, meters)
VALUES ($1, $2, $3, 8, 0)`, org.ID, f.brandID, f.product)
	if err != nil {
		t.Fatalf("org stock: %v", err)
	}
	customer := f.user(t, "cari")
	if _, err = f.tx.Exec(f.ctx, `
INSERT INTO customer_organizations (user_id, organization_id, brand_id)
VALUES ($1, $2, $3)`, customer, org.ID, f.brandID); err != nil {
		t.Fatalf("cari customer org: %v", err)
	}
	var cari int64
	err = f.tx.QueryRow(f.ctx, `
INSERT INTO cari_accounts (organization_id, brand_id, counterparty_type, counterparty_user_id, currency)
VALUES ($1, $2, 'user', $3, 'TRY') RETURNING id`, org.ID, f.brandID, customer).Scan(&cari)
	if err != nil {
		t.Fatalf("cari: %v", err)
	}
	_, err = f.tx.Exec(f.ctx, `
INSERT INTO finance_entries (organization_id, brand_id, cari_id, direction, category, orig_currency,
 orig_amount, currency, amount, rate, rate_date, role, created_at)
VALUES ($1, $2, $3, 'income', 'service.revenue', 'TRY', 150, 'TRY', 150, 1, $4, 'main', $5)`,
		org.ID, f.brandID, cari, at, at.AddDate(0, 0, -40))
	if err != nil {
		t.Fatalf("finance entry: %v", err)
	}
}

func (f *perfFixture) lead(t *testing.T, org db.Organization, status string, at time.Time) {
	t.Helper()
	_, err := f.tx.Exec(f.ctx, `
INSERT INTO leads (organization_id, brand_id, target_type, source, status, lost_reason, notes, updated_at)
VALUES ($1, $2, 'customer', 'other', $3, $4, '', $5)`,
		org.ID, f.brandID, status, map[string]any{"won": nil, "lost": "no"}[status], at)
	if err != nil {
		t.Fatalf("lead %s: %v", status, err)
	}
}

func (f *perfFixture) certificate(t *testing.T, org db.Organization, userID int64, at time.Time) {
	t.Helper()
	var typ int64
	err := f.tx.QueryRow(f.ctx, `
INSERT INTO certificate_types (organization_id, brand_id, name)
VALUES ($1, $2, '{"tr":"PPF"}') RETURNING id`, f.center.ID, f.brandID).Scan(&typ)
	if err != nil {
		t.Fatalf("cert type: %v", err)
	}
	_, err = f.tx.Exec(f.ctx, `
INSERT INTO certificates (user_id, organization_id, brand_id, type_id, storage_key, sha256,
 issued_at, expires_at, status, verified_by_user_id, verified_by_org_id, verified_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'valid', $1, $2, $7)`,
		userID, org.ID, f.brandID, typ, "cert-"+uuid.NewString(), fmt.Sprintf("%064x", f.seq+1), at.AddDate(0, -1, 0), at.AddDate(1, 0, 0))
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
}

func (f *perfFixture) order(t *testing.T, seller, buyer db.Organization, at time.Time) {
	t.Helper()
	_, err := f.tx.Exec(f.ctx, `
INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, status, currency,
 rate_snapshot, try_rate, subtotal, total, approved_at)
VALUES ($1, $2, $1, $3, 'approved', 'TRY', '{}', 1, 100, 100, $4)`,
		seller.ID, f.brandID, buyer.ID, at)
	if err != nil {
		t.Fatalf("order: %v", err)
	}
}

func TestPerformanceRunComputesMetricsIdempotentlyAndUpdatesPreviousMonth(t *testing.T) {
	f := newPerfFixture(t)
	now := time.Date(2026, 10, 10, 0, 30, 0, 0, time.UTC)
	staff1 := f.user(t, "staff1")
	staff2 := f.user(t, "staff2")
	f.certificate(t, f.dealer1, staff1, now)
	f.service(t, f.dealer1, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), true, 4, true, staff1)
	f.service(t, f.dealer1, time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), false, 2, false, staff2)
	f.service(t, f.dealer2, time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), false, 0, false, staff2)
	f.stockAndAccounting(t, f.dealer1, now)
	f.lead(t, f.dealer1, "won", now.AddDate(0, 0, -1))
	f.lead(t, f.dealer1, "lost", now.AddDate(0, 0, -1))
	f.order(t, f.dist, f.dealer1, now.AddDate(0, 0, -1))
	f.service(t, f.dealer1, time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC), false, 0, false, staff2)

	mem := outbox.NewMemory()
	svc := New(f.tx, f.q, mem, featureMap{}, nil)
	res, err := svc.Run(f.ctx, f.dealer1.ID, now)
	if err != nil {
		t.Fatalf("run dealer: %v", err)
	}
	if res.Organizations != 1 || res.Periods != 2 || res.Metrics == 0 {
		t.Fatalf("result = %+v", res)
	}
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricServicesCount, "2.0000")
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricWarrantyStartRate, "0.5000")
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricMeasurementRate, "0.5000")
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricReviewAvg, "3.0000")
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricStockTurnover, "0.5000")
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricContractDaysLeft, "10.0000")
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricCariOverdueAmount, "150.0000")
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricCertificateCoverage, "0.5000")
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricLeadConversionRate, "0.5000")
	assertMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricOrderVolume, "100.0000")
	assertMetric(t, f, f.dealer1.ID, "2026-09", ScopeOrg, model.MetricServicesCount, "1.0000")
	if got := len(mem.All()); got != 2 {
		t.Fatalf("events = %d, want current+previous", got)
	}

	before := metricRows(t, f, f.dealer1.ID)
	if _, err := svc.Run(f.ctx, f.dealer1.ID, now); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if after := metricRows(t, f, f.dealer1.ID); after != before {
		t.Fatalf("idempotent row count = %d, want %d", after, before)
	}

	f.service(t, f.dealer1, time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC), false, 0, false, staff2)
	if _, err := svc.Run(f.ctx, f.dealer1.ID, now); err != nil {
		t.Fatalf("late previous run: %v", err)
	}
	assertMetric(t, f, f.dealer1.ID, "2026-09", ScopeOrg, model.MetricServicesCount, "2.0000")

	if _, err := New(f.tx, f.q, outbox.NewMemory(), featureMap{features.ModuleReviews: false}, nil).Run(f.ctx, f.dealer1.ID, now); err != nil {
		t.Fatalf("reviews off run: %v", err)
	}
	assertNoMetric(t, f, f.dealer1.ID, "2026-10", ScopeOrg, model.MetricReviewAvg)

	if _, err := svc.Run(f.ctx, f.dist.ID, now); err != nil {
		t.Fatalf("dist run: %v", err)
	}
	assertMetric(t, f, f.dist.ID, "2026-10", ScopeSubtree, model.MetricServicesCount, "3.0000")
}

func metricRows(t *testing.T, f *perfFixture, orgID int64) int {
	t.Helper()
	var n int
	if err := f.tx.QueryRow(f.ctx, `SELECT count(*) FROM performance_metrics_monthly WHERE organization_id = $1`, orgID).Scan(&n); err != nil {
		t.Fatalf("metric rows: %v", err)
	}
	return n
}

func assertMetric(t *testing.T, f *perfFixture, orgID int64, period, scope, metric, want string) {
	t.Helper()
	var got string
	err := f.tx.QueryRow(f.ctx, `
SELECT value::text FROM performance_metrics_monthly
WHERE organization_id = $1 AND period = $2 AND scope = $3 AND metric = $4`,
		orgID, period, scope, metric).Scan(&got)
	if err != nil {
		t.Fatalf("metric %s/%s/%s: %v", period, scope, metric, err)
	}
	if got != want {
		t.Fatalf("metric %s/%s/%s = %s, want %s", period, scope, metric, got, want)
	}
}

func assertNoMetric(t *testing.T, f *perfFixture, orgID int64, period, scope, metric string) {
	t.Helper()
	var n int
	err := f.tx.QueryRow(f.ctx, `
SELECT count(*) FROM performance_metrics_monthly
WHERE organization_id = $1 AND period = $2 AND scope = $3 AND metric = $4`,
		orgID, period, scope, metric).Scan(&n)
	if err != nil {
		t.Fatalf("no metric %s: %v", metric, err)
	}
	if n != 0 {
		t.Fatalf("metric %s exists with count %d", metric, n)
	}
}
