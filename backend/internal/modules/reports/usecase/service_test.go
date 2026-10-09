package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testNow = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

// featureMap: every module on unless listed.
type featureMap map[string]bool

func (f featureMap) Enabled(_ context.Context, _ int64, key string) (bool, error) {
	if v, ok := f[key]; ok {
		return v, nil
	}
	return true, nil
}

type fixture struct {
	ctx     context.Context
	tx      pgx.Tx
	q       *db.Queries
	prefix  string
	seq     int
	brandID int64
	center  db.Organization
	distA   db.Organization
	distB   db.Organization
	dealer1 db.Organization
	dealer2 db.Organization
	dealer3 db.Organization
	carB    int64
	carM    int64
	product int64
}

// newFixture seeds an isolated brand: center -> distA -> dealer1, dealer2
// and center -> distB -> dealer3. dealer1 holds 1 record of every domain,
// dealer2 2 and dealer3 3, so the reach of every report is visible in its
// totals (dealer1 = 1, distA = 3, center = 6).
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
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	f := &fixture{ctx: ctx, tx: tx, q: db.New(tx), prefix: fmt.Sprintf("t495-%d", time.Now().UnixNano())}
	if err := tx.QueryRow(ctx, `INSERT INTO brands (slug, name) VALUES ($1, $1) RETURNING id`, f.prefix).Scan(&f.brandID); err != nil {
		t.Fatalf("brand: %v", err)
	}
	f.center = f.org(t, "center", "center", 0)
	f.distA = f.org(t, "distributor", "dist-a", f.center.ID)
	f.distB = f.org(t, "distributor", "dist-b", f.center.ID)
	f.dealer1 = f.org(t, "dealer", "dealer-1", f.distA.ID)
	f.dealer2 = f.org(t, "dealer", "dealer-2", f.distA.ID)
	f.dealer3 = f.org(t, "dealer", "dealer-3", f.distB.ID)
	f.catalog(t)
	for n, org := range []db.Organization{f.dealer1, f.dealer2, f.dealer3} {
		for i := 0; i <= n; i++ {
			f.records(t, org)
		}
	}
	return f
}

func (f *fixture) org(t *testing.T, typ, name string, parent int64) db.Organization {
	t.Helper()
	f.seq++
	o, err := f.q.CreateOrganization(f.ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("%s-%s-%d", f.prefix, typ, f.seq), Name: f.prefix + " " + name,
		Status: "active", Type: typ, ParentID: pgtype.Int8{Int64: parent, Valid: parent != 0}, BrandID: f.brandID,
		Currency: "TRY", Locale: "tr", Timezone: "Europe/Istanbul", Settings: []byte("{}"),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	})
	if err != nil {
		t.Fatalf("org %s: %v", name, err)
	}
	return o
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.tx.Exec(f.ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (f *fixture) id(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, sql, args...).Scan(&id); err != nil {
		t.Fatalf("insert %q: %v", sql, err)
	}
	return id
}

