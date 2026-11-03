package popularity

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/redis/go-redis/v9"
)

// fakeQuerier answers from rows keyed by (brand, org): org 0 is the
// brand-wide (general) count. It records every call.
type fakeQuerier struct {
	brands map[[2]int64]Counts
	models map[[2]int64]Counts
	calls  []db.PopularityCarBrandCountsParams
	mcalls []db.PopularityCarModelCountsParams
}

func rowsOf(c Counts) []db.PopularityCarBrandCountsRow {
	out := []db.PopularityCarBrandCountsRow{}
	for id, n := range c {
		out = append(out, db.PopularityCarBrandCountsRow{ID: id, UseCount: n})
	}
	return out
}

func (f *fakeQuerier) PopularityCarBrandCounts(_ context.Context, arg db.PopularityCarBrandCountsParams) ([]db.PopularityCarBrandCountsRow, error) {
	f.calls = append(f.calls, arg)
	return rowsOf(f.brands[[2]int64{arg.BrandID, arg.OrgID.Int64}]), nil
}

func (f *fakeQuerier) PopularityCarModelCounts(_ context.Context, arg db.PopularityCarModelCountsParams) ([]db.PopularityCarModelCountsRow, error) {
	f.mcalls = append(f.mcalls, arg)
	out := []db.PopularityCarModelCountsRow{}
	for id, n := range f.models[[2]int64{arg.BrandID, arg.OrgID.Int64}] {
		out = append(out, db.PopularityCarModelCountsRow{ID: id, UseCount: n})
	}
	return out, nil
}

// Two domain brands; brand 1 has organizations 10 and 11.
func newFake() *fakeQuerier {
	return &fakeQuerier{brands: map[[2]int64]Counts{
		{1, 0}:  {100: 6, 200: 4},
		{1, 10}: {100: 1},
		{1, 11}: {200: 3},
		{2, 0}:  {300: 9},
	}}
}

