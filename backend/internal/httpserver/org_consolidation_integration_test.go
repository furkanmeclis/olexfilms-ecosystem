package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-1003: the center opens a service for a dealer directly under it (the
// legacy center's dealer picker), the action is in the activity log; the
// center's service point staff and a dealer cannot; the service point name
// shows on customer surfaces only while it is on.
func TestIntegrationCenterServiceOnBehalf(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()

	center := it.brandCenter("olex")
	dealer := it.org("t1003-dealer", "dealer", center)
	other := it.org("t1003-other", "dealer", center)
	staff, spw := it.user("t1003-staff")
	it.member(center, staff, "staff")
	spStaff, sppw := it.user("t1003-sp")
	it.member(center, spStaff, "staff", rbac.RoleCenterService)
	owner, opw := it.user("t1003-owner")
	it.member(dealer, owner, "owner")
	cust, veh := it.svcCustomer(dealer, "t1003-cust", "34T1003"+it.suffix[len(it.suffix)-3:])

	staffTok := it.loginOrg(staff, spw, center)
	spTok := it.loginOrg(spStaff, sppw, center)
	ownerTok := it.loginOrg(owner, opw, dealer)
	// Center settings belong to the platform admin (center staff has no
	// tenant.settings.write).
	admin, apw := it.user("t1003-admin", rbac.RoleSuperAdmin)
	it.member(center, admin, "staff")
	adminTok := it.loginOrg(admin, apw, center)

	body := func(org string) map[string]any {
		return map[string]any{"organization_uuid": org, "customer_uuid": cust.Uuid.String(), "vehicle_uuid": veh.Uuid.String()}
	}

	code, env := it.do("POST", "/v1/services", hostOlex, staffTok, body(dealer.Uuid.String()))
	if code != http.StatusCreated {
		t.Fatalf("center on behalf = %d %s", code, errCode(env))
	}
	var created struct {
		UUID         string `json:"uuid"`
		Organization struct {
			UUID string `json:"uuid"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(env.Data, &created); err != nil {
		t.Fatal(err)
	}
	var orgID int64
	if err := it.pool.QueryRow(ctx, `SELECT organization_id FROM services WHERE uuid = $1`, uuid.MustParse(created.UUID)).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	if orgID != dealer.ID {
		t.Fatalf("service organization = %d, want dealer %d", orgID, dealer.ID)
	}
	var logged int
	if err := it.pool.QueryRow(ctx, `SELECT count(*) FROM activity_events
		WHERE action = 'services.created_on_behalf' AND resource_uuid = $1 AND actor_user_id = $2`,
		uuid.MustParse(created.UUID), staff.ID).Scan(&logged); err != nil {
		t.Fatal(err)
	}
	if logged != 1 {
		t.Fatalf("activity rows = %d, want 1", logged)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM activity_events WHERE resource_uuid = $1", uuid.MustParse(created.UUID))
	})

	// The service point staff (managed scope) and a dealer cannot.
	if code, env := it.do("POST", "/v1/services", hostOlex, spTok, body(dealer.Uuid.String())); code < 400 {
		t.Fatalf("center_service on behalf = %d %s", code, errCode(env))
	}
	if code, env := it.do("POST", "/v1/services", hostOlex, ownerTok, body(other.Uuid.String())); code < 400 {
		t.Fatalf("dealer on behalf = %d %s", code, errCode(env))
	}

	// Service point: name for customers, the panel keeps the center name.
	var before struct {
		Enabled bool
		Name    *string
	}
	if err := it.pool.QueryRow(ctx, `SELECT service_point_enabled, service_point_name FROM organizations WHERE id = $1`, center.ID).
		Scan(&before.Enabled, &before.Name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `UPDATE organizations SET service_point_enabled = $2, service_point_name = $3 WHERE id = $1`,
			center.ID, before.Enabled, before.Name)
	})
	if code, env := it.do("PATCH", "/v1/tenant/settings", hostOlex, ownerTok,
		map[string]any{"service_point_enabled": true, "service_point_name": "x"}); code < 400 {
		t.Fatalf("dealer service point = %d %s", code, errCode(env))
	}
	code, env = it.do("PATCH", "/v1/tenant/settings", hostOlex, adminTok,
		map[string]any{"service_point_enabled": true, "service_point_name": "  AFM Test  "})
	if code != http.StatusOK {
		t.Fatalf("center service point = %d %s", code, errCode(env))
	}
	var settings struct {
		CompanyName  string `json:"company_name"`
		ServicePoint *struct {
			Enabled bool    `json:"enabled"`
			Name    *string `json:"name"`
		} `json:"service_point"`
	}
	if err := json.Unmarshal(env.Data, &settings); err != nil {
		t.Fatal(err)
	}
	if settings.ServicePoint == nil || !settings.ServicePoint.Enabled || settings.ServicePoint.Name == nil ||
		*settings.ServicePoint.Name != "AFM Test" || settings.CompanyName != center.Name {
		t.Fatalf("settings = %+v", settings)
	}
	var customerName string
	if err := it.pool.QueryRow(ctx, `SELECT org_customer_name(type, name, service_point_enabled, service_point_name)
		FROM organizations WHERE id = $1`, center.ID).Scan(&customerName); err != nil {
		t.Fatal(err)
	}
	if customerName != "AFM Test" {
		t.Fatalf("customer name = %q", customerName)
	}
	if code, env := it.do("PATCH", "/v1/tenant/settings", hostOlex, adminTok,
		map[string]any{"service_point_enabled": false}); code != http.StatusOK {
		t.Fatalf("disable = %d %s", code, errCode(env))
	}
	if err := it.pool.QueryRow(ctx, `SELECT org_customer_name(type, name, service_point_enabled, service_point_name)
		FROM organizations WHERE id = $1`, center.ID).Scan(&customerName); err != nil {
		t.Fatal(err)
	}
	if customerName != center.Name {
		t.Fatalf("customer name with the service point off = %q", customerName)
	}
}