func (f *fixture) catalog(t *testing.T) {
	t.Helper()
	f.carB = f.id(t, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`, f.prefix+" car")
	f.carM = f.id(t, `INSERT INTO car_models (car_brand_id, name, body_type) VALUES ($1, $2, 'sedan') RETURNING id`, f.carB, f.prefix+" model")
	cat := f.id(t, `INSERT INTO product_categories (organization_id, brand_id, name, available_parts)
VALUES ($1, $2, $3, '["hood"]') RETURNING id`, f.center.ID, f.brandID, f.prefix+" cat")
	f.product = f.id(t, `INSERT INTO products (organization_id, brand_id, category_id, sku, name, unit_type)
VALUES ($1, $2, $3, $4, $5, 'piece') RETURNING id`, f.center.ID, f.brandID, cat, f.prefix+"-sku", f.prefix+" film")
}

// records seeds one record of every domain for org, two days before now.
func (f *fixture) records(t *testing.T, org db.Organization) {
	t.Helper()
	f.seq++
	at := testNow.AddDate(0, 0, -2)
	customer := f.id(t, `INSERT INTO users (password_hash, name, surname, status, email)
VALUES ('x', 'c', 'T495', 'active', $1) RETURNING id`, fmt.Sprintf("%s-%d@example.test", f.prefix, f.seq))
	f.exec(t, `INSERT INTO customer_organizations (user_id, organization_id, brand_id, created_at) VALUES ($1, $2, $3, $4)`,
		customer, org.ID, f.brandID, at)
	vehicle := f.id(t, `INSERT INTO vehicles (user_id, organization_id, brand_id, car_brand_id, car_model_id, vin)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, customer, org.ID, f.brandID, f.carB, f.carM, fmt.Sprintf("A495ABCDEFG%06d", f.seq))
	svc := f.id(t, `INSERT INTO services (service_no, organization_id, brand_id, customer_user_id, vehicle_id,
 car_brand_id, car_model_id, status, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'draft', $8) RETURNING id`, fmt.Sprintf("S-%s-%d", f.prefix, f.seq), org.ID, f.brandID,
		customer, vehicle, f.carB, f.carM, at)
	unit := f.id(t, `INSERT INTO units (organization_id, brand_id, product_id, barcode, status)
VALUES ($1, $2, $3, $4, 'used') RETURNING id`, f.center.ID, f.brandID, f.product, fmt.Sprintf("BC-%s-%d", f.prefix, f.seq))
	item := f.id(t, `INSERT INTO service_items (service_id, organization_id, brand_id, product_id, unit_id, kind)
VALUES ($1, $2, $3, $4, $5, 'full') RETURNING id`, svc, org.ID, f.brandID, f.product, unit)
	f.exec(t, `UPDATE services SET status = 'completed', completed_at = $2 WHERE id = $1`, svc, at)
	f.exec(t, `INSERT INTO warranties (organization_id, brand_id, service_id, service_item_id, product_id, unit_id, item_kind,
 vehicle_id, holder_user_id, start_at, end_at)
VALUES ($1, $2, $3, $4, $5, $6, 'full', $7, $8, $9, $10)`, org.ID, f.brandID, svc, item, f.product, unit, vehicle, customer,
		at, at.AddDate(1, 0, 0))
	parent := f.distA
	if org.ParentID.Int64 == f.distB.ID {
		parent = f.distB
	}
	f.exec(t, `INSERT INTO orders (organization_id, brand_id, seller_org_id, buyer_org_id, status, currency,
 rate_snapshot, try_rate, subtotal, total, created_at, approved_at)
VALUES ($1, $2, $1, $3, 'approved', 'TRY', '{}', 1, 100, 100, $4, $4)`, parent.ID, f.brandID, org.ID, at)
	f.exec(t, `INSERT INTO measurement_results (organization_id, brand_id, vin, status, raw, measured_at, created_at, created_by)
VALUES ($1, $2, $3, 'accepted', '{}', $4, $4, $5)`, org.ID, f.brandID, fmt.Sprintf("M495ABCDEFG%06d", f.seq), at, customer)
	f.exec(t, `INSERT INTO organization_product_stocks (organization_id, brand_id, product_id, quantity, meters)
VALUES ($1, $2, $3, 1, 0)
ON CONFLICT (organization_id, product_id) DO UPDATE SET quantity = organization_product_stocks.quantity + 1`, org.ID, f.brandID, f.product)
	stockUnit := f.id(t, `INSERT INTO units (organization_id, brand_id, product_id, barcode, status)
VALUES ($1, $2, $3, $4, 'available') RETURNING id`, f.center.ID, f.brandID, f.product, fmt.Sprintf("SU-%s-%d", f.prefix, f.seq))
	f.exec(t, `INSERT INTO unit_current_state (unit_id, brand_id, owner_type, owner_id, holder_org_id, status)
VALUES ($1, $2, 'organization', $3, $3, 'available')`, stockUnit, f.brandID, org.ID)
}

// caller builds the member of org with the union of system role grants.
func (f *fixture) caller(t *testing.T, org db.Organization, roles ...string) Caller {
	t.Helper()
	grants := map[string]rbac.Scope{}
	for _, role := range roles {
		def, ok := rbac.RoleBySlug(role)
		if !ok {
			t.Fatalf("role %s", role)
		}
		for perm, scope := range rbac.RoleGrants(def) {
			if prev, ok := grants[perm]; ok {
				scope = rbac.Broadest([]rbac.Scope{prev, scope})
			}
			grants[perm] = scope
		}
	}
	p := authctx.Principal{UserInternal: f.id(t, `INSERT INTO users (password_hash, name, surname, status, email)
VALUES ('x', 'm', 'T495', 'active', $1) RETURNING id`, fmt.Sprintf("%s-m%d@example.test", f.prefix, f.nextSeq())),
		Roles: roles, PermissionScopes: grants}
	for perm := range grants {
		p.Permissions = append(p.Permissions, perm)
	}
	return Caller{Principal: p, Org: orgctx.Scope{
		InternalID: org.ID, UUID: org.Uuid, Slug: org.Slug, Name: org.Name, OrgType: org.Type, BrandID: org.BrandID,
	}}
}

