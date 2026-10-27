package httpserver

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

type portalFeaturesPayload struct {
	Enabled []string `json:"enabled"`
}

func (it *itest) portalFeatures(tok string) []string {
	it.t.Helper()
	return decodeData[portalFeaturesPayload](it.t, it.accDo("GET", "/v1/portal/features", tok, nil, http.StatusOK)).Enabled
}

// closeSystemModule closes a module system wide through the service (cache
// generation bumped) and reopens it the same way on cleanup.
func (it *itest) closeSystemModule(key string) {
	it.t.Helper()
	off, on := false, true
	if _, err := it.srv.features.UpdatePlatformModule(context.Background(), 0, key, features.PlatformModuleInput{Enabled: &off}); err != nil {
		it.t.Fatalf("close %s: %v", key, err)
	}
	it.t.Cleanup(func() {
		_, _ = it.srv.features.UpdatePlatformModule(context.Background(), 0, key, features.PlatformModuleInput{Enabled: &on})
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM module_flags WHERE scope = 'system' AND module_key = $1", key)
	})
}

func (it *itest) setOrgModule(org db.Organization, key string, enabled bool) {
	it.t.Helper()
	if _, err := it.srv.features.SetByAdmin(context.Background(), 0, org.ID, key, enabled); err != nil {
		it.t.Fatalf("set %s=%v: %v", key, enabled, err)
	}
	it.t.Cleanup(func() { _, _ = it.srv.features.ClearByAdmin(context.Background(), org.ID, key) })
}

func expectFeatureDisabled(t *testing.T, it *itest, method, path, tok string, body any) {
	t.Helper()
	if code, env := it.do(method, path, hostOlex, tok, body); code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("%s %s = %d %s, want 403 FEATURE_DISABLED", method, path, code, errCode(env))
	}
}

