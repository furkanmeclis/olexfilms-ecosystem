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
	// TEC-216 (000061): the distributor reads dealer stock (subtree) but
	// writes and adjusts only its own (managed); other distributor roles
	// and dealer_staff hold no stock grant.
	for _, slug := range []string{RoleDistributorOwner, RoleDistributorWarehouseStaff} {
		r, _ := RoleBySlug(slug)
		if r.Grants[PermStockRead] != ScopeSubtree {
			t.Fatalf("%s stock.read = %q, want subtree", slug, r.Grants[PermStockRead])
		}
		if r.Grants[PermStockWrite] != ScopeManaged || r.Grants[PermStockAdjust] != ScopeManaged {
			t.Fatalf("%s stock.write/adjust must stay managed: %v", slug, r.Grants)
		}
	}
	for _, slug := range []string{RoleDistributorStaff, RoleDistributorAccounting, RoleDealerStaff} {
		r, _ := RoleBySlug(slug)
		if _, ok := r.Grants[PermStockRead]; ok {
			t.Fatalf("%s must not hold stock.read", slug)
		}
	}
}

// TEC-483: stock forecast grants. Network demand is center-only; threshold
// management is owner/center only while staff can read forecasts.
func TestStockForecastGrants(t *testing.T) {
	for _, slug := range []string{RoleCenterStaff, RoleCenterWarehouse} {
		r, _ := RoleBySlug(slug)
		if r.Grants[PermStockForecastRead] != ScopeBrand ||
			r.Grants[PermStockForecastManage] != ScopeBrand ||
			r.Grants[PermStockForecastNetworkRead] != ScopeBrand {
			t.Fatalf("%s stock forecast grants = %v", slug, r.Grants)
		}
	}
	distOwner, _ := RoleBySlug(RoleDistributorOwner)
	if distOwner.Grants[PermStockForecastRead] != ScopeSubtree ||
		distOwner.Grants[PermStockForecastManage] != ScopeManaged {
		t.Fatalf("distributor_owner stock forecast grants = %v", distOwner.Grants)
	}
	dealerOwner, _ := RoleBySlug(RoleDealerOwner)
	if dealerOwner.Grants[PermStockForecastRead] != ScopeManaged ||
		dealerOwner.Grants[PermStockForecastManage] != ScopeManaged {
		t.Fatalf("dealer_owner stock forecast grants = %v", dealerOwner.Grants)
	}
	for _, slug := range []string{RoleDistributorStaff, RoleDistributorWarehouseStaff, RoleDealerStaff} {
		r, _ := RoleBySlug(slug)
		if _, ok := r.Grants[PermStockForecastManage]; ok {
			t.Fatalf("%s must not manage stock forecast thresholds", slug)
		}
		if _, ok := r.Grants[PermStockForecastNetworkRead]; ok {
			t.Fatalf("%s must not read network stock forecasts", slug)
		}
		if _, ok := r.Grants[PermStockForecastRead]; !ok {
			t.Fatalf("%s must read stock forecasts", slug)
		}
	}
	for _, r := range Roles {
		if r.Slug == RoleSuperAdmin || r.OrgType == OrgTypeCenter {
			continue
		}
		if _, ok := r.Grants[PermStockForecastNetworkRead]; ok {
			t.Fatalf("%s must not hold stock_forecast.network.read", r.Slug)
		}
	}
}