func (f *fixture) nextSeq() int { f.seq++; return f.seq }

func (f *fixture) svc(checker FeatureChecker) *Service {
	return New(f.q, checker).WithClock(func() time.Time { return testNow })
}

var in30 = Input{Fallback: i18n.Resolved{Locale: i18n.LocaleEN, Timezone: "UTC"}}

func sumSeries(env Envelope, key string) float64 {
	var sum float64
	for _, s := range env.Series {
		if key != "" && s.Key != key {
			continue
		}
		for _, p := range s.Points {
			sum += p.Value
		}
	}
	return sum
}

func card(env Envelope, key string) float64 {
	for _, s := range env.Series {
		if s.Key != "cards" {
			continue
		}
		for _, p := range s.Points {
			if p.Key == key {
				return p.Value
			}
		}
	}
	return -1
}

// TestReportsFollowScope: every report reads exactly the caller's reach —
// a dealer its own organization, a distributor its subtree, the center the
// brand.
func TestReportsFollowScope(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	callers := []struct {
		name string
		c    Caller
		want float64
	}{
		{"dealer", f.caller(t, f.dealer1, rbac.RoleDealerOwner), 1},
		{"distributor", f.caller(t, f.distA, rbac.RoleDistributorOwner), 3},
		{"center", f.caller(t, f.center, rbac.RoleCenterStaff, rbac.RoleCenterWarehouse), 6},
	}
	total := map[string]func(Envelope) float64{
		ReportServicesTrend:              func(e Envelope) float64 { return sumSeries(e, "created") },
		ReportServicesStatusDistribution: func(e Envelope) float64 { return sumSeries(e, "") },
		ReportServicesTopBrands:          func(e Envelope) float64 { return sumSeries(e, "") },
		ReportServicesTopModels:          func(e Envelope) float64 { return sumSeries(e, "") },
		ReportServicesTopProducts:        func(e Envelope) float64 { return sumSeries(e, "") },
		ReportOrdersTrend:                func(e Envelope) float64 { return sumSeries(e, "created") },
		ReportOrdersStatusDistribution:   func(e Envelope) float64 { return sumSeries(e, "") },
		ReportCustomersTrend:             func(e Envelope) float64 { return sumSeries(e, "") },
		ReportStockSummary:               func(e Envelope) float64 { return card(e, "stock_quantity") },
		ReportWarrantiesSummary:          func(e Envelope) float64 { return card(e, "warranties_active") },
		ReportMeasurementsSummary:        func(e Envelope) float64 { return card(e, "measurements_total") },
		ReportDealersTopByWarranty:       func(e Envelope) float64 { return sumSeries(e, "") },
		ReportActivitiesRecent:           func(e Envelope) float64 { return float64(len(e.Rows)) },
		ReportOverview:                   func(e Envelope) float64 { return card(e, "services_completed") },
	}
	for _, d := range Definitions {
		if d.Key == ReportDealersPerformance {
			continue // TestDealersPerformanceScope
		}
		get, ok := total[d.Key]
		if !ok {
			t.Fatalf("no scope check for %s", d.Key)
		}
		for _, cl := range callers {
			t.Run(d.Key+"/"+cl.name, func(t *testing.T) {
				in := in30
				if d.Limited {
					in.Limit = "50"
				}
				env, err := s.Report(f.ctx, cl.c, d.Key, in)
				if d.Network && cl.name == "dealer" {
					if !errors.Is(err, ErrForbidden) {
						t.Fatalf("network report for a dealer: err = %v, want ErrForbidden", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if got := get(env); got != cl.want {
					t.Fatalf("total = %v, want %v", got, cl.want)
				}
			})
		}
	}
	// Overview network cards: the distributor counts its subtree dealers.
	env, err := s.Report(f.ctx, callers[1].c, ReportOverview, in30)
	if err != nil {
		t.Fatal(err)
	}
	if got := card(env, "dealers_total"); got != 2 {
		t.Fatalf("distributor dealers_total = %v, want 2", got)
	}
	env, err = s.Report(f.ctx, callers[0].c, ReportOverview, in30)
	if err != nil {
		t.Fatal(err)
	}
	if got := card(env, "dealers_total"); got != -1 {
		t.Fatalf("dealer overview has network card %v", got)
	}
}

// TestDealerStaffLegacyMatrix: dealer staff lacks the owner-only reports of
// the legacy role-access matrix (dealers.performance, stock).
func TestDealerStaffLegacyMatrix(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	staff := f.caller(t, f.dealer1, rbac.RoleDealerStaff)
	for _, key := range []string{ReportDealersPerformance, ReportDealersTopByWarranty, ReportStockSummary} {
		if _, err := s.Report(f.ctx, staff, key, in30); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s: err = %v, want ErrForbidden", key, err)
		}
	}
	items, err := s.Catalog(f.ctx, staff, i18n.LocaleTR)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Key == ReportDealersPerformance || it.Key == ReportStockSummary {
			t.Fatalf("catalog of dealer staff lists %s", it.Key)
		}
	}
	env, err := s.Report(f.ctx, staff, ReportServicesTrend, in30)
	if err != nil || sumSeries(env, "created") != 1 {
		t.Fatalf("staff services.trend = %v / %v, want 1", sumSeries(env, "created"), err)
	}
}

func TestDealersPerformanceScope(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	for _, org := range []db.Organization{f.dealer1, f.dealer2, f.dealer3} {
		if _, err := f.q.UpsertPerformanceMetric(f.ctx, db.UpsertPerformanceMetricParams{
			OrganizationID: org.ID, BrandID: f.brandID, Period: "2026-10", Scope: "org", Metric: "services_count",
			Value: pgtype.Numeric{Int: bigInt(org.ID % 7), Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		c    Caller
		want []string
	}{
		{f.caller(t, f.dealer1, rbac.RoleDealerOwner), []string{f.dealer1.Name}},
		{f.caller(t, f.distA, rbac.RoleDistributorOwner), []string{f.dealer1.Name, f.dealer2.Name}},
		{f.caller(t, f.center, rbac.RoleCenterStaff, rbac.RoleCenterWarehouse), []string{f.dealer1.Name, f.dealer2.Name, f.dealer3.Name}},
	}
	for _, tc := range cases {
		env, err := s.Report(f.ctx, tc.c, ReportDealersPerformance, Input{Limit: "50", Fallback: in30.Fallback})
		if err != nil {
			t.Fatalf("%s: %v", tc.c.Org.OrgType, err)
		}
		var got []string
		for _, r := range env.Rows {
			if r["type"] == "dealer" {
				got = append(got, r["organization"].(string))
			}
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s rows = %v, want %v", tc.c.Org.OrgType, got, tc.want)
		}
	}
}

// TestDisabledModuleHiddenAndRejected: a report whose module is off is not
// in the catalog and answers ErrFeatureDisabled (403 FEATURE_DISABLED).
func TestDisabledModuleHiddenAndRejected(t *testing.T) {
	f := newFixture(t)
	owner := f.caller(t, f.dealer1, rbac.RoleDealerOwner)
	on := f.svc(featureMap{})
	off := f.svc(featureMap{features.ModulePerformance: false, features.ModuleMeasurements: false})
	keys := func(s *Service) map[string]bool {
		items, err := s.Catalog(f.ctx, owner, i18n.LocaleEN)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, it := range items {
			out[it.Key] = true
		}
		return out
	}
	all, some := keys(on), keys(off)
	for _, k := range []string{ReportDealersPerformance, ReportMeasurementsSummary} {
		if !all[k] {
			t.Fatalf("%s missing with the module on", k)
		}
		if some[k] {
			t.Fatalf("%s listed with the module off", k)
		}
		if _, err := off.Report(f.ctx, owner, k, in30); !errors.Is(err, ErrFeatureDisabled) {
			t.Fatalf("%s: err = %v, want ErrFeatureDisabled", k, err)
		}
	}
	if !some[ReportServicesTrend] || !some[ReportOverview] {
		t.Fatal("other reports must stay in the catalog")
	}
	env, err := off.Report(f.ctx, owner, ReportOverview, in30)
	if err != nil {
		t.Fatal(err)
	}
	if card(env, "measurements_total") != -1 {
		t.Fatal("overview shows measurement cards with the module off")
	}
}

// TestEnvelopeShapeSameForEveryReport: the table test of the shared
// envelope — every report marshals to the same top-level keys.
func TestEnvelopeShapeSameForEveryReport(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{})
	center := f.caller(t, f.center, rbac.RoleCenterStaff, rbac.RoleCenterWarehouse)
	want := []string{"columns", "generated_at", "granularity", "kind", "locale", "period", "range_from", "range_to", "report", "rows", "scope", "series", "timezone", "title"}
	for _, d := range Definitions {
		t.Run(d.Key, func(t *testing.T) {
			in := in30
			in.Locale = "tr"
			env, err := s.Report(f.ctx, center, d.Key, in)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(env)
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(m))
			for k := range m {
				got = append(got, k)
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("keys = %v, want %v", got, want)
			}
			for _, k := range []string{"series", "columns", "rows"} {
				if string(m[k])[0] != '[' {
					t.Fatalf("%s is not an array: %s", k, m[k])
				}
			}
			if env.Report != d.Key || env.Kind != d.Kind || env.Locale != "tr" || env.Title == "" || env.Title == "title."+d.Key {
				t.Fatalf("header = %+v", env)
			}
			if d.Periodic != (env.Period != nil && env.RangeFrom != nil && env.RangeTo != nil) {
				t.Fatalf("periodic %v but period=%v range=%v..%v", d.Periodic, env.Period, env.RangeFrom, env.RangeTo)
			}
			if d.Granular != (env.Granularity != nil) {
				t.Fatalf("granular %v but granularity=%v", d.Granular, env.Granularity)
			}
			for _, sr := range env.Series {
				if sr.Label == "" || sr.Label == "series."+sr.Key {
					t.Fatalf("series %s has no label", sr.Key)
				}
			}
		})
	}
}

// TestLayoutPutIsAtomic: one invalid widget rejects the whole layout and
// the stored one stays unchanged.
func TestLayoutPutIsAtomic(t *testing.T) {
	f := newFixture(t)
	s := f.svc(featureMap{features.ModulePerformance: false})
	owner := f.caller(t, f.dealer1, rbac.RoleDealerOwner)

	l, err := s.GetLayout(f.ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Widgets) != 1 || l.Widgets[0].Report != ReportOverview || l.UpdatedAt != nil {
		t.Fatalf("default layout = %+v", l)
	}
	p30, week := "30d", "week"
	good := LayoutInput{Widgets: []WidgetInput{
		{ID: "550e8400-e29b-41d4-a716-446655440000", Report: ReportServicesTrend, Period: &p30, Granularity: &week},
		{ID: "660e8400-e29b-41d4-a716-446655440001", Report: ReportOverview},
	}}
	saved, err := s.PutLayout(f.ctx, owner, good)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Widgets) != 2 || saved.Widgets[1].SortOrder != 1 {
		t.Fatalf("saved = %+v", saved)
	}
	bad := []struct {
		name  string
		in    WidgetInput
		field string
	}{
		{"unknown report", WidgetInput{ID: "770e8400-e29b-41d4-a716-446655440002", Report: "services.nope"}, "widgets[2].report"},
		{"module off", WidgetInput{ID: "770e8400-e29b-41d4-a716-446655440002", Report: ReportDealersPerformance}, "widgets[2].report"},
		{"network for dealer", WidgetInput{ID: "770e8400-e29b-41d4-a716-446655440002", Report: ReportDealersTopByWarranty}, "widgets[2].report"},
		{"duplicate report", WidgetInput{ID: "770e8400-e29b-41d4-a716-446655440002", Report: ReportOverview}, "widgets[2].report"},
		{"duplicate id", WidgetInput{ID: "550e8400-e29b-41d4-a716-446655440000", Report: ReportOrdersTrend}, "widgets[2].id"},
		{"period on snapshot", WidgetInput{ID: "770e8400-e29b-41d4-a716-446655440002", Report: ReportStockSummary, Period: &p30}, "widgets[2].period"},
	}
	for _, b := range bad {
		in := LayoutInput{Widgets: append(append([]WidgetInput{}, good.Widgets[0], good.Widgets[1]), b.in)}
		in.Widgets[0].Report = ReportOrdersTrend // would change the stored layout if applied
		_, err := s.PutLayout(f.ctx, owner, in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != b.field {
			t.Fatalf("%s: err = %v, want validation on %s", b.name, err, b.field)
		}
		got, err := s.GetLayout(f.ctx, owner)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Widgets) != 2 || got.Widgets[0].Report != ReportServicesTrend || got.Widgets[1].Report != ReportOverview {
			t.Fatalf("%s: stored layout changed: %+v", b.name, got.Widgets)
		}
	}
	// A report that closes later disappears from GET.
	closed := f.svc(featureMap{features.ModuleServices: false, features.ModulePerformance: false})
	got, err := closed.GetLayout(f.ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range got.Widgets {
		if w.Report == ReportServicesTrend {
			t.Fatal("closed report still in layout")
		}
	}
}

func TestResolveQuery(t *testing.T) {
	s := (&Service{now: func() time.Time { return testNow }})
	trend, _ := DefinitionByKey(ReportServicesTrend)
	stock, _ := DefinitionByKey(ReportStockSummary)
	fb := in30.Fallback
	cases := []struct {
		name  string
		d     Definition
		in    Input
		field string
		gran  string
		n     int
	}{
		{"default 30d day", trend, Input{Fallback: fb}, "", "day", 30},
		{"7d", trend, Input{Period: "7d", Fallback: fb}, "", "day", 7},
		{"90d week", trend, Input{Period: "90d", Fallback: fb}, "", "week", 14},
		{"12m month", trend, Input{Period: "12m", Fallback: fb}, "", "month", 12},
		{"custom", trend, Input{RangeFrom: "2026-09-01", RangeTo: "2026-09-30", Fallback: fb}, "", "day", 30},
		{"bad period", trend, Input{Period: "5y", Fallback: fb}, "period", "", 0},
		{"half range", trend, Input{RangeFrom: "2026-09-01", Fallback: fb}, "range_from", "", 0},
		{"reverse range", trend, Input{RangeFrom: "2026-09-02", RangeTo: "2026-09-01", Fallback: fb}, "range_to", "", 0},
		{"day too long", trend, Input{RangeFrom: "2025-01-01", RangeTo: "2026-09-01", Granularity: "day", Fallback: fb}, "granularity", "", 0},
		{"bad locale", trend, Input{Locale: "xx", Fallback: fb}, "locale", "", 0},
		{"snapshot period", stock, Input{Period: "7d", Fallback: fb}, "period", "", 0},
		{"snapshot granularity", stock, Input{Granularity: "day", Fallback: fb}, "granularity", "", 0},
		{"no limit", trend, Input{Limit: "5", Fallback: fb}, "limit", "", 0},
	}
	for _, tc := range cases {
		q, err := s.resolveQuery(tc.d, tc.in)
		if tc.field != "" {
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("%s: err = %v, want field %s", tc.name, err, tc.field)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if q.granularity != tc.gran || len(q.buckets()) != tc.n {
			t.Fatalf("%s: granularity %s buckets %d, want %s %d", tc.name, q.granularity, len(q.buckets()), tc.gran, tc.n)
		}
	}
}

func TestLabelsCompleteInEveryLocale(t *testing.T) {
	if len(labelLocales) != len(i18n.Supported) {
		t.Fatalf("label locales %d, supported %d", len(labelLocales), len(i18n.Supported))
	}
	for k, l := range labels {
		for i, v := range l {
			if v == "" {
				t.Errorf("%s: empty %s", k, labelLocales[i])
			}
		}
	}
	keys := []string{}
	for _, d := range Definitions {
		keys = append(keys, "title."+d.Key)
	}
	for _, list := range []struct {
		prefix string
		values []string
	}{
		{"services.status.", serviceStatuses}, {"orders.status.", orderStatuses}, {"warranties.status.", warrantyStatuses},
		{"units.status.", unitStatuses}, {"measurements.status.", measurementStatuses}, {"series.", customerTypes},
		{"column.", performanceColumns},
	} {
		for _, v := range list.values {
			keys = append(keys, list.prefix+v)
		}
	}
	for _, k := range keys {
		if _, ok := labels[k]; !ok {
			t.Errorf("missing label %s", k)
		}
	}
}

func bigInt(n int64) *big.Int { return big.NewInt(n) }
