package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// TEC-1033: the brand catalog screens opened from the center's tenant
// settings call the /v1/platform/... catalog routes in the center's
// organization context; center owner and staff hold the grants at brand
// scope, so every list answers 200 for them (and 403 for a dealer).
func TestIntegrationCenterCatalogSettings(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("ccs-dist", "distributor", center)
	dealer := it.org("ccs-dealer", "dealer", dist)
	ownerUser, ownerPW := it.user("ccs-owner")
	it.member(center, ownerUser, "owner", rbac.RoleCenterOwner)
	staffUser, staffPW := it.user("ccs-staff")
	it.member(center, staffUser, "staff", rbac.RoleCenterStaff)
	dealerUser, dealerPW := it.user("ccs-dealer")
	it.member(dealer, dealerUser, "owner")

	modules := []string{
		features.ModuleCertificates, features.ModulePhotoStandard, features.ModuleEfficiency,
		features.ModuleIntakeContracts, // services (review questions) is a core module
	}
	admin, _ := it.user("ccs-admin", rbac.RoleSuperAdmin)
	for _, org := range []int64{center.ID, dealer.ID} {
		for _, m := range modules {
			if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, org, m, true); err != nil {
				t.Fatalf("enable %s: %v", m, err)
			}
		}
	}
	t.Cleanup(func() {
		for _, org := range []int64{center.ID, dealer.ID} {
			for _, m := range modules {
				_, _ = it.srv.features.ClearByAdmin(context.Background(), org, m)
			}
		}
	})

	paths := []string{
		"/v1/platform/certificate-types",
		"/v1/platform/photo-standard/angles",
		"/v1/platform/part-consumption-expectations",
		"/v1/platform/review-questions",
		"/v1/platform/contract-templates",
	}
	ownerTok := it.loginOrg(ownerUser, ownerPW, center)
	staffTok := it.loginOrg(staffUser, staffPW, center)
	dealerTok := it.loginOrg(dealerUser, dealerPW, dealer)
	for _, path := range paths {
		for name, tok := range map[string]string{"center_owner": ownerTok, "center_staff": staffTok} {
			if code, env := it.do("GET", path, hostOlex, tok, nil); code != http.StatusOK {
				t.Errorf("%s GET %s = %d %s, want 200", name, path, code, errCode(env))
			}
		}
		if code, _ := it.do("GET", path, hostOlex, dealerTok, nil); code != http.StatusForbidden {
			t.Errorf("dealer GET %s = %d, want 403", path, code)
		}
	}
}
