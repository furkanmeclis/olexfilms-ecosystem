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
			if scope == ScopeAll && r.Slug != RoleSuperAdmin && !BrandIndependentGrants[r.Slug][slug] {
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

// TEC-153: the center warehouse holds stock.* at scope all (K20); the
// dealer owner only reads its stock (K12); import is center-only (K14).
func TestStockGrants(t *testing.T) {
	wh, _ := RoleBySlug(RoleCenterWarehouse)
	for _, slug := range []string{PermStockRead, PermStockWrite, PermStockAdjust, PermStockReclassify, PermStockImport} {
		if wh.Grants[slug] != ScopeAll {
			t.Fatalf("center_warehouse %s = %q, want all", slug, wh.Grants[slug])
		}
	}
	dealer, _ := RoleBySlug(RoleDealerOwner)
	if dealer.Grants[PermStockRead] != ScopeManaged {
		t.Fatalf("dealer_owner stock.read = %q", dealer.Grants[PermStockRead])
	}
	for _, r := range Roles {
		if r.OrgType == OrgTypeCenter || r.Slug == RoleSuperAdmin {
			continue
		}
		for _, slug := range []string{PermStockImport, PermStockReclassify} {
			if _, ok := r.Grants[slug]; ok {
				t.Fatalf("%s must not hold %s", r.Slug, slug)
			}
		}
	}
	if _, ok := dealer.Grants[PermStockWrite]; ok {
		t.Fatal("dealer_owner must not hold stock.write")
	}
}

// TEC-171 (TEC-99 decision 7, K9/K24): dealer roles read their ledger and
// dispute entries posted by the parent; no dealer role writes accounting.
func TestAccountingGrants(t *testing.T) {
	for _, r := range Roles {
		if r.OrgType != OrgTypeDealer {
			continue
		}
		if _, ok := r.Grants[PermAccountingWrite]; ok {
			t.Fatalf("%s must not hold accounting.write", r.Slug)
		}
		// TEC-174: a dealer has no child to resolve disputes for.
		if _, ok := r.Grants[PermAccountingResolve]; ok {
			t.Fatalf("%s must not hold accounting.resolve", r.Slug)
		}
	}
	for slug, want := range map[string]Scope{
		RoleCenterAccounting: ScopeBrand, RoleDistributorOwner: ScopeManaged, RoleDistributorAccounting: ScopeManaged,
	} {
		r, _ := RoleBySlug(slug)
		if r.Grants[PermAccountingResolve] != want {
			t.Fatalf("%s accounting.resolve = %q, want %q", slug, r.Grants[PermAccountingResolve], want)
		}
	}
	for _, slug := range []string{RoleDealerOwner, RoleDealerAccounting, RoleDistributorOwner, RoleDistributorAccounting} {
		r, _ := RoleBySlug(slug)
		if r.Grants[PermAccountingRead] != ScopeManaged || r.Grants[PermAccountingDispute] != ScopeManaged {
			t.Fatalf("%s accounting grants = %v", slug, r.Grants)
		}
	}
	for _, slug := range []string{RoleCenterAccounting, RoleDistributorOwner, RoleDistributorAccounting} {
		r, _ := RoleBySlug(slug)
		if _, ok := r.Grants[PermAccountingWrite]; !ok {
			t.Fatalf("%s must keep accounting.write", slug)
		}
	}
}

// TEC-159 (K19): only center roles (and super_admin) anonymize or merge
// customers; every role that reads customers also reads their vehicles at
// the same scope.
func TestCustomerGrants(t *testing.T) {
	for _, r := range Roles {
		for _, perm := range []string{PermCustomersAnonymize, PermCustomersMerge} {
			if _, ok := r.Grants[perm]; ok && r.OrgType != OrgTypeCenter {
				t.Fatalf("%s (%s) must not hold %s", r.Slug, r.OrgType, perm)
			}
		}
		if sc, ok := r.Grants[PermCustomersRead]; ok && r.Grants[PermVehiclesRead] != sc {
			t.Fatalf("%s vehicles.read = %q, want %q", r.Slug, r.Grants[PermVehiclesRead], sc)
		}
		if sc, ok := r.Grants[PermCustomersWrite]; ok && r.Grants[PermVehiclesWrite] != sc {
			t.Fatalf("%s vehicles.write = %q, want %q", r.Slug, r.Grants[PermVehiclesWrite], sc)
		}
	}
	center, _ := RoleBySlug(RoleCenterStaff)
	if center.Grants[PermCustomersAnonymize] != ScopeBrand || center.Grants[PermCustomersMerge] != ScopeBrand {
		t.Fatalf("center_staff customer grants = %v", center.Grants)
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

// TEC-165 (K6/K13/K20): the seller approves and ships, the buyer receives;
// dealers never approve or ship orders, only distributors approve sibling
// transfers, and no non-super_admin role reaches orders at scope all.
func TestOrderGrants(t *testing.T) {
	staff, _ := RoleBySlug(RoleCenterStaff)
	if staff.Grants[PermOrdersApprove] != ScopeBrand {
		t.Fatalf("center_staff orders.approve = %q", staff.Grants[PermOrdersApprove])
	}
	dist, _ := RoleBySlug(RoleDistributorOwner)
	for _, slug := range []string{PermOrdersApprove, PermOrdersShip, PermOrdersReceive, PermTransfersApprove} {
		if dist.Grants[slug] != ScopeManaged {
			t.Fatalf("distributor_owner %s = %q", slug, dist.Grants[slug])
		}
	}
	dealer, _ := RoleBySlug(RoleDealerOwner)
	if dealer.Grants[PermOrdersReceive] != ScopeManaged || dealer.Grants[PermTransfersRequest] != ScopeManaged {
		t.Fatalf("dealer_owner order grants = %v", dealer.Grants)
	}
	for _, r := range Roles {
		if r.Slug == RoleSuperAdmin {
			continue
		}
		for slug, scope := range r.Grants {
			if (strings.HasPrefix(slug, "orders.") || strings.HasPrefix(slug, "transfers.")) && scope == ScopeAll {
				t.Fatalf("%s holds %s at scope all", r.Slug, slug)
			}
		}
		if r.OrgType == OrgTypeDealer {
			for _, slug := range []string{PermOrdersApprove, PermOrdersShip, PermTransfersApprove} {
				if _, ok := r.Grants[slug]; ok {
					t.Fatalf("%s must not hold %s", r.Slug, slug)
				}
			}
		}
		if _, ok := r.Grants[PermTransfersApprove]; ok && r.OrgType != OrgTypeDistributor {
			t.Fatalf("%s must not hold transfers.approve", r.Slug)
		}
	}
}

// TEC-178: every role that writes services may complete them; only center
// roles (and super_admin) cancel.
func TestServiceGrants(t *testing.T) {
	for _, r := range Roles {
		if r.Slug == RoleSuperAdmin {
			continue
		}
		if _, ok := r.Grants[PermServicesCancel]; ok && r.OrgType != OrgTypeCenter {
			t.Fatalf("%s must not hold services.cancel", r.Slug)
		}
		if _, ok := r.Grants[PermServicesWrite]; ok {
			if _, ok := r.Grants[PermServicesComplete]; !ok {
				t.Fatalf("%s writes services but cannot complete them", r.Slug)
			}
		}
	}
	staff, _ := RoleBySlug(RoleCenterStaff)
	if staff.Grants[PermServicesCancel] != ScopeBrand {
		t.Fatalf("center_staff services.cancel = %q", staff.Grants[PermServicesCancel])
	}
	if g := RoleGrants(RoleDef{Slug: RoleSuperAdmin}); g[PermServicesCancel] != ScopeAll {
		t.Fatalf("super_admin services.cancel = %q", g[PermServicesCancel])
	}
}

// TEC-185: only center roles (and super_admin) void warranties; every role
// that reads services reads warranties at the same scope, and every role
// that writes vehicles may transfer them at the same scope.
func TestWarrantyGrants(t *testing.T) {
	for _, r := range Roles {
		if r.Slug == RoleSuperAdmin {
			continue
		}
		if _, ok := r.Grants[PermWarrantiesVoid]; ok && r.OrgType != OrgTypeCenter {
			t.Fatalf("%s must not hold warranties.void", r.Slug)
		}
		if sc, ok := r.Grants[PermServicesRead]; ok && r.Grants[PermWarrantiesRead] != sc {
			t.Fatalf("%s warranties.read = %q, want services.read scope %q", r.Slug, r.Grants[PermWarrantiesRead], sc)
		}
		if sc, ok := r.Grants[PermVehiclesWrite]; ok && r.Grants[PermVehiclesTransfer] != sc {
			t.Fatalf("%s vehicles.transfer = %q, want vehicles.write scope %q", r.Slug, r.Grants[PermVehiclesTransfer], sc)
		}
	}
	g := RoleGrants(RoleDef{Slug: RoleSuperAdmin})
	if g[PermWarrantiesVoid] != ScopeAll || g[PermWarrantiesRead] != ScopeAll || g[PermVehiclesTransfer] != ScopeAll {
		t.Fatalf("super_admin warranty grants = %q %q %q", g[PermWarrantiesRead], g[PermWarrantiesVoid], g[PermVehiclesTransfer])
	}
}

// TEC-201 (K12): the warehouse module belongs to the center and the
// distributor; no dealer role holds warehouse.*.
func TestWarehouseGrants(t *testing.T) {
	for _, slug := range []string{RoleCenterWarehouse, RoleDistributorOwner, RoleDistributorWarehouseStaff} {
		r, _ := RoleBySlug(slug)
		if _, ok := r.Grants[PermWarehouseRead]; !ok {
			t.Fatalf("%s must hold warehouse.read", slug)
		}
		if _, ok := r.Grants[PermWarehouseWrite]; !ok {
			t.Fatalf("%s must hold warehouse.write", slug)
		}
	}
	for _, r := range Roles {
		if r.OrgType != OrgTypeDealer {
			continue
		}
		for _, slug := range []string{PermWarehouseRead, PermWarehouseWrite} {
			if _, ok := r.Grants[slug]; ok {
				t.Fatalf("%s must not hold %s (K12)", r.Slug, slug)
			}
		}
	}
}