// The organization part only ever reads the caller's organization and the
// result carries only its counts: another organization's count never leaks.
func TestNoCrossOrganizationLeak(t *testing.T) {
	q := newFake()
	res, err := New(q, nil).CarBrands(context.Background(), Scope{BrandID: 1, OrgID: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.OrgCounts) != 1 || res.OrgCounts[100] != 1 {
		t.Fatalf("org counts = %v, want only organization 10's", res.OrgCounts)
	}
	if _, ok := res.OrgCounts[200]; ok {
		t.Fatal("organization 11's usage is in the org counts")
	}
	for _, c := range q.calls {
		if c.OrgID.Valid && c.OrgID.Int64 != 10 {
			t.Fatalf("queried organization %d", c.OrgID.Int64)
		}
	}
	// 200 is ranked by the general count only (other organizations), so
	// 100 = 0.6*1 + 0.4*6/6 = 1.0, 200 = 0.4*4/6.
	if !near(res.Scores[100], 1) || !near(res.Scores[200], 0.4*4/6) {
		t.Fatalf("scores = %v", res.Scores)
	}
}

// "General" stays inside the domain brand (K20).
func TestBrandBoundary(t *testing.T) {
	q := newFake()
	res, err := New(q, nil).CarBrands(context.Background(), Scope{BrandID: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Scores[100]; ok {
		t.Fatalf("brand 1 usage scored for brand 2: %v", res.Scores)
	}
	if !near(res.Scores[300], 1) {
		t.Fatalf("scores = %v", res.Scores)
	}
	for _, c := range q.calls {
		if c.BrandID != 2 {
			t.Fatalf("queried brand %d", c.BrandID)
		}
	}
	if _, err := New(q, nil).CarBrands(context.Background(), Scope{}); err == nil {
		t.Fatal("scope without a brand was accepted")
	}
}

// Without an organization only the general count is read.
func TestNoOrgReadsGeneralOnly(t *testing.T) {
	q := newFake()
	res, err := New(q, nil).CarBrands(context.Background(), Scope{BrandID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.calls) != 1 || q.calls[0].OrgID.Valid {
		t.Fatalf("calls = %+v, want one brand-wide query", q.calls)
	}
	if len(res.OrgCounts) != 0 || !near(res.Scores[100], 1) || !near(res.Scores[200], 4.0/6) {
		t.Fatalf("result = %+v", res)
	}
}

func TestModelsKeyedByCarBrand(t *testing.T) {
	q := newFake()
	q.models = map[[2]int64]Counts{{1, 0}: {7: 2}, {1, 10}: {7: 1}}
	res, err := New(q, nil).CarModels(context.Background(), Scope{BrandID: 1, OrgID: 10, UserID: 5}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !near(res.Scores[7], 1) {
		t.Fatalf("scores = %v", res.Scores)
	}
	for _, c := range q.mcalls {
		if c.CarBrandID != 42 || c.BrandID != 1 {
			t.Fatalf("call = %+v", c)
		}
		if c.OrgID.Valid != c.CreatedByUserID.Valid {
			t.Fatalf("user narrowing on the general count: %+v", c)
		}
	}
}

func TestKeyHasBrandAndOrganization(t *testing.T) {
	cases := map[string]Key{
		"v1:car_brands:b1:oall":        {List: CarBrands, BrandID: 1},
		"v1:car_brands:b1:o10":         {List: CarBrands, BrandID: 1, OrgID: 10},
		"v1:car_brands:b2:o10":         {List: CarBrands, BrandID: 2, OrgID: 10},
		"v1:car_models:b1:o10:u5:cb42": {List: CarModels, BrandID: 1, OrgID: 10, UserID: 5, CarBrandID: 42},
		"v1:car_models:b1:oall:cb42":   {List: CarModels, BrandID: 1, CarBrandID: 42},
	}
	for want, k := range cases {
		if got := k.String(); got != want {
			t.Fatalf("key = %q, want %q", got, want)
		}
	}
}

// Counts are served from the cache (Redis) until CacheTTL; keys are per
// brand and organization, so another brand or organization misses.
func TestRedisCache(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := NewRedisCache(rdb, "test", nil)
	q := newFake()
	svc := New(q, cache)
	ctx := context.Background()

	if _, err := svc.CarBrands(ctx, Scope{BrandID: 1, OrgID: 10}); err != nil {
		t.Fatal(err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("first call queries = %d, want 2", len(q.calls))
	}
	if _, err := svc.CarBrands(ctx, Scope{BrandID: 1, OrgID: 10}); err != nil {
		t.Fatal(err)
	}
	if len(q.calls) != 2 {
		t.Fatalf("cached call queried again (%d)", len(q.calls))
	}
	if ttl := mr.TTL("test:pop:v1:car_brands:b1:o10"); ttl != CacheTTL {
		t.Fatalf("ttl = %v, want %v", ttl, CacheTTL)
	}
	// Organization 11 shares the general counts but not organization 10's.
	res, err := svc.CarBrands(ctx, Scope{BrandID: 1, OrgID: 11})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.calls) != 3 || res.OrgCounts[200] != 3 || res.OrgCounts[100] != 0 {
		t.Fatalf("calls = %d, org counts = %v", len(q.calls), res.OrgCounts)
	}
	mr.FastForward(CacheTTL)
	if _, err := svc.CarBrands(ctx, Scope{BrandID: 1, OrgID: 10}); err != nil {
		t.Fatal(err)
	}
	if len(q.calls) != 5 {
		t.Fatalf("expired cache not reloaded (%d queries)", len(q.calls))
	}
	// An unreachable Redis reads as a miss; the database answers.
	mr.Close()
	if _, err := svc.CarBrands(ctx, Scope{BrandID: 2}); err != nil {
		t.Fatalf("redis down: %v", err)
	}
}
