package rbac

import "sort"

// PermissionDef is one permission catalog entry. The catalog in this file is
// the single source of truth: migration 000029 seeds the same rows and
// `cmd/roles-sync` reconciles a database with it.
type PermissionDef struct {
	Slug        string
	Name        string
	Module      string
	Scopes      []Scope
	Description string
	// Sensitive grants are highlighted in the role editor; sensitive write
	// endpoints also require a recent step-up.
	Sensitive bool
	// SuperAdminOnly permissions cannot be granted to any other role.
	SuperAdminOnly bool
}

// RoleDef is a system role package (role = permission set with scopes).
type RoleDef struct {
	Slug        string
	Name        string
	Description string
	OrgType     string
	// Grants maps permission slug to scope. super_admin has no explicit
	// grants: it receives every permission at its broadest scope.
	Grants map[string]Scope
}

var (
	scopesAll        = []Scope{ScopeAll}
	scopesSelf       = []Scope{ScopeOwn}
	scopesOrg        = []Scope{ScopeManaged}
	scopesTree       = []Scope{ScopeManaged, ScopeSubtree, ScopeBrand, ScopeAll}
	scopesSupplier   = []Scope{ScopeBrand, ScopeAll}
	scopesRecords    = []Scope{ScopeOwn, ScopeAssigned, ScopeManaged, ScopeSubtree, ScopeBrand, ScopeAll, ScopeCustomer}
	scopesRecordsInt = []Scope{ScopeOwn, ScopeAssigned, ScopeManaged, ScopeSubtree, ScopeBrand, ScopeAll}
)

func platformPerm(slug, name string) PermissionDef {
	return PermissionDef{Slug: slug, Name: name, Module: "platform", Scopes: scopesAll}
}

