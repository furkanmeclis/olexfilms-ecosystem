package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legacymobile"
)

var legacyFixtureDir = filepath.Join("..", "modules", "legacymobile", legacymobile.FixtureDir)

// legacyDo sends a contract fixture as the old app would: its headers and
// body, no X-Mobile-Api-Version header.
func (it *itest) legacyDo(name string, vars map[string]string) (legacymobile.Fixture, int, []byte) {
	it.t.Helper()
	f, err := legacymobile.LoadFixture(legacyFixtureDir, name, vars)
	if err != nil {
		it.t.Fatal(err)
	}
	var body *bytes.Reader
	if len(f.Request.Body) > 0 && string(f.Request.Body) != "null" {
		body = bytes.NewReader(f.Request.Body)
	} else {
		body = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(f.Method, f.Path, body)
	req.Host = "backend:8080"
	req.Header.Set("X-Forwarded-Host", hostOlex)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "okhttp/4.9.2")
	for k, v := range f.Request.Headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	return f, rec.Code, rec.Body.Bytes()
}

// legacyExpect checks the answer against the fixture (status, old
// envelope, message, data keys).
func (it *itest) legacyExpect(name string, vars map[string]string) legacymobile.LegacyBody {
	it.t.Helper()
	f, code, body := it.legacyDo(name, vars)
	b, err := f.Check(code, body)
	if err != nil {
		it.t.Fatal(err)
	}
	return b
}

// legacyFail checks an old error envelope with the given status.
func (it *itest) legacyFail(name string, vars map[string]string, status int) legacymobile.LegacyBody {
	it.t.Helper()
	f, code, body := it.legacyDo(name, vars)
	var b legacymobile.LegacyBody
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &b); err != nil || json.Unmarshal(body, &raw) != nil {
		it.t.Fatalf("%s: %s", name, body)
	}
	if _, isNew := raw["error"]; code != status || b.Success == nil || *b.Success || isNew {
		it.t.Fatalf("%s %s = %d %s, want %d in the old error envelope", f.Method, f.Path, code, body, status)
	}
	return b
}

