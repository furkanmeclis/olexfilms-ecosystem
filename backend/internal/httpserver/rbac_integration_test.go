package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// mountPlaceholders adds test-only routes for business modules that do not
// exist yet (pricing, accounting, campaigns, services) behind the real
// Authenticate + RequireOrganization chain.
func (it *itest) mountPlaceholders() {
	it.t.Helper()
	authn := middleware.Authenticate(it.srv.tokens, it.srv.loader)
	org := middleware.RequireOrganization(it.srv.tokens, it.q)
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
	})
	perm := func(slug string) http.Handler {
		return middleware.Chain(okHandler, authn, org, middleware.RequirePermission(slug))
	}
	mux := it.srv.mux
	mux.Handle("GET /v1/tenant/_test/pricing/purchase", perm(rbac.PermPricingPurchaseRead))
	mux.Handle("GET /v1/tenant/_test/pricing/sale", perm(rbac.PermPricingSaleRead))
	mux.Handle("GET /v1/tenant/_test/pricing/recommended", perm(rbac.PermPricingRecommendedRead))
	mux.Handle("GET /v1/tenant/_test/accounting", perm(rbac.PermAccountingRead))
	mux.Handle("GET /v1/tenant/_test/campaigns", perm(rbac.PermCampaignsRead))
	mux.Handle("PUT /v1/tenant/_test/pricing/sale", middleware.Chain(okHandler, authn, org,
		middleware.RequirePermission(rbac.PermPricingSaleWrite), middleware.RequireStepUp(it.srv.stepUp)))
	mux.Handle("GET /v1/tenant/_test/services", middleware.Chain(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f, _ := scopefilter.From(r.Context())
			rows, err := it.q.ListOrganizationsInScope(r.Context(), db.ListOrganizationsInScopeParams{
				OrgIds: f.OrgIDsArg(), BrandID: f.BrandIDArg(), LimitCount: 500,
			})
			if err != nil {
				response.InternalErr(w, r, err, "list")
				return
			}
			ids := []string{}
			for _, row := range rows {
				ids = append(ids, row.Organization.Uuid.String())
			}
			response.JSON(w, r, http.StatusOK, map[string]any{"scope": f.Scope, "organizations": ids})
		}), authn, org, middleware.RequireScope(it.q, rbac.PermServicesRead)))
}

func (it *itest) loginOrg(u db.User, pw string, org db.Organization) string {
	it.t.Helper()
	tp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": u.Email.String, "password": pw, "organization_slug": org.Slug,
	}))
	return tp.AccessToken
}

type meGrants struct {
	Permissions       []string          `json:"permissions"`
	Grants            map[string]string `json:"grants"`
	OrganizationRoles []string          `json:"organization_roles"`
}

func (it *itest) me(access string) meGrants {
	it.t.Helper()
	code, env := it.do("GET", "/v1/auth/me", hostOlex, access, nil)
	if code != http.StatusOK {
		it.t.Fatalf("me: %d %s", code, errCode(env))
	}
	var m meGrants
	if err := json.Unmarshal(env.Data, &m); err != nil {
		it.t.Fatal(err)
	}
	return m
}

func (it *itest) status(method, path, access string) (int, string) {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, access, nil)
	return code, errCode(env)
}

// Acceptance 1: dealer staff sees neither prices nor accounting; the dealer
// owner does, and sensitive price writes need a step-up.
func TestIntegrationDealerStaffNoPricing(t *testing.T) {
	it := newIntegration(t)
	it.mountPlaceholders()
	center := it.brandCenter("olex")
	dealer := it.org("rbac-dealer", "dealer", center)
	staff, spw := it.user("rbac-staff")
	owner, opw := it.user("rbac-owner")
	it.member(dealer, staff, "staff")
	it.member(dealer, owner, "owner")

	staffTok := it.loginOrg(staff, spw, dealer)
	m := it.me(staffTok)
	for slug := range m.Grants {
		if strings.HasPrefix(slug, "pricing.") || strings.HasPrefix(slug, "accounting.") {
			t.Fatalf("dealer staff /me grants include %s", slug)
		}
	}
	for _, slug := range m.Permissions {
		if strings.HasPrefix(slug, "pricing.") || strings.HasPrefix(slug, "accounting.") {
			t.Fatalf("dealer staff /me permissions include %s", slug)
		}
	}
	if m.Grants[rbac.PermServicesRead] != "managed" || m.Grants[rbac.PermServicesWrite] != "own" {
		t.Fatalf("dealer staff service grants = %v", m.Grants)
	}
	if len(m.OrganizationRoles) != 1 || m.OrganizationRoles[0] != rbac.RoleDealerStaff {
		t.Fatalf("organization roles = %v", m.OrganizationRoles)
	}
	for _, path := range []string{"pricing/purchase", "pricing/sale", "pricing/recommended", "accounting"} {
		if code, _ := it.status("GET", "/v1/tenant/_test/"+path, staffTok); code != http.StatusForbidden {
			t.Fatalf("dealer staff GET %s = %d, want 403", path, code)
		}
	}
	if code, ec := it.status("PUT", "/v1/tenant/_test/pricing/sale", staffTok); code != http.StatusForbidden || ec == "STEP_UP_REQUIRED" {
		t.Fatalf("dealer staff price write = %d %s, want permission 403", code, ec)
	}
	if code, _ := it.status("GET", "/v1/tenant/settings", staffTok); code != http.StatusForbidden {
		t.Fatalf("dealer staff tenant settings = %d", code)
	}

	ownerTok := it.loginOrg(owner, opw, dealer)
	om := it.me(ownerTok)
	if om.Grants[rbac.PermPricingSaleWrite] != "managed" || om.Grants[rbac.PermAccountingRead] != "managed" {
		t.Fatalf("dealer owner grants = %v", om.Grants)
	}
	for _, path := range []string{"pricing/purchase", "pricing/sale", "accounting"} {
		if code, ec := it.status("GET", "/v1/tenant/_test/"+path, ownerTok); code != http.StatusOK {
			t.Fatalf("dealer owner GET %s = %d %s", path, code, ec)
		}
	}
	// Step-up: sensitive price write without a recent step-up is 403.
	if code, ec := it.status("PUT", "/v1/tenant/_test/pricing/sale", ownerTok); code != http.StatusForbidden || ec != "STEP_UP_REQUIRED" {
		t.Fatalf("price write without step-up = %d %s", code, ec)
	}
	it.stepUp(owner.Uuid)
	if code, ec := it.status("PUT", "/v1/tenant/_test/pricing/sale", ownerTok); code != http.StatusOK {
		t.Fatalf("price write after step-up = %d %s", code, ec)
	}
}

