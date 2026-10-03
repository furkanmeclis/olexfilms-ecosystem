package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

type portalPrefs struct {
	EmailEnabled    bool `json:"email_enabled"`
	InappEnabled    bool `json:"inapp_enabled"`
	RealtimeEnabled bool `json:"realtime_enabled"`
	PushEnabled     bool `json:"push_enabled"`
}

type portalCountPage struct {
	Items []map[string]any `json:"items"`
	Total int64            `json:"total"`
}

// TEC-245 acceptance:
//   - a customer without a contract gets 200 and an empty list on
//     GET /v1/portal/contracts;
//   - the customer reads and updates the notification preferences from the
//     portal (/v1/portal/notification-preferences; the panel path stays
//     closed to the portal realm);
//   - a fleet session (e-mail + password, read only) gets 200 on the vehicle
//     list and the transfer list but 403 PORTAL_READ_ONLY when it starts,
//     verifies or cancels a transfer.
func TestIntegrationPortalContractsPrefsFleet(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("t245-dist", "distributor", center)
	dealer := it.org("t245-dealer", "dealer", dist)
	tail := it.suffix[len(it.suffix)-4:]

	cust, _ := it.svcCustomer(dealer, "t245-cust", "34C245"+tail)
	custTok, _, err := it.tokens.IssueAccess(jwt.AccessInput{
		UserID: cust.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Contracts: none recorded yet, so an empty list with 200.
	cp := decodeData[portalCountPage](t, it.custDo("GET", "/v1/portal/contracts", custTok, nil, http.StatusOK))
	if cp.Total != 0 || cp.Items == nil || len(cp.Items) != 0 {
		t.Fatalf("contracts = %+v, want an empty list", cp)
	}

	// 2. Notification preferences from the portal: read, update, read back.
	prefsPath := "/v1/portal/notification-preferences"
	before := decodeData[portalPrefs](t, it.custDo("GET", prefsPath, custTok, nil, http.StatusOK))
	want := portalPrefs{EmailEnabled: !before.EmailEnabled, InappEnabled: true, RealtimeEnabled: true}
	saved := decodeData[portalPrefs](t, it.custDo("PUT", prefsPath, custTok, want, http.StatusOK))
	if saved.EmailEnabled != want.EmailEnabled || !saved.InappEnabled {
		t.Fatalf("saved = %+v, want %+v", saved, want)
	}
	if got := decodeData[portalPrefs](t, it.custDo("GET", prefsPath, custTok, nil, http.StatusOK)); got.EmailEnabled != want.EmailEnabled {
		t.Fatalf("read back = %+v", got)
	}
	if code, env := it.do("GET", "/v1/notification-preferences", hostOlex, custTok, nil); code != http.StatusForbidden || errCode(env) != "REALM_FORBIDDEN" {
		t.Fatalf("panel preferences path with a portal token = %d %s", code, errCode(env))
	}

	// 3. Fleet session: reads open, transfer writes 403 PORTAL_READ_ONLY.
	fleet, fpw := it.user("t245-fleet", rbac.RoleFleet)
	_, veh := it.svcCustomer(dealer, "t245-fleet-veh", "34F245"+tail)
	if _, err := it.pool.Exec(ctx, `UPDATE vehicles SET user_id = $2 WHERE id = $1`, veh.ID, fleet.ID); err != nil {
		t.Fatalf("fleet vehicle: %v", err)
	}
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), `DELETE FROM vehicles WHERE id = $1`, veh.ID)
	})
	fleetTok := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": fleet.Email.String, "password": fpw, "realm": "portal",
	})).AccessToken

	vp := decodeData[portalCountPage](t, it.custDo("GET", "/v1/portal/vehicles", fleetTok, nil, http.StatusOK))
	if vp.Total != 1 || len(vp.Items) != 1 || vp.Items[0]["uuid"] != veh.Uuid.String() {
		t.Fatalf("fleet vehicles = %+v", vp)
	}
	vPath := "/v1/portal/vehicles/" + veh.Uuid.String() + "/transfers"
	it.custDo("GET", vPath, fleetTok, nil, http.StatusOK)
	it.custDo("GET", "/v1/portal/contracts", fleetTok, nil, http.StatusOK)
	it.custDo("GET", prefsPath, fleetTok, nil, http.StatusOK)

	tr := "/v1/portal/vehicle-transfers/" + uuid.NewString()
	for _, c := range []struct {
		path string
		body any
	}{
		{vPath, map[string]any{"phone": itPhone()}},
		{tr + "/verify", map[string]any{"from_code": "123456"}},
		{tr + "/cancel", nil},
	} {
		if code, env := it.do("POST", c.path, hostOlex, fleetTok, c.body); code != http.StatusForbidden || errCode(env) != "PORTAL_READ_ONLY" {
			t.Fatalf("fleet POST %s = %d %s, want 403 PORTAL_READ_ONLY", c.path, code, errCode(env))
		}
	}
	var transfers int64
	if err := it.pool.QueryRow(ctx, `SELECT COUNT(*) FROM vehicle_transfers WHERE vehicle_id = $1`, veh.ID).Scan(&transfers); err != nil || transfers != 0 {
		t.Fatalf("fleet started a transfer: %d %v", transfers, err)
	}
}