// TEC-234 / TEC-284 acceptance (F2-05b, F2-05b2): with MOBILE_LEGACY_ALIASES
// on, the contract fixtures (the old hub's paths and bodies) run against the
// real server: login with only email + password returns a token in the
// only organization, me is the hub's UserResource, the service list is the
// Laravel paginator of the organization's services and another
// organization's service is 404, a NexPTG report upload creates a
// measurement_results row, the push token lands in device_push_tokens.
// With the flag off every alias is 404.
func TestIntegrationLegacyMobileAliases(t *testing.T) {
	it := newIntegrationWith(t, func(c *config.Config) { c.Mobile.LegacyAliases = true })
	ctx := context.Background()
	center := it.brandCenter("olex")
	orgA := it.org("t234a", "dealer", center)
	orgB := it.org("t234b", "dealer", center)
	owner, pw := it.user("t234-owner")
	it.member(orgA, owner, "owner")
	expo := "ExponentPushToken[t234-" + it.suffix + "]"
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = it.pool.Exec(bg, "DELETE FROM measurement_results WHERE organization_id IN ($1, $2)", orgA.ID, orgB.ID)
		_, _ = it.pool.Exec(bg, "DELETE FROM device_push_tokens WHERE expo_token = $1", expo)
		_, _ = it.pool.Exec(bg, "DELETE FROM refresh_tokens WHERE user_id = $1", owner.ID)
	})

	service := func(org db.Organization, name, plate string, seq int) db.Service {
		t.Helper()
		cust, veh := it.svcCustomer(org, name, plate)
		svc, err := it.q.CreateService(ctx, db.CreateServiceParams{
			ServiceNo:      fmt.Sprintf("T234-%s-%d", it.suffix[len(it.suffix)-10:], seq),
			OrganizationID: org.ID, BrandID: org.BrandID, CustomerUserID: cust.ID, VehicleID: veh.ID,
			CarBrandID: veh.CarBrandID.Int64, CarModelID: veh.CarModelID.Int64, Status: "draft",
		})
		if err != nil {
			t.Fatalf("service: %v", err)
		}
		return svc
	}
	own := service(orgA, "t234-cust-a", "34T234A", 1)
	foreign := service(orgB, "t234-cust-b", "34T234B", 2)

	// 1. Login with the old body (email, password, device_name): a token in
	// the only organization, no X-Mobile-Api-Version header. Wrong
	// credentials are the old 422 errors.email.
	vars := map[string]string{"email": owner.Email.String, "password": "wrong-" + pw}
	if b := it.legacyFail("login", vars, http.StatusUnprocessableEntity); len(b.Errors["email"]) != 1 {
		t.Fatalf("wrong password errors = %v", b.Errors)
	}
	vars["password"] = pw
	env := it.legacyExpect("login", vars)
	var login struct {
		Token     string `json:"token"`
		TokenType string `json:"token_type"`
		User      struct {
			ID       int64    `json:"id"`
			Email    string   `json:"email"`
			DealerID *int64   `json:"dealer_id"`
			Roles    []string `json:"roles"`
		} `json:"user"`
	}
	if err := json.Unmarshal(env.Data, &login); err != nil || login.Token == "" || login.TokenType != "Bearer" {
		t.Fatalf("login token: %v %s", err, env.Data)
	}
	if login.User.ID != owner.ID || login.User.DealerID == nil || *login.User.DealerID != orgA.ID ||
		len(login.User.Roles) != 1 || login.User.Roles[0] != "dealer_owner" {
		t.Fatalf("login user = %+v", login.User)
	}
	vars = map[string]string{
		"access_token": login.Token, "service_id": strconv.FormatInt(own.ID, 10), "expo_push_token": expo,
	}

	// 2. me is the hub's UserResource of the signed-in user.
	env = it.legacyExpect("me", vars)
	var me struct {
		ID     int64  `json:"id"`
		Email  string `json:"email"`
		Dealer struct {
			ID   *int64 `json:"id"`
			Name string `json:"name"`
		} `json:"dealer"`
	}
	if err := json.Unmarshal(env.Data, &me); err != nil {
		t.Fatal(err)
	}
	if me.ID != owner.ID || me.Email != owner.Email.String || me.Dealer.ID == nil || *me.Dealer.ID != orgA.ID || me.Dealer.Name != orgA.Name {
		t.Fatalf("me = %+v", me)
	}

	// 3. The service list is the organization's, in the Laravel paginator;
	// the detail takes the integer id the list returned; another
	// organization's service is 404.
	env = it.legacyExpect("services_list", vars)
	var page struct {
		Data []struct {
			ID        int64  `json:"id"`
			ServiceNo string `json:"service_no"`
			DealerID  int64  `json:"dealer_id"`
		} `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	var hasOwn, hasForeign bool
	for _, s := range page.Data {
		hasOwn = hasOwn || (s.ID == own.ID && s.ServiceNo == own.ServiceNo && s.DealerID == orgA.ID)
		hasForeign = hasForeign || s.ID == foreign.ID
	}
	if !hasOwn || hasForeign || page.Meta.Total < 1 {
		t.Fatalf("service list = %s", env.Data)
	}
	env = it.legacyExpect("service_detail", vars)
	var detail struct {
		ID         int64  `json:"id"`
		ServiceNo  string `json:"service_no"`
		CustomerID int64  `json:"customer_id"`
	}
	if err := json.Unmarshal(env.Data, &detail); err != nil || detail.ID != own.ID || detail.CustomerID != own.CustomerUserID {
		t.Fatalf("service detail = %v %s", err, env.Data)
	}
	foreignVars := map[string]string{"access_token": login.Token, "service_id": strconv.FormatInt(foreign.ID, 10)}
	it.legacyFail("service_detail", foreignVars, http.StatusNotFound)
	foreignVars["service_id"] = foreign.Uuid.String()
	it.legacyFail("service_detail", foreignVars, http.StatusNotFound)

	// 4. A NexPTG report upload creates a measurement_results row with the
	// old body kept as raw.
	env = it.legacyExpect("measurement", vars)
	var report struct {
		ID     int64  `json:"id"`
		UUID   string `json:"uuid"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(env.Data, &report); err != nil || report.Status != "accepted" || report.ID == 0 {
		t.Fatalf("report = %v %s", err, env.Data)
	}
	var n int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM measurement_results
		WHERE id = $1 AND uuid = $2 AND organization_id = $3 AND created_by = $4 AND service_id IS NULL
		  AND vin = 'WBA1234567890ABCD' AND device_serial = 'NEXPTG-001' AND raw->'raw'->>'body_type' = 'sedan'`,
		report.ID, report.UUID, orgA.ID, owner.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("measurement rows = %d %v", n, err)
	}

	// 5. The push token is written to device_push_tokens, then revoked.
	it.legacyExpect("push_token", vars)
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM device_push_tokens
		WHERE expo_token = $1 AND user_id = $2 AND revoked_at IS NULL`, expo, owner.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("push rows = %d %v", n, err)
	}
	it.legacyExpect("push_token_delete", vars)
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM device_push_tokens
		WHERE expo_token = $1 AND revoked_at IS NOT NULL`, expo).Scan(&n); err != nil || n != 1 {
		t.Fatalf("revoked push rows = %d %v", n, err)
	}

	// 6. Flag off (the default): every alias is 404.
	off := newIntegration(t)
	vars["email"], vars["password"] = owner.Email.String, pw
	for _, r := range legacymobile.Routes {
		if _, code, body := off.legacyDo(r.Name, vars); code != http.StatusNotFound {
			t.Fatalf("flag off %s = %d %s, want 404", r.Name, code, body)
		}
	}
}