// TEC-171 (TEC-99 decision 7, K9/K24): dealer roles read their ledger and
// dispute entries posted by the parent. TEC-341 (F3-07) opens manual writes
// of the dealer's own book to dealer_owner and dealer_accounting (the use
// case still requires the dealer_accounting module); dealer_staff never
// writes accounting.
func TestAccountingGrants(t *testing.T) {
	for _, r := range Roles {
		if r.OrgType != OrgTypeDealer {
			continue
		}
		_, ok := r.Grants[PermAccountingWrite]
		switch r.Slug {
		case RoleDealerOwner, RoleDealerAccounting:
			if !ok || r.Grants[PermAccountingWrite] != ScopeManaged {
				t.Fatalf("%s accounting.write = %q, want managed", r.Slug, r.Grants[PermAccountingWrite])
			}
		default:
			if ok {
				t.Fatalf("%s must not hold accounting.write", r.Slug)
			}
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
// transfers (and center staff/warehouse returns, TEC-228), and no non-super_admin role reaches orders at scope all.
func TestOrderGrants(t *testing.T) {
	staff, _ := RoleBySlug(RoleCenterStaff)
	if staff.Grants[PermOrdersApprove] != ScopeBrand {
		t.Fatalf("center_staff orders.approve = %q", staff.Grants[PermOrdersApprove])
	}
	dist, _ := RoleBySlug(RoleDistributorOwner)
	for _, slug := range []string{PermOrdersApprove, PermOrdersShip, PermOrdersReceive, PermTransfersApprove, PermTransfersRequest} {
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
		// TEC-228: besides the distributor (sibling transfers), the center
		// staff and the center warehouse decide returns sent to the center;
		// center accounting and center social do not.
		centerApprover := r.Slug == RoleCenterStaff || r.Slug == RoleCenterWarehouse
		if _, ok := r.Grants[PermTransfersApprove]; ok && r.OrgType != OrgTypeDistributor && !centerApprover {
			t.Fatalf("%s must not hold transfers.approve", r.Slug)
		}
		if centerApprover && r.Grants[PermTransfersApprove] != ScopeBrand {
			t.Fatalf("%s transfers.approve = %q, want brand", r.Slug, r.Grants[PermTransfersApprove])
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

// TEC-214: tasks.* are center-only (brand scope); every center role holds
// both, no distributor or dealer role holds either.
func TestTaskGrants(t *testing.T) {
	for _, r := range Roles {
		if r.Slug == RoleSuperAdmin {
			continue
		}
		for _, slug := range []string{PermTasksRead, PermTasksWrite} {
			sc, ok := r.Grants[slug]
			if r.OrgType == OrgTypeCenter {
				if sc != ScopeBrand {
					t.Fatalf("%s %s = %q, want brand", r.Slug, slug, sc)
				}
			} else if ok {
				t.Fatalf("%s must not hold %s", r.Slug, slug)
			}
		}
	}
	g := RoleGrants(RoleDef{Slug: RoleSuperAdmin})
	if g[PermTasksRead] != ScopeAll || g[PermTasksWrite] != ScopeAll {
		t.Fatalf("super_admin task grants = %q %q", g[PermTasksRead], g[PermTasksWrite])
	}
}

// TEC-285: contracts.* are in the catalog; templates.manage and void are
// center-only (brand scope); every role that writes services reads and
// writes contracts at its services.read / services.write scope.
func TestContractGrants(t *testing.T) {
	for _, slug := range []string{PermContractsTemplatesManage, PermContractsRead, PermContractsWrite, PermContractsVoid} {
		def, ok := PermissionBySlug(slug)
		if !ok || def.Module != "contracts" {
			t.Fatalf("catalog misses %s", slug)
		}
	}
	for _, r := range Roles {
		if r.Slug == RoleSuperAdmin {
			continue
		}
		for _, slug := range []string{PermContractsTemplatesManage, PermContractsVoid} {
			if _, ok := r.Grants[slug]; ok && r.OrgType != OrgTypeCenter {
				t.Fatalf("%s must not hold %s", r.Slug, slug)
			}
		}
		if _, ok := r.Grants[PermServicesWrite]; ok {
			if r.Grants[PermContractsRead] != r.Grants[PermServicesRead] {
				t.Fatalf("%s contracts.read = %q, want services.read scope %q",
					r.Slug, r.Grants[PermContractsRead], r.Grants[PermServicesRead])
			}
			if r.Grants[PermContractsWrite] != r.Grants[PermServicesWrite] {
				t.Fatalf("%s contracts.write = %q, want services.write scope %q",
					r.Slug, r.Grants[PermContractsWrite], r.Grants[PermServicesWrite])
			}
		}
	}
	staff, _ := RoleBySlug(RoleCenterStaff)
	if staff.Grants[PermContractsTemplatesManage] != ScopeBrand || staff.Grants[PermContractsVoid] != ScopeBrand {
		t.Fatalf("center_staff contract grants = %q %q",
			staff.Grants[PermContractsTemplatesManage], staff.Grants[PermContractsVoid])
	}
	g := RoleGrants(RoleDef{Slug: RoleSuperAdmin})
	for _, slug := range []string{PermContractsTemplatesManage, PermContractsRead, PermContractsWrite, PermContractsVoid} {
		if g[slug] != ScopeAll {
			t.Fatalf("super_admin %s = %q", slug, g[slug])
		}
	}
}

// TEC-305: catalog management and cancellation approval are center-only;
// distributors assign to their subtree; dealers only read and request
// cancellation.
func TestServiceCatalogGrants(t *testing.T) {
	for _, r := range Roles {
		if r.Slug == RoleSuperAdmin || r.OrgType == OrgTypeCenter {
			continue
		}
		for _, slug := range []string{PermServiceCatalogManage, PermServiceSubscriptionsCancelApprove} {
			if _, ok := r.Grants[slug]; ok {
				t.Fatalf("%s must not hold %s", r.Slug, slug)
			}
		}
	}
	dist, _ := RoleBySlug(RoleDistributorOwner)
	if dist.Grants[PermServiceSubscriptionsAssign] != ScopeSubtree {
		t.Fatalf("distributor_owner assign = %q", dist.Grants[PermServiceSubscriptionsAssign])
	}
	dealer, _ := RoleBySlug(RoleDealerOwner)
	if _, ok := dealer.Grants[PermServiceSubscriptionsAssign]; ok {
		t.Fatal("dealer_owner must not assign service subscriptions")
	}
	if dealer.Grants[PermServiceSubscriptionsCancelRequest] != ScopeManaged {
		t.Fatalf("dealer_owner cancel_request = %q", dealer.Grants[PermServiceSubscriptionsCancelRequest])
	}
	staff, _ := RoleBySlug(RoleCenterStaff)
	if staff.Grants[PermServiceCatalogManage] != ScopeBrand || staff.Grants[PermServiceSubscriptionsCancelApprove] != ScopeBrand {
		t.Fatalf("center_staff service grants = %v", staff.Grants)
	}
}

// TEC-329: every network role reads announcements and the library in its
// organization; the center writes both for its brand, the distributor
// owner writes announcements for its subtree only; dealers and portal
// roles write nothing.
func TestAnnouncementLibraryGrants(t *testing.T) {
	for _, r := range Roles {
		g := RoleGrants(r)
		switch r.OrgType {
		case OrgTypeCenter, OrgTypeDistributor, OrgTypeDealer:
			if g[PermAnnouncementsRead] != ScopeManaged || g[PermLibraryRead] != ScopeManaged {
				t.Fatalf("%s announcements.read = %q, library.read = %q", r.Slug, g[PermAnnouncementsRead], g[PermLibraryRead])
			}
		case OrgTypeCustomer, OrgTypeFleet:
			if _, ok := g[PermAnnouncementsRead]; ok {
				t.Fatalf("%s must not read announcements", r.Slug)
			}
		}
		if r.OrgType == OrgTypeDealer || r.OrgType == OrgTypeCustomer || r.OrgType == OrgTypeFleet {
			if _, ok := g[PermAnnouncementsWrite]; ok {
				t.Fatalf("%s must not write announcements", r.Slug)
			}
		}
		if r.OrgType != OrgTypeCenter && r.Slug != RoleSuperAdmin {
			if _, ok := g[PermLibraryManage]; ok {
				t.Fatalf("%s must not manage the library", r.Slug)
			}
		}
	}
	staff, _ := RoleBySlug(RoleCenterStaff)
	if staff.Grants[PermAnnouncementsWrite] != ScopeBrand || staff.Grants[PermLibraryManage] != ScopeBrand {
		t.Fatalf("center_staff = %v", staff.Grants)
	}
	owner, _ := RoleBySlug(RoleDistributorOwner)
	if owner.Grants[PermAnnouncementsWrite] != ScopeSubtree {
		t.Fatalf("distributor_owner announcements.write = %q", owner.Grants[PermAnnouncementsWrite])
	}
}

// TEC-312: leads and quotes are managed-organization permissions; the social
// center role sees all leads, and organization conversion is limited to the
// center brand or a distributor subtree.
func TestLeadQuoteGrants(t *testing.T) {
	social, _ := RoleBySlug(RoleCenterSocial)
	if social.Grants[PermLeadsRead] != ScopeAll || social.Grants[PermLeadsWrite] != ScopeAll {
		t.Fatalf("center_social lead grants = %v", social.Grants)
	}
	if social.Grants[PermQuotesRead] != ScopeManaged || social.Grants[PermQuotesWrite] != ScopeManaged {
		t.Fatalf("center_social quote grants = %v", social.Grants)
	}
	staff, _ := RoleBySlug(RoleCenterStaff)
	if staff.Grants[PermLeadsRead] != ScopeManaged || staff.Grants[PermQuotesWrite] != ScopeManaged {
		t.Fatalf("center_staff lead/quote grants = %v", staff.Grants)
	}
	if staff.Grants[PermLeadsConvertOrg] != ScopeBrand {
		t.Fatalf("center_staff convert = %q", staff.Grants[PermLeadsConvertOrg])
	}
	dist, _ := RoleBySlug(RoleDistributorOwner)
	if dist.Grants[PermLeadsConvertOrg] != ScopeSubtree {
		t.Fatalf("distributor_owner convert = %q", dist.Grants[PermLeadsConvertOrg])
	}
	dealer, _ := RoleBySlug(RoleDealerOwner)
	if dealer.Grants[PermLeadsRead] != ScopeManaged || dealer.Grants[PermQuotesWrite] != ScopeManaged {
		t.Fatalf("dealer_owner lead/quote grants = %v", dealer.Grants)
	}
	if _, ok := dealer.Grants[PermLeadsConvertOrg]; ok {
		t.Fatal("dealer_owner must not convert leads to organizations")
	}
}

// TEC-341 (F3-07): the dealer accounting permissions and the dealer's own
// accounting writes belong to dealer_owner and dealer_accounting at the
// managed scope; dealer_staff holds none of them and no other role does.
func TestDealerAccountingGrants(t *testing.T) {
	perms := []string{
		PermDealerPricingWrite, PermProductSalesWrite, PermSuppliersManage,
		PermPurchasesWrite, PermStaffManage, PermStaffPaymentsWrite,
	}
	for _, slug := range []string{RoleDealerOwner, RoleDealerAccounting} {
		r, _ := RoleBySlug(slug)
		for _, p := range append([]string{PermAccountingWrite}, perms...) {
			if r.Grants[p] != ScopeManaged {
				t.Fatalf("%s %s = %q, want managed", slug, p, r.Grants[p])
			}
		}
	}
	staff, _ := RoleBySlug(RoleDealerStaff)
	for _, p := range append([]string{PermAccountingWrite, PermAccountingRead}, perms...) {
		if _, ok := staff.Grants[p]; ok {
			t.Fatalf("dealer_staff must not hold %s", p)
		}
	}
	for _, r := range Roles {
		if r.Slug == RoleSuperAdmin || r.Slug == RoleDealerOwner || r.Slug == RoleDealerAccounting {
			continue
		}
		for _, p := range perms {
			if _, ok := r.Grants[p]; ok {
				t.Fatalf("%s must not hold %s", r.Slug, p)
			}
		}
	}
	sa, _ := RoleBySlug(RoleSuperAdmin)
	for _, p := range perms {
		if RoleGrants(sa)[p] != ScopeAll {
			t.Fatalf("super_admin %s = %q, want all", p, RoleGrants(sa)[p])
		}
	}
}

// TEC-479: certificate definitions are center-owned; certificate upload
// belongs to dealer/distributor owners, and verification is center or
// distributor subtree only. Service-warning approval stays center-only.
func TestCertificateGrants(t *testing.T) {
	for _, slug := range []string{
		PermCertificateTypesManage, PermCertificatesRead, PermCertificatesWrite,
		PermCertificatesVerify, PermCertificatesApproveService,
	} {
		if _, ok := PermissionBySlug(slug); !ok {
			t.Fatalf("catalog misses %s", slug)
		}
	}
	center, _ := RoleBySlug(RoleCenterStaff)
	if center.Grants[PermCertificateTypesManage] != ScopeBrand ||
		center.Grants[PermCertificatesRead] != ScopeBrand ||
		center.Grants[PermCertificatesVerify] != ScopeBrand ||
		center.Grants[PermCertificatesApproveService] != ScopeBrand {
		t.Fatalf("center certificate grants = %v", center.Grants)
	}
	dist, _ := RoleBySlug(RoleDistributorOwner)
	if dist.Grants[PermCertificatesRead] != ScopeSubtree ||
		dist.Grants[PermCertificatesWrite] != ScopeSubtree ||
		dist.Grants[PermCertificatesVerify] != ScopeSubtree {
		t.Fatalf("distributor certificate grants = %v", dist.Grants)
	}
	dealer, _ := RoleBySlug(RoleDealerOwner)
	if dealer.Grants[PermCertificatesRead] != ScopeManaged || dealer.Grants[PermCertificatesWrite] != ScopeManaged {
		t.Fatalf("dealer certificate grants = %v", dealer.Grants)
	}
	for _, r := range Roles {
		if r.Slug == RoleSuperAdmin || r.Slug == RoleCenterStaff || r.Slug == RoleDistributorOwner || r.Slug == RoleDealerOwner {
			continue
		}
		for _, p := range []string{PermCertificateTypesManage, PermCertificatesWrite, PermCertificatesVerify, PermCertificatesApproveService} {
			if _, ok := r.Grants[p]; ok {
				t.Fatalf("%s must not hold %s", r.Slug, p)
			}
		}
	}
}

// TEC-383: every panel role uses the assistant and confirms its own actions;
// usage is read by owners and the center; settings stay super_admin only;
// portal roles get no AI permission (realm check instead).
func TestAIGrants(t *testing.T) {
	for _, slug := range []string{PermAIUse, PermAIUsageRead, PermAISettingsManage, PermAIActionsConfirm} {
		if _, ok := PermissionBySlug(slug); !ok {
			t.Fatalf("catalog misses %s", slug)
		}
	}
	if !SuperAdminOnly(PermAISettingsManage) {
		t.Fatal("ai.settings.manage must be super_admin only")
	}
	usage := map[string]Scope{
		RoleCenterStaff: ScopeBrand, RoleCenterAccounting: ScopeBrand,
		RoleDistributorOwner: ScopeManaged, RoleDealerOwner: ScopeManaged,
	}
	for _, r := range Roles {
		g := RoleGrants(r)
		switch r.OrgType {
		case OrgTypeCenter, OrgTypeDistributor, OrgTypeDealer:
			if g[PermAIUse] != ScopeOwn || g[PermAIActionsConfirm] != ScopeOwn {
				t.Fatalf("%s ai.use=%q ai.actions.confirm=%q, want own", r.Slug, g[PermAIUse], g[PermAIActionsConfirm])
			}
			if g[PermAIUsageRead] != usage[r.Slug] {
				t.Fatalf("%s ai.usage.read = %q, want %q", r.Slug, g[PermAIUsageRead], usage[r.Slug])
			}
		case OrgTypePlatform:
			if g[PermAISettingsManage] != ScopeAll || g[PermAIUsageRead] != ScopeAll {
				t.Fatalf("super_admin AI grants: %v", g)
			}
		default:
			for _, p := range []string{PermAIUse, PermAIUsageRead, PermAIActionsConfirm} {
				if _, ok := g[p]; ok {
					t.Fatalf("%s must not hold %s", r.Slug, p)
				}
			}
		}
		if r.Slug != RoleSuperAdmin {
			if _, ok := g[PermAISettingsManage]; ok {
				t.Fatalf("%s must not hold ai.settings.manage", r.Slug)
			}
		}
	}
}

// TEC-393: WhatsApp conversations are visible to the platform admin only
// (F4 user answer S2); no center, distributor, dealer or portal role holds
// any conversations permission.
func TestConversationGrants(t *testing.T) {
	slugs := []string{PermConversationsRead, PermConversationsReply, PermConversationsManage}
	for _, slug := range slugs {
		if _, ok := PermissionBySlug(slug); !ok {
			t.Fatalf("catalog misses %s", slug)
		}
		if !SuperAdminOnly(slug) {
			t.Fatalf("%s must be super_admin only", slug)
		}
	}
	for _, r := range Roles {
		g := RoleGrants(r)
		for _, slug := range slugs {
			scope, ok := g[slug]
			if r.Slug == RoleSuperAdmin {
				if scope != ScopeAll {
					t.Fatalf("super_admin %s = %q, want all", slug, scope)
				}
				continue
			}
			if ok {
				t.Fatalf("%s must not hold %s", r.Slug, slug)
			}
		}
	}
}

// TEC-404: campaigns. The center (super_admin, center_social) and the
// distributor owner approve; the dealer owner writes but never approves; no
// staff, accounting, warehouse or portal role holds a campaign permission.
func TestCampaignGrants(t *testing.T) {
	def, ok := PermissionBySlug(PermCampaignsApprove)
	if !ok {
		t.Fatalf("catalog misses %s", PermCampaignsApprove)
	}
	if def.Module != "campaigns" || def.SuperAdminOnly {
		t.Fatalf("campaigns.approve def = %+v", def)
	}
	slugs := []string{PermCampaignsRead, PermCampaignsWrite, PermCampaignsApprove}
	want := map[string]map[string]Scope{
		RoleSuperAdmin: {PermCampaignsRead: ScopeAll, PermCampaignsWrite: ScopeAll, PermCampaignsApprove: ScopeAll},
		RoleCenterSocial: {
			PermCampaignsRead: ScopeBrand, PermCampaignsWrite: ScopeBrand, PermCampaignsApprove: ScopeBrand,
		},
		RoleDistributorOwner: {
			PermCampaignsRead: ScopeSubtree, PermCampaignsWrite: ScopeManaged, PermCampaignsApprove: ScopeSubtree,
		},
		RoleDealerOwner: {PermCampaignsRead: ScopeManaged, PermCampaignsWrite: ScopeManaged},
	}
	for _, r := range Roles {
		g := RoleGrants(r)
		for _, slug := range slugs {
			if got, exp := g[slug], want[r.Slug][slug]; got != exp {
				t.Fatalf("%s %s = %q, want %q", r.Slug, slug, got, exp)
			}
		}
	}
}

// TEC-472: fleets. Dealer owner/staff, the distributor owner (subtree) and
// the center (center_staff, brand) read fleets; only owners and the center
// manage them; dealer owner and staff plan; the fleet role reads its own
// fleet in the portal. No other role holds a fleet permission.
func TestFleetGrants(t *testing.T) {
	slugs := []string{PermFleetsRead, PermFleetsManage, PermFleetsPlan, PermFleetPortalRead}
	for _, slug := range slugs {
		def, ok := PermissionBySlug(slug)
		if !ok {
			t.Fatalf("catalog misses %s", slug)
		}
		if def.Module != "fleet" || def.SuperAdminOnly {
			t.Fatalf("%s def = %+v", slug, def)
		}
	}
	want := map[string]map[string]Scope{
		RoleSuperAdmin: {
			PermFleetsRead: ScopeAll, PermFleetsManage: ScopeAll, PermFleetsPlan: ScopeAll, PermFleetPortalRead: ScopeOwn,
		},
		RoleCenterStaff:      {PermFleetsRead: ScopeBrand, PermFleetsManage: ScopeBrand},
		RoleDistributorOwner: {PermFleetsRead: ScopeSubtree, PermFleetsManage: ScopeSubtree},
		RoleDealerOwner:      {PermFleetsRead: ScopeManaged, PermFleetsManage: ScopeManaged, PermFleetsPlan: ScopeManaged},
		RoleDealerStaff:      {PermFleetsRead: ScopeManaged, PermFleetsPlan: ScopeManaged},
		RoleFleet:            {PermFleetPortalRead: ScopeOwn},
	}
	for _, r := range Roles {
		g := RoleGrants(r)
		for _, slug := range slugs {
			if got, exp := g[slug], want[r.Slug][slug]; got != exp {
				t.Fatalf("%s %s = %q, want %q", r.Slug, slug, got, exp)
			}
		}
	}
}

// TEC-466: dealer showcase. Dealer owner and distributor owner (subtree)
// write, dealer staff and the center read, the center reviews; no
// accounting, warehouse, distributor staff or portal role holds a showcase
// permission.
func TestShowcaseGrants(t *testing.T) {
	for _, slug := range []string{PermShowcaseRead, PermShowcaseWrite, PermPlatformShowcaseReview} {
		def, ok := PermissionBySlug(slug)
		if !ok {
			t.Fatalf("catalog misses %s", slug)
		}
		if def.Module != "dealer_showcase" || def.SuperAdminOnly {
			t.Fatalf("%s def = %+v", slug, def)
		}
	}
	slugs := []string{PermShowcaseRead, PermShowcaseWrite, PermPlatformShowcaseReview}
	want := map[string]map[string]Scope{
		RoleSuperAdmin:       {PermShowcaseRead: ScopeAll, PermShowcaseWrite: ScopeAll, PermPlatformShowcaseReview: ScopeAll},
		RoleCenterStaff:      {PermShowcaseRead: ScopeBrand, PermPlatformShowcaseReview: ScopeBrand},
		RoleCenterSocial:     {PermShowcaseRead: ScopeBrand, PermPlatformShowcaseReview: ScopeBrand},
		RoleDistributorOwner: {PermShowcaseRead: ScopeSubtree, PermShowcaseWrite: ScopeSubtree},
		RoleDealerOwner:      {PermShowcaseRead: ScopeManaged, PermShowcaseWrite: ScopeManaged},
		RoleDealerStaff:      {PermShowcaseRead: ScopeManaged},
	}
	for _, r := range Roles {
		g := RoleGrants(r)
		for _, slug := range slugs {
			if got, exp := g[slug], want[r.Slug][slug]; got != exp {
				t.Fatalf("%s %s = %q, want %q", r.Slug, slug, got, exp)
			}
		}
	}
}

// TEC-487: efficiency analytics. Dealer owner reads own organization,
// distributor owner reads the subtree, center staff manages expected part
// consumption. No staff/accounting/warehouse role holds the add-on grants.
func TestEfficiencyGrants(t *testing.T) {
	for _, slug := range []string{PermEfficiencyRead, PermEfficiencyExpectationsManage} {
		def, ok := PermissionBySlug(slug)
		if !ok {
			t.Fatalf("catalog misses %s", slug)
		}
		if def.Module != "efficiency" || def.SuperAdminOnly {
			t.Fatalf("%s def = %+v", slug, def)
		}
	}
	want := map[string]map[string]Scope{
		RoleSuperAdmin:       {PermEfficiencyRead: ScopeAll, PermEfficiencyExpectationsManage: ScopeAll},
		RoleCenterStaff:      {PermEfficiencyRead: ScopeBrand, PermEfficiencyExpectationsManage: ScopeBrand},
		RoleDistributorOwner: {PermEfficiencyRead: ScopeSubtree},
		RoleDealerOwner:      {PermEfficiencyRead: ScopeManaged},
	}
	for _, r := range Roles {
		g := RoleGrants(r)
		for _, slug := range []string{PermEfficiencyRead, PermEfficiencyExpectationsManage} {
			if got, exp := g[slug], want[r.Slug][slug]; got != exp {
				t.Fatalf("%s %s = %q, want %q", r.Slug, slug, got, exp)
			}
		}
	}
}

// TEC-490: performance and targets. Dealer owner reads own metrics and
// manages staff targets and bonuses (accounting shares bonuses);
// distributors read the subtree and the distributor owner sets targets and
// weak dealer rules below; center staff covers the brand.
func TestPerformanceGrants(t *testing.T) {
	slugs := []string{
		PermPerformanceRead, PermPerformanceTargetsManage, PermPerformanceStaffTargetsManage,
		PermPerformanceBonusManage, PermPerformanceRulesManage,
	}
	for _, slug := range slugs {
		def, ok := PermissionBySlug(slug)
		if !ok {
			t.Fatalf("catalog misses %s", slug)
		}
		if def.Module != "performance" || def.SuperAdminOnly {
			t.Fatalf("%s def = %+v", slug, def)
		}
	}
	want := map[string]map[string]Scope{
		RoleSuperAdmin: {
			PermPerformanceRead: ScopeAll, PermPerformanceTargetsManage: ScopeAll,
			PermPerformanceStaffTargetsManage: ScopeAll, PermPerformanceBonusManage: ScopeAll,
			PermPerformanceRulesManage: ScopeAll,
		},
		RoleCenterStaff: {
			PermPerformanceRead: ScopeBrand, PermPerformanceTargetsManage: ScopeBrand,
			PermPerformanceRulesManage: ScopeBrand,
		},
		RoleDistributorOwner: {
			PermPerformanceRead: ScopeSubtree, PermPerformanceTargetsManage: ScopeSubtree,
			PermPerformanceRulesManage: ScopeSubtree,
		},
		RoleDistributorStaff: {PermPerformanceRead: ScopeSubtree},
		RoleDealerOwner: {
			PermPerformanceRead: ScopeManaged, PermPerformanceStaffTargetsManage: ScopeManaged,
			PermPerformanceBonusManage: ScopeManaged,
		},
		RoleDealerAccounting: {PermPerformanceBonusManage: ScopeManaged},
	}
	for _, r := range Roles {
		g := RoleGrants(r)
		for _, slug := range slugs {
			if got, exp := g[slug], want[r.Slug][slug]; got != exp {
				t.Fatalf("%s %s = %q, want %q", r.Slug, slug, got, exp)
			}
		}
	}
}

// TEC-501: e-Invoice settings are super_admin-only; the archive and void
// workflow is held by center accounting (and super_admin through the catalog).
// No dealer, distributor, social, warehouse or portal role receives it.
func TestEinvoiceGrants(t *testing.T) {
	slugs := []string{PermEinvoiceRead, PermEinvoiceManage, PermEinvoiceSettings}
	for _, slug := range slugs {
		def, ok := PermissionBySlug(slug)
		if !ok {
			t.Fatalf("catalog misses %s", slug)
		}
		if def.Module != "einvoice" {
			t.Fatalf("%s def = %+v", slug, def)
		}
		if slug == PermEinvoiceSettings && !def.SuperAdminOnly {
			t.Fatalf("%s must be super_admin-only: %+v", slug, def)
		}
		if slug == PermEinvoiceManage && !def.Sensitive {
			t.Fatalf("%s must be sensitive: %+v", slug, def)
		}
	}
	want := map[string]map[string]Scope{
		RoleSuperAdmin: {
			PermEinvoiceRead: ScopeAll, PermEinvoiceManage: ScopeAll, PermEinvoiceSettings: ScopeAll,
		},
		RoleCenterAccounting: {PermEinvoiceRead: ScopeBrand, PermEinvoiceManage: ScopeBrand},
	}
	for _, r := range Roles {
		g := RoleGrants(r)
		for _, slug := range slugs {
			if got, exp := g[slug], want[r.Slug][slug]; got != exp {
				t.Fatalf("%s %s = %q, want %q", r.Slug, slug, got, exp)
			}
		}
	}
}