// Permissions is the permission catalog in display order.
var Permissions = []PermissionDef{
	{Slug: PermAuthSession, Name: "Auth session", Module: "auth", Scopes: scopesSelf},
	{Slug: PermNotificationsRead, Name: "Read notifications", Module: "notifications", Scopes: scopesSelf},
	{Slug: PermNotificationsManage, Name: "Manage notifications", Module: "notifications", Scopes: scopesSelf},

	platformPerm(PermPlatformUsersRead, "Read platform users"),
	platformPerm(PermPlatformUsersWrite, "Write platform users"),
	platformPerm(PermPlatformUsersExport, "Export platform users"),
	platformPerm(PermPlatformUsersImport, "Import platform users"),
	platformPerm(PermPlatformUsersBulkDisable, "Bulk disable platform users"),
	platformPerm(PermPlatformUsersBulkEnable, "Bulk enable platform users"),
	{
		Slug: PermPlatformUsersImpersonate, Name: "Impersonate platform users", Module: "platform",
		Scopes: scopesAll, Sensitive: true, SuperAdminOnly: true,
		Description: "Only super_admin may hold this permission.",
	},
	platformPerm(PermPlatformRolesRead, "Read platform roles"),
	platformPerm(PermPlatformRolesWrite, "Write platform roles"),
	platformPerm(PermPlatformRolesExport, "Export platform roles"),
	platformPerm(PermPlatformRolesImport, "Import platform roles"),
	platformPerm(PermPlatformRolesBulkDelete, "Bulk delete platform roles"),
	platformPerm(PermPlatformBulkRead, "Read bulk jobs"),
	platformPerm(PermPlatformNotificationsRead, "Read platform notifications"),
	platformPerm(PermPlatformNotificationsReadAll, "Read all platform notifications"),
	platformPerm(PermPlatformNotificationsExport, "Export platform notifications"),
	platformPerm(PermPlatformSettingsRead, "Read platform settings"),
	platformPerm(PermPlatformSettingsWrite, "Write platform settings"),
	platformPerm(PermPlatformActivityRead, "Read activity log"),
	platformPerm(PermPlatformImportsRead, "Read import jobs"),
	platformPerm(PermPlatformExportsRead, "Read export jobs"),
	platformPerm(PermPlatformStorageRead, "Read platform storage"),
	platformPerm(PermPlatformStorageWrite, "Write platform storage"),
	platformPerm(PermPlatformLogsRead, "Read platform logs"),
	platformPerm(PermPlatformLogsWrite, "Write platform logs"),
	platformPerm(PermPlatformAccessRead, "Read access settings"),
	platformPerm(PermPlatformAccessWrite, "Write access settings"),
	platformPerm(PermPlatformIntegrationsGitHubRead, "Read GitHub integration settings"),
	platformPerm(PermPlatformIntegrationsGitHubWrite, "Write GitHub integration settings"),
	platformPerm(PermPlatformIntegrationsGoogleRead, "Read Google integration settings"),
	platformPerm(PermPlatformIntegrationsGoogleWrite, "Write Google integration settings"),
	platformPerm(PermPlatformIntegrationsFacebookRead, "Read Facebook integration settings"),
	platformPerm(PermPlatformIntegrationsFacebookWrite, "Write Facebook integration settings"),
	platformPerm(PermPlatformIntegrationsAppleRead, "Read Apple integration settings"),
	platformPerm(PermPlatformIntegrationsAppleWrite, "Write Apple integration settings"),
	platformPerm(PermPlatformAuthSettingsRead, "Read auth registration settings"),
	platformPerm(PermPlatformAuthSettingsWrite, "Write auth registration settings"),
	platformPerm(PermPlatformOrganizationsRead, "Read platform organizations"),
	platformPerm(PermPlatformOrganizationsWrite, "Write platform organizations"),

	{Slug: PermTenantSettingsRead, Name: "Read organization settings", Module: "tenant", Scopes: scopesOrg},
	{Slug: PermTenantSettingsWrite, Name: "Write organization settings", Module: "tenant", Scopes: scopesOrg},
	{Slug: PermTenantImportsRead, Name: "Read organization import jobs", Module: "tenant", Scopes: scopesOrg},
	{Slug: PermTenantExportsRead, Name: "Read organization export jobs", Module: "tenant", Scopes: scopesOrg},

	{Slug: PermOrganizationsRead, Name: "Read organizations", Module: "organizations", Scopes: scopesTree},
	{Slug: PermOrganizationsWrite, Name: "Write organizations", Module: "organizations", Scopes: scopesTree},
	{
		Slug: PermOrganizationsSupplierWrite, Name: "Change organization supplier", Module: "organizations",
		Scopes: scopesSupplier, Sensitive: true,
		Description: "Move a dealer under another distributor (K25). Requires step-up.",
	},
	{Slug: PermMembersRead, Name: "Read organization members", Module: "members", Scopes: scopesTree},
	{Slug: PermMembersWrite, Name: "Write organization members", Module: "members", Scopes: scopesTree},

	{Slug: PermServicesRead, Name: "Read services", Module: "services", Scopes: scopesRecords},
	{Slug: PermServicesWrite, Name: "Write services", Module: "services", Scopes: scopesRecordsInt},
	{Slug: PermCustomersRead, Name: "Read customers", Module: "customers", Scopes: scopesRecords},
	{Slug: PermCustomersWrite, Name: "Write customers", Module: "customers", Scopes: scopesRecordsInt},

	{
		Slug: PermPricingPurchaseRead, Name: "Read purchase prices", Module: "pricing",
		Scopes: scopesTree, Sensitive: true,
	},
	{Slug: PermPricingSaleRead, Name: "Read sale prices", Module: "pricing", Scopes: scopesTree},
	{
		Slug: PermPricingSaleWrite, Name: "Write sale prices", Module: "pricing",
		Scopes: scopesTree, Sensitive: true, Description: "Requires step-up.",
	},
	{
		Slug: PermPricingRecommendedRead, Name: "Read recommended prices", Module: "pricing",
		Scopes: scopesTree, Sensitive: true,
	},
	{
		Slug: PermPricingRecommendedWrite, Name: "Publish recommended prices", Module: "pricing",
		Scopes: scopesSupplier, Sensitive: true, Description: "Requires step-up.",
	},

	{Slug: PermAccountingRead, Name: "Read accounting", Module: "accounting", Scopes: scopesTree},
	{Slug: PermAccountingWrite, Name: "Write accounting", Module: "accounting", Scopes: scopesTree},

	{Slug: PermWarehouseRead, Name: "Read warehouse", Module: "warehouse", Scopes: scopesTree},
	{Slug: PermWarehouseWrite, Name: "Write warehouse", Module: "warehouse", Scopes: scopesTree},

	{Slug: PermCampaignsRead, Name: "Read campaigns", Module: "campaigns", Scopes: scopesTree},
	{Slug: PermCampaignsWrite, Name: "Write campaigns", Module: "campaigns", Scopes: scopesTree},
	{Slug: PermLeadsRead, Name: "Read leads", Module: "leads", Scopes: scopesTree},
	{Slug: PermLeadsWrite, Name: "Write leads", Module: "leads", Scopes: scopesTree},
	{Slug: PermSocialRead, Name: "Read social", Module: "social", Scopes: scopesTree},
	{Slug: PermSocialWrite, Name: "Write social", Module: "social", Scopes: scopesTree},

	{
		Slug: PermPrivacyAnonymize, Name: "Anonymize personal data", Module: "privacy",
		Scopes: []Scope{ScopeManaged, ScopeBrand, ScopeAll}, Sensitive: true,
		Description: "KVKK/GDPR anonymization (K19). Requires step-up.",
	},

	// Module packages (TEC-86). Appended so earlier sort orders stay put.
	{
		Slug: PermModulesRead, Name: "Read module settings", Module: "modules",
		Scopes:      []Scope{ScopeManaged, ScopeSubtree},
		Description: "Module flags of the organization (managed) or of its dealers (subtree); request a module.",
	},
	{
		Slug: PermModulesManage, Name: "Manage dealer modules", Module: "modules",
		Scopes:      []Scope{ScopeSubtree},
		Description: "Switch modules of the distributor's dealers and edit the dealer standard.",
	},
	platformPerm(PermPlatformModulesRead, "Read platform modules"),
	platformPerm(PermPlatformModulesWrite, "Write platform modules"),
}