// TEC-1023 acceptance (HTTP): GET /v1/portal/features lists the
// customer-facing modules of the portal session; a module the customer's
// dealer turned off drops out (reviews) and its portal writes answer 403
// FEATURE_DISABLED (review POST, appointment availability / create); a
// module closed system wide is never listed and its portal routes answer
// 403 FEATURE_DISABLED (appointments, warranty_claims, reviews) while the
// history reads of an org-level closure stay open.
func TestIntegrationPortalFeatures(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()

	center := it.brandCenter("olex")
	dist := it.org("t1023-dist", "distributor", center)
	dealer := it.org("t1023-dealer", "dealer", dist)
	plate := "34T1023" + it.suffix[len(it.suffix)-3:]
	cust, veh := it.svcCustomer(dealer, "t1023-cust", plate)
	if err := it.q.AssignUserRoleBySlug(ctx, db.AssignUserRoleBySlugParams{UserID: cust.ID, Slug: rbac.RoleCustomer}); err != nil {
		t.Fatalf("customer role: %v", err)
	}
	svc := it.directService(dealer, cust, veh, 1023)
	if _, err := it.q.UpsertAppointmentSettings(ctx, db.UpsertAppointmentSettingsParams{
		OrganizationID: dealer.ID, BrandID: dealer.BrandID, DailyVehicleCapacity: 3,
		DefaultEstimatedMinutes: 60, SlotIntervalMinutes: 60, WorkingHours: []byte(`{}`),
		PortalAppointmentsEnabled: true,
	}); err != nil {
		t.Fatalf("appointment settings: %v", err)
	}
	tok, _, err := it.tokens.IssueAccess(jwt.AccessInput{UserID: cust.Uuid, Roles: []string{rbac.RoleCustomer}, Audience: jwt.AudiencePortal})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Everything on: appointments, warranty_claims and reviews; a
	// customer session never gets fleet.
	got := it.portalFeatures(tok)
	for _, k := range []string{features.ModuleAppointments, features.ModuleWarrantyClaims, features.ModuleReviews} {
		if !hasKey(got, k) {
			t.Fatalf("enabled = %v, want %s", got, k)
		}
	}
	if hasKey(got, features.ModuleFleet) {
		t.Fatalf("customer session lists fleet: %v", got)
	}

	// 2. The only linked dealer turns reviews off: not listed, the review
	// form cannot be sent (403) and reports can_review false.
	it.setOrgModule(dealer, features.ModuleReviews, false)
	if got := it.portalFeatures(tok); hasKey(got, features.ModuleReviews) {
		t.Fatalf("reviews off at the dealer, enabled = %v", got)
	}
	reviewPath := "/v1/portal/services/" + svc.Uuid.String() + "/review"
	expectFeatureDisabled(t, it, "POST", reviewPath, tok, map[string]any{"platform_rating": 5, "product_rating": 5})
	if st := decodeData[portalServiceReviewState](t, it.accDo("GET", reviewPath, tok, nil, http.StatusOK)); st.CanReview {
		t.Fatal("can_review with reviews off")
	}

	// 3. The dealer turns appointments off: availability and booking 403;
	// the "my appointments" list (history) stays open.
	it.setOrgModule(dealer, features.ModuleAppointments, false)
	from := time.Now().UTC().AddDate(0, 0, 1).Format(time.DateOnly)
	to := time.Now().UTC().AddDate(0, 0, 3).Format(time.DateOnly)
	availability := "/v1/portal/dealers/" + dealer.Uuid.String() + "/availability?" + url.Values{"from": {from}, "to": {to}}.Encode()
	expectFeatureDisabled(t, it, "GET", availability, tok, nil)
	expectFeatureDisabled(t, it, "POST", "/v1/portal/appointments", tok, map[string]any{
		"dealer_uuid": dealer.Uuid.String(), "vehicle_uuid": veh.Uuid.String(),
		"starts_at": time.Now().UTC().AddDate(0, 0, 2).Truncate(time.Hour).Format(time.RFC3339),
	})
	it.accDo("GET", "/v1/portal/appointments", tok, nil, http.StatusOK)
	it.accDo("GET", "/v1/portal/warranty-claims", tok, nil, http.StatusOK)

	// 4. Closed system wide: never listed, every portal route 403.
	it.closeSystemModule(features.ModuleAppointments)
	it.closeSystemModule(features.ModuleWarrantyClaims)
	it.setOrgModule(dealer, features.ModuleReviews, true)
	it.closeSystemModule(features.ModuleReviews)
	if got := it.portalFeatures(tok); len(got) != 0 {
		t.Fatalf("closed system wide, enabled = %v", got)
	}
	expectFeatureDisabled(t, it, "GET", "/v1/portal/appointments", tok, nil)
	expectFeatureDisabled(t, it, "GET", "/v1/portal/warranty-claims", tok, nil)
	expectFeatureDisabled(t, it, "GET", reviewPath, tok, nil)
}

// TEC-1023: a fleet session lists fleet while an actively linked dealer has
// the module on (the rule that opens /v1/portal/fleet/*).
func TestIntegrationPortalFeaturesFleet(t *testing.T) {
	it := newIntegration(t)
	n := it.fleetNet("t1023f")
	it.enableFleet(n.dealerA)
	fleet := decodeData[fleetView](t, it.fleetDo("POST", "/v1/fleets", n.tokA, map[string]any{
		"legal_name": "T1023 Filo", "tax_number": tecVKN(t, it.suffix),
	}, http.StatusCreated)).UUID
	tok := it.fleetPortalUser(fleet, n.tokA, "t1023-fleet")
	if got := it.portalFeatures(tok); !hasKey(got, features.ModuleFleet) {
		t.Fatalf("fleet session, enabled = %v", got)
	}
	it.setOrgModule(n.dealerA, features.ModuleFleet, false)
	if got := it.portalFeatures(tok); hasKey(got, features.ModuleFleet) {
		t.Fatalf("fleet off at the only dealer, enabled = %v", got)
	}
	expectFeatureDisabled(t, it, "GET", "/v1/portal/fleet/overview", tok, nil)
}
