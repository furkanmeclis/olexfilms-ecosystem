package rbac

import (
	"strings"
	"testing"
)

func TestScopeCovers(t *testing.T) {
	cases := []struct {
		have, need Scope
		ok         bool
	}{
		{ScopeAll, ScopeBrand, true},
		{ScopeBrand, ScopeSubtree, true},
		{ScopeSubtree, ScopeManaged, true},
		{ScopeManaged, ScopeSubtree, false},
		{ScopeManaged, ScopeOwn, true},
		{ScopeOwn, ScopeManaged, false},
		{ScopeAll, ScopeCustomer, false},
		{ScopeCustomer, ScopeCustomer, true},
		{ScopeCustomer, ScopeOwn, false},
	}
	for _, c := range cases {
		if got := c.have.Covers(c.need); got != c.ok {
			t.Fatalf("%s.Covers(%s) = %v, want %v", c.have, c.need, got, c.ok)
		}
	}
	if Broader(ScopeManaged, ScopeSubtree) != ScopeSubtree || Broader(ScopeCustomer, ScopeOwn) != ScopeOwn {
		t.Fatal("Broader picks the wider internal scope")
	}
}

func TestParseGrantAndGrantScope(t *testing.T) {
	slug, sc := ParseGrant(" services.read:subtree ")
	if slug != "services.read" || sc != ScopeSubtree {
		t.Fatalf("ParseGrant = %q %q", slug, sc)
	}
	if _, sc := ParseGrant("services.read:nope"); sc != "" {
		t.Fatal("unknown scope must be dropped")
	}
	if got, ok := GrantScope([]string{"managed", "subtree"}, false, ""); !ok || got != ScopeSubtree {
		t.Fatalf("default grant scope = %q", got)
	}
	if _, ok := GrantScope([]string{"all"}, true, ScopeAll); ok {
		t.Fatal("super_admin-only permissions cannot be granted")
	}
}

// Every role grant names a catalog permission with a scope it allows.
func TestCatalogConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Permissions {
		if seen[p.Slug] {
			t.Fatalf("duplicate permission %s", p.Slug)
		}
		seen[p.Slug] = true
		if len(p.Scopes) == 0 || p.Module == "" || p.Name == "" {
			t.Fatalf("incomplete permission %s", p.Slug)
		}
	}
	for _, r := range Roles {
		for slug, scope := range RoleGrants(r) {
			def, ok := PermissionBySlug(slug)
			if !ok {
				t.Fatalf("role %s grants unknown %s", r.Slug, slug)
			}
			if !def.Allows(scope) {
				t.Fatalf("role %s grants %s with disallowed scope %s", r.Slug, slug, scope)
			}
			if def.SuperAdminOnly && r.Slug != RoleSuperAdmin {
				t.Fatalf("role %s must not hold %s", r.Slug, slug)
			}
			if scope == ScopeAll && r.Slug != RoleSuperAdmin {
				t.Fatalf("only super_admin reaches all; %s has %s:all", r.Slug, slug)
			}
		}
	}
}

// Acceptance: dealer staff sees no prices or accounting.
func TestDealerStaffHasNoPricingOrAccounting(t *testing.T) {
	r, _ := RoleBySlug(RoleDealerStaff)
	for slug := range r.Grants {
		if strings.HasPrefix(slug, "pricing.") || strings.HasPrefix(slug, "accounting.") {
			t.Fatalf("dealer_staff holds %s", slug)
		}
	}
}

// Acceptance: center_social has campaigns, center_staff does not; center
// roles default to the brand scope.
func TestCenterRoles(t *testing.T) {
	social, _ := RoleBySlug(RoleCenterSocial)
	staff, _ := RoleBySlug(RoleCenterStaff)
	if social.Grants[PermCampaignsRead] != ScopeBrand || social.Grants[PermCampaignsWrite] != ScopeBrand {
		t.Fatalf("center_social campaigns = %v", social.Grants)
	}
	if _, ok := staff.Grants[PermCampaignsRead]; ok {
		t.Fatal("center_staff must not hold campaigns.read")
	}
	if !SuperAdminOnly(PermPlatformUsersImpersonate) {
		t.Fatal("impersonation is super_admin-only")
	}
}

func TestDefaultMemberRole(t *testing.T) {
	cases := map[[2]string]string{
		{OrgTypeCenter, "owner"}:      RoleCenterStaff,
		{OrgTypeDistributor, "owner"}: RoleDistributorOwner,
		{OrgTypeDistributor, "staff"}: RoleDistributorStaff,
		{OrgTypeDealer, "owner"}:      RoleDealerOwner,
		{OrgTypeDealer, "staff"}:      RoleDealerStaff,
	}
	for in, want := range cases {
		if got := DefaultMemberRole(in[0], in[1]); got != want {
			t.Fatalf("DefaultMemberRole(%v) = %s, want %s", in, got, want)
		}
	}
}
