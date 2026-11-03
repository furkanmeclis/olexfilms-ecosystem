package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-1043: sort=popularity and /v1/stats/frequent-vehicles on real rows: the
// organization's own usage weighs more than the brand's, an organization
// without usage still gets the brand's order, cancelled services do not
// count and another organization's counts never show.
func TestIntegrationPopularityOrder(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	s := it.suffix
	var orgIDs []int64
	t.Cleanup(func() {
		_, _ = it.pool.Exec(ctx, "DELETE FROM services WHERE organization_id = ANY($1)", orgIDs)
		_, _ = it.pool.Exec(ctx, "DELETE FROM vehicles WHERE organization_id = ANY($1)", orgIDs)
		_, _ = it.pool.Exec(ctx, "DELETE FROM car_models WHERE name LIKE '%' || $1", s)
		_, _ = it.pool.Exec(ctx, "DELETE FROM car_brands WHERE name LIKE '%' || $1", s)
	})
	admin, apw := it.user("t1043-admin", rbac.RoleSuperAdmin)
	adminTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": apw,
	})).AccessToken
	type car struct{ brandID, modelID int64 }
	carOf := func(name string) car {
		code, env := it.do("POST", "/v1/platform/vehicle-catalog/brands", hostOlex, adminTok, map[string]any{"name": name + " " + s})
		if code != http.StatusCreated {
			t.Fatalf("brand %s: %d %s", name, code, errCode(env))
		}
		var b listItem
		_ = json.Unmarshal(env.Data, &b)
		code, env = it.do("POST", "/v1/platform/vehicle-catalog/models", hostOlex, adminTok, map[string]any{"brand_uuid": b.UUID, "name": name + "-m " + s})
		if code != http.StatusCreated {
			t.Fatalf("model %s: %d %s", name, code, errCode(env))
		}
		var c car
		if err := it.pool.QueryRow(ctx, `SELECT b.id, m.id FROM car_brands b JOIN car_models m ON m.car_brand_id = b.id WHERE b.name = $1`, name+" "+s).Scan(&c.brandID, &c.modelID); err != nil {
			t.Fatal(err)
		}
		return c
	}
	alfa, beta, gama := carOf("Alfa"), carOf("Beta"), carOf("Gama")

	// The dealers live under Glorian: earlier integration tests leave olex
	// services behind in the shared test database, and the frequent list keeps
	// only the first FrequentLimit brands of the whole domain brand.
	const host = hostGlorian
	center := it.brandCenter("glorian")
	dist := it.org("t1043-dist", "distributor", center)
	dealer := func(name string) (db.Organization, string) {
		org := it.org(name, "dealer", dist)
		orgIDs = append(orgIDs, org.ID)
		u, pw := it.user(name + "-owner")
		it.member(org, u, "owner")
		return org, it.catalogLogin(u, pw, org, host)
	}
	one, oneTok := dealer("t1043-one")
	two, twoTok := dealer("t1043-two")
	_, newTok := dealer("t1043-new")
	customer, _ := it.user("t1043-customer")

	service := func(org db.Organization, c car, status string) {
		t.Helper()
		var vehicleID int64
		if err := it.pool.QueryRow(ctx, `INSERT INTO vehicles (user_id, organization_id, brand_id, car_brand_id, car_model_id)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`, customer.ID, org.ID, org.BrandID, c.brandID, c.modelID).Scan(&vehicleID); err != nil {
			t.Fatalf("vehicle: %v", err)
		}
		if _, err := it.pool.Exec(ctx, `INSERT INTO services (service_no, organization_id, brand_id, customer_user_id,
			vehicle_id, car_brand_id, car_model_id, status, completed_at, cancelled_at)
			VALUES ('T1043' || right(gen_random_uuid()::text, 8), $1, $2, $3, $4, $5, $6, $7::text,
				CASE WHEN $7::text = 'completed' THEN now() END,
				CASE WHEN $7::text = 'cancelled' THEN now() END)`,
			org.ID, org.BrandID, customer.ID, vehicleID, c.brandID, c.modelID, status); err != nil {
			t.Fatalf("service: %v", err)
		}
	}
	// One: Gama once. Two: Beta five times, cancelled Alfa ten times.
	service(one, gama, "completed")
	for range 5 {
		service(two, beta, "completed")
	}
	for range 10 {
		service(two, alfa, "cancelled")
	}

	order := func(token string) string {
		t.Helper()
		names, _, _ := it.listNames("/v1/vehicle-catalog/brands?sort=popularity&q="+url.QueryEscape(s), host, token, http.StatusOK, false)
		for i, n := range names {
			names[i] = strings.TrimSuffix(n, " "+s)
		}
		return strings.Join(names, ",")
	}
	// One: Gama 0.6 + 0.4*1/5 = 0.68, Beta 0.4, Alfa 0.
	if got := order(oneTok); got != "Gama,Beta,Alfa" {
		t.Errorf("one order = %s", got)
	}
	// Two: Beta 1.0, Gama 0.08, Alfa 0 (its cancelled services do not count).
	if got := order(twoTok); got != "Beta,Gama,Alfa" {
		t.Errorf("two order = %s", got)
	}
	// No usage of its own: the brand's order.
	if got := order(newTok); got != "Beta,Gama,Alfa" {
		t.Errorf("new org order = %s", got)
	}

	frequent := func(token string) map[string]int64 {
		t.Helper()
		code, env := it.do("GET", "/v1/stats/frequent-vehicles", host, token, nil)
		if code != http.StatusOK {
			t.Fatalf("frequent: %d %s", code, errCode(env))
		}
		var out struct {
			Brands []struct {
				Name         string `json:"name"`
				ServiceCount int64  `json:"service_count"`
			} `json:"brands"`
		}
		if err := json.Unmarshal(env.Data, &out); err != nil {
			t.Fatal(err)
		}
		counts := map[string]int64{}
		for _, b := range out.Brands {
			if name, ok := strings.CutSuffix(b.Name, " "+s); ok {
				counts[name] = b.ServiceCount
			}
		}
		return counts
	}
	// A new organization sees the brand's popular cars, with its own count 0.
	got := frequent(newTok)
	if len(got) != 2 || got["Beta"] != 0 || got["Gama"] != 0 {
		t.Errorf("new org frequent = %v", got)
	}
	if _, ok := got["Alfa"]; ok {
		t.Errorf("cancelled-only brand in frequent: %v", got)
	}
	// One never sees Two's count of Beta.
	got = frequent(oneTok)
	if got["Gama"] != 1 || got["Beta"] != 0 {
		t.Errorf("one frequent = %v", got)
	}

	// Other sorts stay as they were.
	if names, _, _ := it.listNames("/v1/vehicle-catalog/brands?q="+url.QueryEscape(s), host, oneTok, http.StatusOK, false); len(names) != 3 || !strings.HasPrefix(names[0], "Alfa") {
		t.Errorf("default order = %v", names)
	}
	if _, _, code := it.listNames("/v1/vehicle-catalog/brands?sort=-popularity", host, oneTok, http.StatusBadRequest, false); code == "" {
		t.Error("-popularity: no error code")
	}
}
