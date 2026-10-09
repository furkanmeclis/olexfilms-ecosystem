package usecase

import (
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/performance/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/jackc/pgx/v5/pgtype"
)

const mapTestPeriod = "2026-09"

func TestRegionMapProvinceCountsEmptyAndDistributorScope(t *testing.T) {
	f := newPerfFixture(t)
	svc := New(nil, f.q, nil, nil, nil)
	tr := f.country(t, "TR")
	p34 := f.province(t, tr, "34")
	p35 := f.province(t, tr, "35")
	p06 := f.province(t, tr, "06")

	dist2 := f.org(t, "distributor", "dist-two", f.center.ID)
	f.territory(t, f.dist.ID, tr, p34, 0)
	f.territory(t, f.dist.ID, tr, p35, 0)
	f.territory(t, dist2.ID, tr, p06, 0)

	d1 := f.org(t, "dealer", "map-one", f.dist.ID)
	d2 := f.org(t, "dealer", "map-two", f.dist.ID)
	d3 := f.org(t, "dealer", "map-missing", f.dist.ID)
	foreign := f.org(t, "dealer", "map-foreign", dist2.ID)
	f.place(t, d1.ID, tr, p34, 41.01, 28.97)
	f.place(t, d2.ID, tr, p34, 41.02, 28.98)
	f.place(t, d3.ID, tr, p34, 0, 0)
	f.place(t, foreign.ID, tr, p06, 39.92, 32.85)
	f.mapMetric(t, d1.ID, mapTestPeriod, model.MetricServicesCount, "2")
	f.mapMetric(t, d2.ID, mapTestPeriod, model.MetricServicesCount, "4")
	f.mapMetric(t, d3.ID, mapTestPeriod, model.MetricServicesCount, "6")
	f.mapMetric(t, foreign.ID, mapTestPeriod, model.MetricServicesCount, "99")

	got, err := svc.RegionMap(f.ctx, mapCallerFor(f.dist), MapFilter{
		Level: LevelProvince, CountryISO: "TR", Period: mapTestPeriod, Metric: model.MetricServicesCount,
	})
	if err != nil {
		t.Fatalf("region map: %v", err)
	}
	p34Item := findRegion(t, got.Items, "34")
	if p34Item.DealerCount != 3 {
		t.Fatalf("Istanbul dealer_count = %d, want 3", p34Item.DealerCount)
	}
	if p34Item.MissingCoordinates != 1 {
		t.Fatalf("Istanbul missing = %d, want 1", p34Item.MissingCoordinates)
	}
	if p34Item.MetricAvg == nil || *p34Item.MetricAvg != 4 {
		t.Fatalf("Istanbul metric avg = %v, want 4", p34Item.MetricAvg)
	}
	if findRegionOrNil(got.Items, "06") != nil {
		t.Fatalf("distributor saw another distributor territory")
	}
	empty := findEmpty(t, got.EmptyRegions, "35")
	if empty.Reason != "territory_no_dealers" || empty.Distributor == nil || empty.Distributor.Name != f.dist.Name {
		t.Fatalf("empty territory = %+v", empty)
	}

	points, err := svc.DealerMap(f.ctx, mapCallerFor(f.dist), MapFilter{
		CountryISO: "TR", Period: mapTestPeriod, Metric: model.MetricServicesCount,
	})
	if err != nil {
		t.Fatalf("dealer map: %v", err)
	}
	if len(points.Items) != 2 {
		t.Fatalf("dealer points = %d, want 2", len(points.Items))
	}
	if points.MissingCoordinates != 1 {
		t.Fatalf("dealer points missing = %d, want 1", points.MissingCoordinates)
	}
}

func TestGeoCentroidsSeedHas81TRProvinces(t *testing.T) {
	f := newPerfFixture(t)
	var n int
	if err := f.tx.QueryRow(f.ctx, `
SELECT COUNT(*)
FROM provinces p
JOIN countries c ON c.id = p.country_id
WHERE c.iso2 = 'TR' AND p.latitude IS NOT NULL AND p.longitude IS NOT NULL`).Scan(&n); err != nil {
		t.Fatalf("seed count: %v", err)
	}
	if n != 81 {
		t.Fatalf("TR province centroids = %d, want 81", n)
	}
}

func (f *perfFixture) country(t *testing.T, iso2 string) int64 {
	t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, `SELECT id FROM countries WHERE iso2 = $1`, iso2).Scan(&id); err != nil {
		t.Fatalf("country %s: %v", iso2, err)
	}
	return id
}

func (f *perfFixture) province(t *testing.T, country int64, code string) int64 {
	t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, `SELECT id FROM provinces WHERE country_id = $1 AND code = $2`, country, code).Scan(&id); err != nil {
		t.Fatalf("province %s: %v", code, err)
	}
	return id
}

func (f *perfFixture) territory(t *testing.T, org, country, province, district int64) {
	t.Helper()
	_, err := f.tx.Exec(f.ctx, `
INSERT INTO territories (brand_id, organization_id, country_id, province_id, district_id)
VALUES ($1, $2, $3, NULLIF($4, 0), NULLIF($5, 0))`, f.brandID, org, country, province, district)
	if err != nil {
		t.Fatalf("territory: %v", err)
	}
}

func (f *perfFixture) place(t *testing.T, org, country, province int64, lat, lon float64) {
	t.Helper()
	var latv, lonv any
	if lat != 0 || lon != 0 {
		latv, lonv = lat, lon
	}
	if _, err := f.tx.Exec(f.ctx, `
UPDATE organizations
SET country_id = $2, province_id = $3, latitude = $4, longitude = $5
WHERE id = $1`, org, country, province, latv, lonv); err != nil {
		t.Fatalf("place org: %v", err)
	}
}

func (f *perfFixture) mapMetric(t *testing.T, orgID int64, period, metric, value string) {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(value); err != nil {
		t.Fatalf("numeric %s: %v", value, err)
	}
	if _, err := f.q.UpsertPerformanceMetric(f.ctx, db.UpsertPerformanceMetricParams{
		OrganizationID: orgID, BrandID: f.brandID, Period: period, Scope: ScopeOrg, Metric: metric, Value: n,
	}); err != nil {
		t.Fatalf("metric %s=%s: %v", metric, value, err)
	}
}

func mapCallerFor(org db.Organization) Caller {
	return Caller{Org: orgctx.Scope{
		InternalID: org.ID, UUID: org.Uuid, Slug: org.Slug, Name: org.Name,
		OrgType: org.Type, BrandID: org.BrandID,
	}}
}

func findRegion(t *testing.T, items []RegionItem, code string) RegionItem {
	t.Helper()
	if got := findRegionOrNil(items, code); got != nil {
		return *got
	}
	t.Fatalf("region %s not found", code)
	return RegionItem{}
}

func findRegionOrNil(items []RegionItem, code string) *RegionItem {
	for i := range items {
		if items[i].Code == code {
			return &items[i]
		}
	}
	return nil
}

func findEmpty(t *testing.T, items []EmptyRegion, code string) EmptyRegion {
	t.Helper()
	for _, item := range items {
		if item.Code == code {
			return item
		}
	}
	t.Fatalf("empty %s not found in %+v", code, items)
	return EmptyRegion{}
}

func TestParseMapFilterValidation(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	got, err := ParseMapFilter(nil, now)
	if err != nil {
		t.Fatalf("default filter: %v", err)
	}
	if got.Level != LevelProvince || got.Period != "2026-10" || got.Metric != model.MetricServicesCount {
		t.Fatalf("defaults = %+v", got)
	}
}