// Acceptance 2: a distributor owner reaches its own dealers (subtree) and
// nothing of another distributor, both in lists and single GETs.
func TestIntegrationDistributorSubtree(t *testing.T) {
	it := newIntegration(t)
	it.mountPlaceholders()
	center := it.brandCenter("olex")
	d1 := it.org("rbac-d1", "distributor", center)
	d2 := it.org("rbac-d2", "distributor", center)
	a1 := it.org("rbac-a1", "dealer", d1)
	a2 := it.org("rbac-a2", "dealer", d1)
	b1 := it.org("rbac-b1", "dealer", d2)
	o1, pw1 := it.user("rbac-d1-owner")
	o2, pw2 := it.user("rbac-d2-owner")
	da, pwa := it.user("rbac-a1-owner")
	it.member(d1, o1, "owner")
	it.member(d2, o2, "owner")
	it.member(a1, da, "owner")

	listed := func(access, path string) map[string]bool {
		t.Helper()
		code, env := it.do("GET", path, hostOlex, access, nil)
		if code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, code, errCode(env))
		}
		var payload struct {
			Scope string `json:"scope"`
			Items []struct {
				UUID string `json:"uuid"`
			} `json:"items"`
			Organizations []string `json:"organizations"`
		}
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{"scope:" + payload.Scope: true}
		for _, item := range payload.Items {
			out[item.UUID] = true
		}
		for _, id := range payload.Organizations {
			out[id] = true
		}
		return out
	}
	expect := func(got map[string]bool, scope string, in []db.Organization, out []db.Organization) {
		t.Helper()
		if !got["scope:"+scope] {
			t.Fatalf("scope = %v, want %s", got, scope)
		}
		for _, o := range in {
			if !got[o.Uuid.String()] {
				t.Fatalf("%s must be visible: %v", o.Name, got)
			}
		}
		for _, o := range out {
			if got[o.Uuid.String()] {
				t.Fatalf("%s must not be visible: %v", o.Name, got)
			}
		}
	}

	t1 := it.loginOrg(o1, pw1, d1)
	if g := it.me(t1).Grants; g[rbac.PermServicesRead] != "subtree" || g[rbac.PermOrganizationsRead] != "subtree" {
		t.Fatalf("distributor owner grants = %v", g)
	}
	expect(listed(t1, "/v1/tenant/organizations"), "subtree", []db.Organization{d1, a1, a2}, []db.Organization{d2, b1, center})
	expect(listed(t1, "/v1/tenant/_test/services"), "subtree", []db.Organization{d1, a1, a2}, []db.Organization{d2, b1})
	for _, o := range []db.Organization{d1, a1, a2} {
		if code, ec := it.status("GET", "/v1/tenant/organizations/"+o.Uuid.String(), t1); code != http.StatusOK {
			t.Fatalf("d1 owner GET %s = %d %s", o.Name, code, ec)
		}
	}
	for _, o := range []db.Organization{d2, b1} {
		if code, _ := it.status("GET", "/v1/tenant/organizations/"+o.Uuid.String(), t1); code != http.StatusNotFound {
			t.Fatalf("d1 owner GET %s = %d, want 404", o.Name, code)
		}
	}

	t2 := it.loginOrg(o2, pw2, d2)
	expect(listed(t2, "/v1/tenant/organizations"), "subtree", []db.Organization{d2, b1}, []db.Organization{d1, a1, a2})
	if code, _ := it.status("GET", "/v1/tenant/organizations/"+a1.Uuid.String(), t2); code != http.StatusNotFound {
		t.Fatalf("d2 owner GET a1 = %d, want 404", code)
	}

	// A dealer owner only reaches its own organization (managed).
	ta := it.loginOrg(da, pwa, a1)
	expect(listed(ta, "/v1/tenant/organizations"), "managed", []db.Organization{a1}, []db.Organization{a2, d1, b1})
	if code, _ := it.status("GET", "/v1/tenant/organizations/"+d1.Uuid.String(), ta); code != http.StatusNotFound {
		t.Fatalf("dealer owner GET its distributor = %d, want 404", code)
	}
}

