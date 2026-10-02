package db_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	svcuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-151: top-10 car brand / model statistics (service_stats.sql). Reuses
// the service fixture (rolled-back transaction).

func (f *serviceFixture) carBrandNamed(t *testing.T, name string) int64 {
	t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_brands (name) VALUES ($1) RETURNING id`,
		fmt.Sprintf("%s %d", name, time.Now().UnixNano())).Scan(&id); err != nil {
		t.Fatalf("car brand %s: %v", name, err)
	}
	return id
}

func (f *serviceFixture) carModelNamed(t *testing.T, carBrand int64, name string) int64 {
	t.Helper()
	var id int64
	if err := f.tx.QueryRow(f.ctx, `INSERT INTO car_models (car_brand_id, name) VALUES ($1, $2) RETURNING id`,
		carBrand, name).Scan(&id); err != nil {
		t.Fatalf("car model %s: %v", name, err)
	}
	return id
}

// statService creates a service of org for the car model and moves it to
// status (completed at completedAt, or cancelled; draft stays as is).
func (f *serviceFixture) statService(t *testing.T, org db.Organization, carBrand, carModel int64, status string, completedAt time.Time) {
	t.Helper()
	arg := f.serviceParams(org)
	arg.CarBrandID, arg.CarModelID = carBrand, carModel
	s, err := f.q.CreateService(f.ctx, arg)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	switch status {
	case "completed":
		_, err = f.tx.Exec(f.ctx, `UPDATE services SET status = 'completed', completed_at = $2 WHERE id = $1`, s.ID, completedAt)
	case "cancelled":
		_, err = f.tx.Exec(f.ctx, `UPDATE services SET status = 'cancelled', cancelled_at = NOW() WHERE id = $1`, s.ID)
	}
	if err != nil {
		t.Fatalf("service -> %s: %v", status, err)
	}
}

func sinceArg(t *testing.T, period string, now time.Time) pgtype.Timestamptz {
	t.Helper()
	since, err := svcuc.StatsSince(period, now)
	if err != nil {
		t.Fatal(err)
	}
	if since == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *since, Valid: true}
}

func modelNames(rows []db.TopServicedCarModelsRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s:%d", r.CarModelName, r.ServiceCount))
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTopServicedCarModelsPeriodsAndScope(t *testing.T) {
	f := newServiceFixture(t)
	now := time.Now().UTC()
	day := 24 * time.Hour
	p := f.carBrandNamed(t, "T151 P")
	p1 := f.carModelNamed(t, p, "P1")
	p2 := f.carModelNamed(t, p, "P2")
	p3 := f.carModelNamed(t, p, "P3")
	p4 := f.carModelNamed(t, p, "P4")
	olex := f.dealer
	f.statService(t, olex, p, p1, "completed", now.Add(-5*day))
	f.statService(t, olex, p, p1, "completed", now.Add(-6*day))
	f.statService(t, olex, p, p2, "completed", now.Add(-40*day))
	f.statService(t, olex, p, p3, "completed", now.Add(-200*day))
	f.statService(t, olex, p, p4, "completed", now.Add(-400*day))
	// Not completed: never counted.
	f.statService(t, olex, p, p1, "draft", time.Time{})
	f.statService(t, olex, p, p4, "cancelled", time.Time{})
	f.statService(t, olex, p, p4, "draft", time.Time{})

	// Another domain brand: its completed services stay out of olex.
	glorian := brandCenter(t, f.ctx, f.q, "glorian")
	g := f.carBrandNamed(t, "T151 G")
	g1 := f.carModelNamed(t, g, "G1")
	for range 5 {
		f.statService(t, glorian, g, g1, "completed", now.Add(-day))
	}

	for _, tc := range []struct {
		period string
		want   []string
	}{
		{svcuc.StatsPeriod30d, []string{"P1:2"}},
		{svcuc.StatsPeriod90d, []string{"P1:2", "P2:1"}},
		{svcuc.StatsPeriod12m, []string{"P1:2", "P2:1", "P3:1"}},
		{svcuc.StatsPeriodAll, []string{"P1:2", "P2:1", "P3:1", "P4:1"}},
	} {
		rows, err := f.q.TopServicedCarModels(f.ctx, db.TopServicedCarModelsParams{
			BrandID: olex.BrandID, Since: sinceArg(t, tc.period, now),
		})
		if err != nil {
			t.Fatalf("%s: %v", tc.period, err)
		}
		if got := modelNames(rows); !equalStrings(got, tc.want) {
			t.Fatalf("period %s = %v, want %v", tc.period, got, tc.want)
		}
	}

	brands, err := f.q.TopServicedCarBrands(f.ctx, db.TopServicedCarBrandsParams{BrandID: olex.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	if len(brands) != 1 || brands[0].ServiceCount != 5 {
		t.Fatalf("olex brands = %+v, want one car brand with 5 completed services", brands)
	}

	rows, err := f.q.TopServicedCarModels(f.ctx, db.TopServicedCarModelsParams{BrandID: glorian.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	if got := modelNames(rows); !equalStrings(got, []string{"G1:5"}) {
		t.Fatalf("glorian models = %v, want [G1:5]", got)
	}
}

func TestTopServicedCarModelsTopTen(t *testing.T) {
	f := newServiceFixture(t)
	now := time.Now().UTC()
	x := f.carBrandNamed(t, "T151 X")
	y := f.carBrandNamed(t, "T151 Y")
	type model struct {
		brand int64
		name  string
		count int
	}
	// x1..x8: 12..5, y1 and y2 tie at 4 (name order), y3: 3 and y4: 2 fall
	// outside the top 10.
	var models []model
	for i := 1; i <= 8; i++ {
		models = append(models, model{x, fmt.Sprintf("X%d", i), 13 - i})
	}
	models = append(models, model{y, "Y2", 4}, model{y, "Y1", 4}, model{y, "Y3", 3}, model{y, "Y4", 2})
	for _, m := range models {
		id := f.carModelNamed(t, m.brand, m.name)
		for i := range m.count {
			f.statService(t, f.dealer, m.brand, id, "completed", now.Add(-time.Duration(i+1)*time.Hour))
		}
	}

	rows, err := f.q.TopServicedCarModels(f.ctx, db.TopServicedCarModelsParams{
		BrandID: f.dealer.BrandID, Since: sinceArg(t, svcuc.StatsPeriod30d, now),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"X1:12", "X2:11", "X3:10", "X4:9", "X5:8", "X6:7", "X7:6", "X8:5", "Y1:4", "Y2:4"}
	if got := modelNames(rows); !equalStrings(got, want) {
		t.Fatalf("top 10 = %v, want %v", got, want)
	}

	// group=brand: X (12+...+5 = 68) before Y (4+4+3+2 = 13).
	brands, err := f.q.TopServicedCarBrands(f.ctx, db.TopServicedCarBrandsParams{BrandID: f.dealer.BrandID})
	if err != nil {
		t.Fatal(err)
	}
	if len(brands) != 2 || brands[0].ServiceCount != 68 || brands[1].ServiceCount != 13 {
		t.Fatalf("brands = %+v, want X:68, Y:13", brands)
	}
}