func grants(base map[string]Scope, extra map[string]Scope) map[string]Scope {
	out := make(map[string]Scope, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

var baseGrants = map[string]Scope{
	PermAuthSession:         ScopeOwn,
	PermNotificationsRead:   ScopeOwn,
	PermNotificationsManage: ScopeOwn,
}

var ownerTenantGrants = map[string]Scope{
	PermTenantSettingsRead:  ScopeManaged,
	PermTenantSettingsWrite: ScopeManaged,
	PermTenantImportsRead:   ScopeManaged,
	PermTenantExportsRead:   ScopeManaged,
}

// Roles is the system role matrix. Center roles default to the brand scope;
// only super_admin reaches `all`.
var Roles = []RoleDef{
	{
		Slug: RoleSuperAdmin, Name: "Platform Admin", OrgType: OrgTypePlatform,
		Description: "Platform-level operator with all permissions",
	},
	{
		Slug: RoleCenterStaff, Name: "Center staff", OrgType: OrgTypeCenter,
		Description: "Center staff: services, customers and the organization tree of the brand",
		Grants: grants(baseGrants, map[string]Scope{
			PermOrganizationsRead:      ScopeBrand,
			PermMembersRead:            ScopeBrand,
			PermServicesRead:           ScopeBrand,
			PermServicesWrite:          ScopeBrand,
			PermCustomersRead:          ScopeBrand,
			PermCustomersWrite:         ScopeBrand,
			PermPricingRecommendedRead: ScopeBrand,
		}),
	},
	{
		Slug: RoleCenterWarehouse, Name: "Center warehouse", OrgType: OrgTypeCenter,
		Description: "Center warehouse operator",
		Grants: grants(baseGrants, map[string]Scope{
			PermOrganizationsRead: ScopeBrand,
			PermWarehouseRead:     ScopeBrand,
			PermWarehouseWrite:    ScopeBrand,
		}),
	},
	{
		Slug: RoleCenterAccounting, Name: "Center accounting", OrgType: OrgTypeCenter,
		Description: "Center accounting and price management",
		Grants: grants(baseGrants, map[string]Scope{
			PermOrganizationsRead:       ScopeBrand,
			PermAccountingRead:          ScopeBrand,
			PermAccountingWrite:         ScopeBrand,
			PermPricingPurchaseRead:     ScopeBrand,
			PermPricingSaleRead:         ScopeBrand,
			PermPricingSaleWrite:        ScopeBrand,
			PermPricingRecommendedRead:  ScopeBrand,
			PermPricingRecommendedWrite: ScopeBrand,
		}),
	},
	{
		Slug: RoleCenterSocial, Name: "Center social", OrgType: OrgTypeCenter,
		Description: "Leads, campaigns and social media for the brand",
		Grants: grants(baseGrants, map[string]Scope{
			PermOrganizationsRead: ScopeBrand,
			PermCustomersRead:     ScopeBrand,
			PermCampaignsRead:     ScopeBrand,
			PermCampaignsWrite:    ScopeBrand,
			PermLeadsRead:         ScopeBrand,
			PermLeadsWrite:        ScopeBrand,
			PermSocialRead:        ScopeBrand,
			PermSocialWrite:       ScopeBrand,
		}),
	},
	{
		Slug: RoleDistributorOwner, Name: "Distributor owner", OrgType: OrgTypeDistributor,
		Description: "Distributor owner: own prices, accounting and warehouse; services and dealers of the subtree",
		Grants: grants(grants(baseGrants, ownerTenantGrants), map[string]Scope{
			PermOrganizationsRead:      ScopeSubtree,
			PermOrganizationsWrite:     ScopeSubtree,
			PermMembersRead:            ScopeSubtree,
			PermMembersWrite:           ScopeSubtree,
			PermServicesRead:           ScopeSubtree,
			PermServicesWrite:          ScopeSubtree,
			PermCustomersRead:          ScopeSubtree,
			PermCustomersWrite:         ScopeSubtree,
			PermPricingPurchaseRead:    ScopeManaged,
			PermPricingSaleRead:        ScopeManaged,
			PermPricingSaleWrite:       ScopeManaged,
			PermPricingRecommendedRead: ScopeManaged,
			PermAccountingRead:         ScopeManaged,
			PermAccountingWrite:        ScopeManaged,
			PermWarehouseRead:          ScopeManaged,
			PermWarehouseWrite:         ScopeManaged,
			PermModulesRead:            ScopeSubtree,
			PermModulesManage:          ScopeSubtree,
		}),
	},
	{
		Slug: RoleDistributorStaff, Name: "Distributor staff", OrgType: OrgTypeDistributor,
		Description: "Distributor staff: services of the subtree",
		Grants: grants(baseGrants, map[string]Scope{
			PermOrganizationsRead: ScopeSubtree,
			PermServicesRead:      ScopeSubtree,
			PermServicesWrite:     ScopeSubtree,
			PermCustomersRead:     ScopeSubtree,
			PermCustomersWrite:    ScopeSubtree,
		}),
	},
	{
		Slug: RoleDistributorWarehouseStaff, Name: "Distributor warehouse staff", OrgType: OrgTypeDistributor,
		Description: "Distributor warehouse operator",
		Grants: grants(baseGrants, map[string]Scope{
			PermWarehouseRead:  ScopeManaged,
			PermWarehouseWrite: ScopeManaged,
		}),
	},
	{
		Slug: RoleDistributorAccounting, Name: "Distributor accounting", OrgType: OrgTypeDistributor,
		Description: "Distributor accounting and prices",
		Grants: grants(baseGrants, map[string]Scope{
			PermAccountingRead:         ScopeManaged,
			PermAccountingWrite:        ScopeManaged,
			PermPricingPurchaseRead:    ScopeManaged,
			PermPricingSaleRead:        ScopeManaged,
			PermPricingSaleWrite:       ScopeManaged,
			PermPricingRecommendedRead: ScopeManaged,
		}),
	},
	{
		Slug: RoleDealerOwner, Name: "Dealer owner", OrgType: OrgTypeDealer,
		Description: "Dealer owner: sets final prices (K8), sees accounting and dealer stock",
		Grants: grants(grants(baseGrants, ownerTenantGrants), map[string]Scope{
			PermOrganizationsRead:      ScopeManaged,
			PermMembersRead:            ScopeManaged,
			PermMembersWrite:           ScopeManaged,
			PermServicesRead:           ScopeManaged,
			PermServicesWrite:          ScopeManaged,
			PermCustomersRead:          ScopeManaged,
			PermCustomersWrite:         ScopeManaged,
			PermPricingPurchaseRead:    ScopeManaged,
			PermPricingSaleRead:        ScopeManaged,
			PermPricingSaleWrite:       ScopeManaged,
			PermPricingRecommendedRead: ScopeManaged,
			PermAccountingRead:         ScopeManaged,
			PermAccountingWrite:        ScopeManaged,
			PermWarehouseRead:          ScopeManaged,
			PermModulesRead:            ScopeManaged,
		}),
	},
	{
		Slug: RoleDealerStaff, Name: "Dealer staff", OrgType: OrgTypeDealer,
		Description: "Dealer staff: reads the dealer's services, writes own records; no prices or accounting",
		Grants: grants(baseGrants, map[string]Scope{
			PermServicesRead:   ScopeManaged,
			PermServicesWrite:  ScopeOwn,
			PermCustomersRead:  ScopeManaged,
			PermCustomersWrite: ScopeOwn,
		}),
	},
	{
		Slug: RoleDealerAccounting, Name: "Dealer accounting", OrgType: OrgTypeDealer,
		Description: "Dealer accounting",
		Grants: grants(baseGrants, map[string]Scope{
			PermAccountingRead:         ScopeManaged,
			PermAccountingWrite:        ScopeManaged,
			PermPricingPurchaseRead:    ScopeManaged,
			PermPricingSaleRead:        ScopeManaged,
			PermPricingRecommendedRead: ScopeManaged,
		}),
	},
	{
		Slug: RoleCustomer, Name: "Customer", OrgType: OrgTypeCustomer,
		Description: "Portal customer: own services and warranties",
		Grants: grants(baseGrants, map[string]Scope{
			PermServicesRead:  ScopeCustomer,
			PermCustomersRead: ScopeCustomer,
		}),
	},
	{
		Slug: RoleFleet, Name: "Fleet", OrgType: OrgTypeFleet,
		Description: "Fleet account (read-only)",
		Grants: grants(baseGrants, map[string]Scope{
			PermServicesRead:  ScopeCustomer,
			PermCustomersRead: ScopeCustomer,
		}),
	},
}

var (
	permIndex = map[string]int{}
	roleIndex = map[string]int{}
)

func init() {
	for i, p := range Permissions {
		permIndex[p.Slug] = i
	}
	for i, r := range Roles {
		roleIndex[r.Slug] = i
	}
}

// PermissionBySlug returns a catalog entry.
func PermissionBySlug(slug string) (PermissionDef, bool) {
	i, ok := permIndex[slug]
	if !ok {
		return PermissionDef{}, false
	}
	return Permissions[i], true
}

// RoleBySlug returns a system role package.
func RoleBySlug(slug string) (RoleDef, bool) {
	i, ok := roleIndex[slug]
	if !ok {
		return RoleDef{}, false
	}
	return Roles[i], true
}

// SortOrder is the display order of a permission.
func (p PermissionDef) SortOrder() int32 {
	return int32((permIndex[p.Slug] + 1) * 10)
}

// Allows reports whether the permission may be granted with scope s.
func (p PermissionDef) Allows(s Scope) bool {
	for _, sc := range p.Scopes {
		if sc == s {
			return true
		}
	}
	return false
}

// RoleGrants returns the effective grant set of a system role. super_admin
// receives every permission at its broadest scope.
func RoleGrants(r RoleDef) map[string]Scope {
	if r.Slug == RoleSuperAdmin {
		out := make(map[string]Scope, len(Permissions))
		for _, p := range Permissions {
			out[p.Slug] = Broadest(p.Scopes)
		}
		return out
	}
	return r.Grants
}

// SortedGrantSlugs returns the permission slugs of a grant map, sorted.
func SortedGrantSlugs(g map[string]Scope) []string {
	out := make([]string, 0, len(g))
	for k := range g {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