// Acceptance 3: center_social reaches campaigns, center_staff does not.
func TestIntegrationCenterSocialCampaigns(t *testing.T) {
	it := newIntegration(t)
	it.mountPlaceholders()
	center := it.brandCenter("olex")
	social, spw := it.user("rbac-social")
	staff, tpw := it.user("rbac-center-staff")
	it.member(center, social, "staff", rbac.RoleCenterSocial)
	it.member(center, staff, "staff", rbac.RoleCenterStaff)

	st := it.loginOrg(social, spw, center)
	if g := it.me(st).Grants; g[rbac.PermCampaignsRead] != "brand" || g[rbac.PermCampaignsWrite] != "brand" {
		t.Fatalf("center_social grants = %v", g)
	}
	if code, ec := it.status("GET", "/v1/tenant/_test/campaigns", st); code != http.StatusOK {
		t.Fatalf("center_social campaigns = %d %s", code, ec)
	}
	ct := it.loginOrg(staff, tpw, center)
	if _, ok := it.me(ct).Grants[rbac.PermCampaignsRead]; ok {
		t.Fatal("center_staff must not hold campaigns.read")
	}
	if code, _ := it.status("GET", "/v1/tenant/_test/campaigns", ct); code != http.StatusForbidden {
		t.Fatalf("center_staff campaigns = %d, want 403", code)
	}
}

// Impersonation stays with super_admin; organization roles are never granted
// globally.
func TestIntegrationImpersonationReserved(t *testing.T) {
	it := newIntegration(t)
	admin, apw := it.user("rbac-admin", rbac.RoleSuperAdmin)
	target, _ := it.user("rbac-target")
	atp := it.tokensFrom(it.do("POST", "/v1/auth/login", hostOlex, "", map[string]string{
		"email": admin.Email.String, "password": apw,
	}))
	slug := "rbac_custom_" + it.suffix
	t.Cleanup(func() {
		_, _ = it.pool.Exec(context.Background(), "DELETE FROM roles WHERE slug = $1", slug)
	})
	for _, body := range []map[string]any{
		{"name": "Imp", "slug": slug, "permission_slugs": []string{rbac.PermPlatformUsersImpersonate}},
		{"name": "Imp", "slug": slug, "grants": map[string]string{rbac.PermPlatformUsersImpersonate: "all"}},
	} {
		if code, ec := it.status2("POST", "/v1/platform/roles", atp.AccessToken, body); code != http.StatusBadRequest {
			t.Fatalf("create role with impersonate = %d %s, want 400", code, ec)
		}
	}
	code, env := it.do("POST", "/v1/platform/roles", hostOlex, atp.AccessToken, map[string]any{
		"name": "Custom", "slug": slug, "grants": map[string]string{rbac.PermServicesRead: "managed"},
	})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("create custom role = %d %s", code, errCode(env))
	}
	var role struct {
		UUID   string `json:"uuid"`
		Grants []struct {
			Permission string `json:"permission"`
			Scope      string `json:"scope"`
		} `json:"grants"`
	}
	_ = json.Unmarshal(env.Data, &role)
	if len(role.Grants) != 1 || role.Grants[0].Scope != "managed" {
		t.Fatalf("custom role grants = %+v", role.Grants)
	}
	if code, ec := it.status2("PATCH", "/v1/platform/roles/"+role.UUID, atp.AccessToken,
		map[string]any{"permission_slugs": []string{rbac.PermPlatformUsersImpersonate}}); code != http.StatusBadRequest {
		t.Fatalf("patch role with impersonate = %d %s, want 400", code, ec)
	}
	if code, ec := it.status2("PATCH", "/v1/platform/roles/"+role.UUID, atp.AccessToken,
		map[string]any{"grants": map[string]string{rbac.PermServicesWrite: "customer"}}); code != http.StatusBadRequest {
		t.Fatalf("patch role with disallowed scope = %d %s, want 400", code, ec)
	}

	dealerStaff, err := it.q.GetRoleBySlug(context.Background(), rbac.RoleDealerStaff)
	if err != nil {
		t.Fatal(err)
	}
	if code, ec := it.status2("PATCH", "/v1/platform/users/"+target.Uuid.String(), atp.AccessToken,
		map[string]any{"role_uuids": []string{dealerStaff.Uuid.String()}}); code != http.StatusBadRequest {
		t.Fatalf("global grant of an organization role = %d %s, want 400", code, ec)
	}
}

func (it *itest) status2(method, path, access string, body any) (int, string) {
	it.t.Helper()
	code, env := it.do(method, path, hostOlex, access, body)
	return code, errCode(env)
}
