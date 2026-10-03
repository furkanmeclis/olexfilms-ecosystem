package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legacymobile"
)

var legacyFixtureDir = filepath.Join("..", "modules", "legacymobile", legacymobile.FixtureDir)

// legacyDo sends a contract fixture as the old app would: its headers and
// body, no X-Mobile-Api-Version header.
func (it *itest) legacyDo(name string, vars map[string]string) (legacymobile.Fixture, int, envelope) {
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
	req.Header.Set("User-Agent", "OlexLegacy/1.9")
	for k, v := range f.Request.Headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	it.handler.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return f, rec.Code, env
}

// legacyExpect checks the fixture's status and data keys.
func (it *itest) legacyExpect(name string, vars map[string]string) envelope {
	it.t.Helper()
	f, code, env := it.legacyDo(name, vars)
	if code != f.Response.Status {
		it.t.Fatalf("%s %s = %d %s, want %d", f.Method, f.Path, code, errCode(env), f.Response.Status)
	}
	if missing := f.MissingKeys(env.Data); len(missing) > 0 {
		it.t.Fatalf("%s: data misses %v: %s", name, missing, env.Data)
	}
	return env
}

// TEC-234 acceptance (F2-05b): with MOBILE_LEGACY_ALIASES on, the contract
// fixtures run against the real server: login returns a token, me is the
// signed-in user, the service list is organization scoped and another
// organization's service is 404, a measurement creates a measurement_results
// row, the push token lands in device_push_tokens. With the flag off every
// alias is 404.
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

	// 1. Login returns a token (no X-Mobile-Api-Version header).
	env := it.legacyExpect("login", map[string]string{
		"email": owner.Email.String, "password": pw, "organization_slug": orgA.Slug, "device_id": "t234-dev-" + it.suffix,
	})
	var login struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(env.Data, &login); err != nil || login.AccessToken == "" {
		t.Fatalf("login token: %v %s", err, env.Data)
	}
	vars := map[string]string{
		"access_token": login.AccessToken, "service_uuid": own.Uuid.String(),
		"idempotency_key": "t234-key-" + it.suffix, "expo_push_token": expo,
	}

	// 2. me is the signed-in user in the organization of the login.
	env = it.legacyExpect("me", vars)
	var me struct {
		User struct {
			UUID  string `json:"uuid"`
			Email string `json:"email"`
		} `json:"user"`
		ActiveOrganization string `json:"active_organization_uuid"`
	}
	if err := json.Unmarshal(env.Data, &me); err != nil {
		t.Fatal(err)
	}
	if me.User.UUID != owner.Uuid.String() || me.User.Email != owner.Email.String || me.ActiveOrganization != orgA.Uuid.String() {
		t.Fatalf("me = %+v", me)
	}

	// 3. The service list is organization scoped; another organization's
	// service is 404.
	env = it.legacyExpect("services_list", vars)
	var page servicePage
	if err := json.Unmarshal(env.Data, &page); err != nil {
		t.Fatal(err)
	}
	if !page.has(own.Uuid.String()) || page.has(foreign.Uuid.String()) {
		t.Fatalf("service list = %+v", page)
	}
	env = it.legacyExpect("service_detail", vars)
	var detail serviceView
	if err := json.Unmarshal(env.Data, &detail); err != nil || detail.UUID != own.Uuid.String() {
		t.Fatalf("service detail = %v %s", err, env.Data)
	}
	foreignVars := map[string]string{"access_token": login.AccessToken, "service_uuid": foreign.Uuid.String()}
	if _, code, env := it.legacyDo("service_detail", foreignVars); code != http.StatusNotFound {
		t.Fatalf("foreign service = %d %s, want 404", code, errCode(env))
	}

	// 4. A measurement creates a measurement_results row for the service.
	env = it.legacyExpect("measurement", vars)
	var accepted struct {
		UUID   string `json:"uuid"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(env.Data, &accepted); err != nil || accepted.Status != "accepted" {
		t.Fatalf("measurement = %v %s", err, env.Data)
	}
	var n int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM measurement_results
		WHERE uuid = $1 AND organization_id = $2 AND service_id = $3 AND created_by = $4`,
		accepted.UUID, orgA.ID, own.ID, owner.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("measurement rows = %d %v", n, err)
	}

	// 5. The push token is written to device_push_tokens.
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
	vars["email"], vars["password"], vars["organization_slug"], vars["device_id"] = owner.Email.String, pw, orgA.Slug, "t234-off"
	for _, r := range legacymobile.Routes {
		if _, code, env := off.legacyDo(r.Name, vars); code != http.StatusNotFound {
			t.Fatalf("flag off %s = %d %s, want 404", r.Name, code, errCode(env))
		}
	}
}
