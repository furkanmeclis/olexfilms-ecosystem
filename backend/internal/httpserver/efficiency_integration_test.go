package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
)

// TEC-488: efficiency reads are feature-gated, and dealers cannot request
// staff-level breakdowns.
func TestIntegrationEfficiencyFeatureGateAndDealerStaffScope(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dealer := it.org("t488-dealer", "dealer", center)
	owner, pw := it.user("t488-owner")
	it.member(dealer, owner, "owner")
	tok := it.loginOrg(owner, pw, dealer)
	path := "/v1/efficiency/summary?dimension=dealer&period_from=2026-01-01&period_to=2026-02-01"

	if code, env := it.do("GET", path, hostOlex, tok, nil); code != http.StatusForbidden || errCode(env) != "FEATURE_DISABLED" {
		t.Fatalf("module off summary = %d %s, want 403 FEATURE_DISABLED", code, errCode(env))
	}
	if _, err := it.srv.features.SetByAdmin(ctx, owner.ID, dealer.ID, features.ModuleEfficiency, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = it.srv.features.ClearByAdmin(context.Background(), dealer.ID, features.ModuleEfficiency)
	})

	if code, env := it.do("GET", path, hostOlex, tok, nil); code != http.StatusOK {
		t.Fatalf("module on summary = %d %s, want 200", code, errCode(env))
	}
	if code, env := it.do("GET", "/v1/efficiency/summary?dimension=staff&period_from=2026-01-01&period_to=2026-02-01", hostOlex, tok, nil); code != http.StatusBadRequest || errCode(env) != "VALIDATION_ERROR" {
		t.Fatalf("dealer staff summary = %d %s, want 400 VALIDATION_ERROR", code, errCode(env))
	}
}
