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

	// TEC-88: PDF document templates (center admin editor). Appended last so
	// earlier sort orders stay stable; migration 000030 seeds them.
	platformPerm(PermPlatformDocumentTemplatesRead, "Read document templates"),
	platformPerm(PermPlatformDocumentTemplatesWrite, "Write document templates"),

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

	// TEC-92: WhatsApp gateway (wuzapi). Appended last so earlier sort orders
	// stay stable; migration 000034 seeds it after the existing rows.
	{
		Slug: PermWhatsAppManage, Name: "Manage WhatsApp integration", Module: "whatsapp", Scopes: scopesAll,
		Description: "Connect the WhatsApp number, send test messages, edit OTP/KVKK texts.",
	},

	// TEC-84: geography, territories (K5) and exchange rates (K7). Migration
	// 000035 runs before 000036, so on a fresh database these rows land
	// before TEC-87's; the catalog keeps the same order (sort = index * 10).
	withDesc(platformPerm(PermPlatformGeoWrite, "Write geography"),
		"Add or remove provinces and districts; activate countries."),
	withDesc(platformPerm(PermPlatformTerritoriesRead, "Read territories"),
		"Distributor territories (country, province, district)."),
	withDesc(platformPerm(PermPlatformTerritoriesWrite, "Write territories"),
		"Assign or remove distributor territories (K5)."),
	withDesc(platformPerm(PermPlatformRatesRead, "Read exchange rates"),
		"Daily TCMB/ECB rates and manual overrides."),
	withDesc(platformPerm(PermPlatformRatesWrite, "Write exchange rates"),
		"Manual rate overrides and on-demand fetch."),

	// TEC-87: notification center admin (template editor, channel switches,
	// delivery log). Appended last; migration 000036 seeds them.
	{
		Slug: PermNotificationTemplatesManage, Name: "Manage notification templates", Module: "notifications",
		Scopes: scopesAll, Description: "Edit notification templates and switch notification channels (SMS).",
	},
	{
		Slug: PermNotificationDeliveriesRead, Name: "Read notification deliveries", Module: "notifications",
		Scopes: scopesAll, Description: "Read the notification delivery log.",
	},

	// TEC-90: portal legal texts (AI guidelines). Appended last; migration
	// 000037 seeds it.
	{
		Slug: PermPlatformLegalTextsWrite, Name: "Write legal texts", Module: "platform", Scopes: scopesAll,
		Description: "Edit the portal legal texts (AI guidelines) in Markdown, per language.",
	},

	// TEC-144: product catalog. Appended last; migration 000039 seeds them.
	// Writes are center-only (K4); every organization role reads the
	// catalog of its brand.
	{
		Slug: PermCatalogRead, Name: "Read catalog", Module: "catalog", Scopes: scopesTree,
		Description: "Product categories and products of the active brand.",
	},
	{
		Slug: PermCatalogWrite, Name: "Write catalog", Module: "catalog", Scopes: scopesSupplier,
		Description: "Create and edit product categories and products (center only, K4).",
	},

	// TEC-149: vehicle catalog (global reference data). Appended last;
	// migration 000044 seeds them. Only super_admin writes (scope all);
	// every organization role and the portal roles read.
	{
		Slug: PermVehicleCatalogRead, Name: "Read vehicle catalog", Module: "vehicle_catalog", Scopes: scopesRecords,
		Description: "Car brands and models (global reference data).",
	},
	{
		Slug: PermVehicleCatalogWrite, Name: "Write vehicle catalog", Module: "vehicle_catalog", Scopes: scopesAll,
		Description: "Create and edit car brands, models, logos and hero images (super_admin only).",
	},

	// TEC-153: stock ledger. Appended last; migration 000046 seeds them.
	// center_warehouse holds them at scope all (K20, see
	// brandIndependentGrants); import and reclassification are center-only.
	{
		Slug: PermStockRead, Name: "Read stock", Module: "stock", Scopes: scopesTree,
		Description: "Units, barcode history and stock summaries of the organization.",
	},
	{
		Slug: PermStockWrite, Name: "Write stock", Module: "stock", Scopes: scopesTree,
		Description: "Stock entry, placement, transfers and receipts.",
	},
	{
		Slug: PermStockAdjust, Name: "Adjust stock", Module: "stock", Scopes: scopesTree, Sensitive: true,
		Description: "Count adjustments, voids and ledger repairs. Requires step-up.",
	},
	{
		Slug: PermStockReclassify, Name: "Reclassify stock", Module: "stock", Scopes: scopesSupplier, Sensitive: true,
		Description: "Approve moving a unit to another product (center only). Requires step-up.",
	},
	{
		Slug: PermStockImport, Name: "Import stock", Module: "stock", Scopes: scopesSupplier,
		Description: "Bulk stock import with dry run and undo (center only, K14).",
	},

	// TEC-171: accounting disputes (K24). Appended last; migration 000047
	// seeds it. Dealer roles read their ledger and dispute entries posted by
	// the parent; they hold no accounting.write (TEC-99 decision 7).
	{
		Slug: PermAccountingDispute, Name: "Dispute accounting entries", Module: "accounting", Scopes: scopesTree,
		Description: "Open a dispute on an entry posted by the parent organization (K24).",
	},

	// TEC-159: customer vehicles and the center-only customer operations
	// (K19). Appended last; migration 000048 seeds them.
	{
		Slug: PermVehiclesRead, Name: "Read vehicles", Module: "vehicles", Scopes: scopesRecords,
		Description: "Customer vehicles: plate, VIN, car brand and model.",
	},
	{
		Slug: PermVehiclesWrite, Name: "Write vehicles", Module: "vehicles", Scopes: scopesRecordsInt,
		Description: "Create and edit customer vehicles.",
	},
	{
		Slug: PermCustomersAnonymize, Name: "Anonymize customers", Module: "customers", Scopes: scopesSupplier,
		Sensitive:   true,
		Description: "KVKK/GDPR anonymization of a customer; services and warranties stay (center only, K19).",
	},
	{
		Slug: PermCustomersMerge, Name: "Merge customers", Module: "customers", Scopes: scopesSupplier,
		Sensitive:   true,
		Description: "Merge duplicate customer accounts into one user (center only).",
	},

	// TEC-165: orders (K6/K7) and sibling dealer transfers (K13). Appended
	// last; migration 000049 seeds them. Orders are bounded by the domain
	// brand (K20), so no role holds them at scope all.
	{
		Slug: PermOrdersRead, Name: "Read orders", Module: "orders", Scopes: scopesTree,
		Description: "Orders the organization sells or buys.",
	},
	{
		Slug: PermOrdersWrite, Name: "Write orders", Module: "orders", Scopes: scopesTree,
		Description: "Create and edit draft orders and submit them to the supplier.",
	},
	{
		Slug: PermOrdersApprove, Name: "Approve orders", Module: "orders", Scopes: scopesTree,
		Description: "Approve incoming orders as the seller; freezes prices and the exchange rate (K7).",
	},
	{
		Slug: PermOrdersShip, Name: "Ship orders", Module: "orders", Scopes: scopesTree,
		Description: "Assign barcodes, prepare, ship and deliver orders as the seller.",
	},
	{
		Slug: PermOrdersReceive, Name: "Receive orders", Module: "orders", Scopes: scopesTree,
		Description: "Confirm receipt of delivered orders as the buyer.",
	},
	{
		Slug: PermOrdersCancel, Name: "Cancel orders", Module: "orders", Scopes: scopesTree,
		Description: "Cancel orders; after shipping the order waits in cancelling until the goods return.",
	},
	{
		Slug: PermTransfersRequest, Name: "Request stock transfers", Module: "transfers", Scopes: scopesTree,
		Description: "Request a stock transfer to a sibling dealer (K13).",
	},
	{
		Slug: PermTransfersApprove, Name: "Approve stock transfers", Module: "transfers", Scopes: scopesTree,
		Description: "Approve or reject transfers between dealers of the distributor (K13).",
	},

	// TEC-178: service completion and cancellation. Appended last; migration
	// 000050 seeds them. Cancel is center-only (decision: only the center
	// cancels a service), so its scopes start at brand.
	{
		Slug: PermServicesComplete, Name: "Complete services", Module: "services", Scopes: scopesRecordsInt,
		Description: "Complete a service: consumes its stock and starts the warranty flow.",
	},
	{
		Slug: PermServicesCancel, Name: "Cancel services", Module: "services", Scopes: scopesSupplier,
		Description: "Cancel a service that is not completed (center only).",
	},

	// TEC-185: warranties and vehicle ownership transfer. Appended last;
	// migration 000051 seeds them. Void is center-only, so its scopes start
	// at brand.
	{
		Slug: PermWarrantiesRead, Name: "Read warranties", Module: "warranties", Scopes: scopesRecords,
		Description: "Warranties opened from completed services: period, status, covered product.",
	},
	{
		Slug: PermWarrantiesVoid, Name: "Void warranties", Module: "warranties", Scopes: scopesSupplier,
		Description: "Void a warranty (center only).",
	},
	{
		Slug: PermVehiclesTransfer, Name: "Transfer vehicles", Module: "vehicles", Scopes: scopesRecordsInt,
		Description: "Transfer a vehicle and its active warranties to a new owner with two codes.",
	},

	// TEC-174: dispute resolution (K24). Appended last; migration 000052
	// seeds it. The parent organization the disputed row came from resolves.
	{
		Slug: PermAccountingResolve, Name: "Resolve accounting disputes", Module: "accounting", Scopes: scopesTree,
		Description: "Resolve a dispute on an entry posted to a child: reversal, revision or rejection (K24).",
	},

	// TEC-214: center tasks. Appended last; migration 000058 seeds them.
	// Center roles only, so the scopes start at brand.
	{
		Slug: PermTasksRead, Name: "Read tasks", Module: "tasks", Scopes: scopesSupplier,
		Description: "Center tasks about distributors and dealers of the brand, with their comments.",
	},
	{
		Slug: PermTasksWrite, Name: "Write tasks", Module: "tasks", Scopes: scopesSupplier,
		Description: "Create, assign, update and close center tasks; comment on them (center only).",
	},
}

// BrandIndependentGrants lists the grants a non-super_admin role may hold at
// scope all. The warehouse is brand-independent (K20): the center warehouse
// manages every brand that enters its depot, so its stock grants are not
// bounded by the active brand.
var BrandIndependentGrants = map[string]map[string]bool{
	RoleCenterWarehouse: {
		PermStockRead: true, PermStockWrite: true, PermStockAdjust: true,
		PermStockReclassify: true, PermStockImport: true,
	},
}

func withDesc(p PermissionDef, desc string) PermissionDef {
	p.Description = desc
	return p
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
			PermVehicleCatalogRead: ScopeBrand,
			PermCatalogRead:        ScopeBrand,
			PermCatalogWrite:       ScopeBrand,
			// TEC-145 (000043): catalog import/export jobs of the center.
			PermTenantImportsRead:      ScopeManaged,
			PermTenantExportsRead:      ScopeManaged,
			PermOrganizationsRead:      ScopeBrand,
			PermMembersRead:            ScopeBrand,
			PermServicesRead:           ScopeBrand,
			PermServicesWrite:          ScopeBrand,
			PermCustomersRead:          ScopeBrand,
			PermCustomersWrite:         ScopeBrand,
			PermPricingRecommendedRead: ScopeBrand,
			// TEC-159 (000048).
			PermVehiclesRead:       ScopeBrand,
			PermVehiclesWrite:      ScopeBrand,
			PermCustomersAnonymize: ScopeBrand,
			PermCustomersMerge:     ScopeBrand,
			// TEC-165 (000049).
			PermOrdersRead:    ScopeBrand,
			PermOrdersWrite:   ScopeBrand,
			PermOrdersApprove: ScopeBrand,
			PermOrdersCancel:  ScopeBrand,
			// TEC-178 (000050).
			PermServicesComplete: ScopeBrand,
			PermServicesCancel:   ScopeBrand,
			// TEC-185 (000051).
			PermWarrantiesRead:   ScopeBrand,
			PermWarrantiesVoid:   ScopeBrand,
			PermVehiclesTransfer: ScopeBrand,
			// TEC-214 (000058).
			PermTasksRead:  ScopeBrand,
			PermTasksWrite: ScopeBrand,
		}),
	},
	{
		Slug: RoleCenterWarehouse, Name: "Center warehouse", OrgType: OrgTypeCenter,
		Description: "Center warehouse operator",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead: ScopeBrand,
			PermCatalogRead:        ScopeBrand,
			PermOrganizationsRead:  ScopeBrand,
			PermWarehouseRead:      ScopeBrand,
			PermWarehouseWrite:     ScopeBrand,
			// TEC-153 (000046): brand-independent stock (K20).
			PermStockRead:       ScopeAll,
			PermStockWrite:      ScopeAll,
			PermStockAdjust:     ScopeAll,
			PermStockReclassify: ScopeAll,
			PermStockImport:     ScopeAll,
			// TEC-158 (000053): preview, confirm and undo of its stock
			// import jobs run on /v1/tenant/imports.
			PermTenantImportsRead: ScopeManaged,
			// TEC-165 (000049): orders stay brand-bound (K20).
			PermOrdersRead: ScopeBrand,
			PermOrdersShip: ScopeBrand,
			// TEC-214 (000058).
			PermTasksRead:  ScopeBrand,
			PermTasksWrite: ScopeBrand,
		}),
	},
	{
		Slug: RoleCenterAccounting, Name: "Center accounting", OrgType: OrgTypeCenter,
		Description: "Center accounting and price management",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead:      ScopeBrand,
			PermCatalogRead:             ScopeBrand,
			PermOrganizationsRead:       ScopeBrand,
			PermAccountingRead:          ScopeBrand,
			PermAccountingWrite:         ScopeBrand,
			PermAccountingResolve:       ScopeBrand, // TEC-174 (000052)
			PermPricingPurchaseRead:     ScopeBrand,
			PermPricingSaleRead:         ScopeBrand,
			PermPricingSaleWrite:        ScopeBrand,
			PermPricingRecommendedRead:  ScopeBrand,
			PermPricingRecommendedWrite: ScopeBrand,
			PermOrdersRead:              ScopeBrand,
			PermTasksRead:               ScopeBrand, // TEC-214 (000058)
			PermTasksWrite:              ScopeBrand,
		}),
	},
	{
		Slug: RoleCenterSocial, Name: "Center social", OrgType: OrgTypeCenter,
		Description: "Leads, campaigns and social media for the brand",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead: ScopeBrand,
			PermCatalogRead:        ScopeBrand,
			PermOrganizationsRead:  ScopeBrand,
			PermCustomersRead:      ScopeBrand,
			PermVehiclesRead:       ScopeBrand,
			PermCampaignsRead:      ScopeBrand,
			PermCampaignsWrite:     ScopeBrand,
			PermLeadsRead:          ScopeBrand,
			PermLeadsWrite:         ScopeBrand,
			PermSocialRead:         ScopeBrand,
			PermSocialWrite:        ScopeBrand,
			PermTasksRead:          ScopeBrand, // TEC-214 (000058)
			PermTasksWrite:         ScopeBrand,
		}),
	},
	{
		Slug: RoleDistributorOwner, Name: "Distributor owner", OrgType: OrgTypeDistributor,
		Description: "Distributor owner: own prices, accounting and warehouse; services and dealers of the subtree",
		Grants: grants(grants(baseGrants, ownerTenantGrants), map[string]Scope{
			PermVehicleCatalogRead:     ScopeManaged,
			PermCatalogRead:            ScopeManaged,
			PermOrganizationsRead:      ScopeSubtree,
			PermOrganizationsWrite:     ScopeSubtree,
			PermMembersRead:            ScopeSubtree,
			PermMembersWrite:           ScopeSubtree,
			PermServicesRead:           ScopeSubtree,
			PermServicesWrite:          ScopeSubtree,
			PermCustomersRead:          ScopeSubtree,
			PermCustomersWrite:         ScopeSubtree,
			PermVehiclesRead:           ScopeSubtree,
			PermVehiclesWrite:          ScopeSubtree,
			PermPricingPurchaseRead:    ScopeManaged,
			PermPricingSaleRead:        ScopeManaged,
			PermPricingSaleWrite:       ScopeManaged,
			PermPricingRecommendedRead: ScopeManaged,
			PermAccountingRead:         ScopeManaged,
			PermAccountingWrite:        ScopeManaged,
			PermAccountingResolve:      ScopeManaged, // TEC-174 (000052)
			PermAccountingDispute:      ScopeManaged,
			PermWarehouseRead:          ScopeManaged,
			PermWarehouseWrite:         ScopeManaged,
			PermStockRead:              ScopeManaged,
			PermStockWrite:             ScopeManaged,
			PermStockAdjust:            ScopeManaged,
			PermModulesRead:            ScopeSubtree,
			PermModulesManage:          ScopeSubtree,
			// TEC-165 (000049): seller to its dealers, buyer from the center.
			PermOrdersRead:       ScopeManaged,
			PermOrdersWrite:      ScopeManaged,
			PermOrdersApprove:    ScopeManaged,
			PermOrdersShip:       ScopeManaged,
			PermOrdersReceive:    ScopeManaged,
			PermOrdersCancel:     ScopeManaged,
			PermTransfersApprove: ScopeManaged,
			// TEC-178 (000050).
			PermServicesComplete: ScopeSubtree,
			// TEC-185 (000051).
			PermWarrantiesRead:   ScopeSubtree,
			PermVehiclesTransfer: ScopeSubtree,
		}),
	},
	{
		Slug: RoleDistributorStaff, Name: "Distributor staff", OrgType: OrgTypeDistributor,
		Description: "Distributor staff: services of the subtree",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead: ScopeManaged,
			PermCatalogRead:        ScopeManaged,
			PermOrganizationsRead:  ScopeSubtree,
			PermServicesRead:       ScopeSubtree,
			PermServicesWrite:      ScopeSubtree,
			PermCustomersRead:      ScopeSubtree,
			PermCustomersWrite:     ScopeSubtree,
			PermVehiclesRead:       ScopeSubtree,
			PermVehiclesWrite:      ScopeSubtree,
			PermServicesComplete:   ScopeSubtree,
			PermWarrantiesRead:     ScopeSubtree,
			PermVehiclesTransfer:   ScopeSubtree,
		}),
	},
	{
		Slug: RoleDistributorWarehouseStaff, Name: "Distributor warehouse staff", OrgType: OrgTypeDistributor,
		Description: "Distributor warehouse operator",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead: ScopeManaged,
			PermCatalogRead:        ScopeManaged,
			PermWarehouseRead:      ScopeManaged,
			PermWarehouseWrite:     ScopeManaged,
			PermStockRead:          ScopeManaged,
			PermStockWrite:         ScopeManaged,
			PermStockAdjust:        ScopeManaged,
			PermOrdersRead:         ScopeManaged,
			PermOrdersShip:         ScopeManaged,
			PermOrdersReceive:      ScopeManaged,
		}),
	},
	{
		Slug: RoleDistributorAccounting, Name: "Distributor accounting", OrgType: OrgTypeDistributor,
		Description: "Distributor accounting and prices",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead:     ScopeManaged,
			PermCatalogRead:            ScopeManaged,
			PermAccountingRead:         ScopeManaged,
			PermAccountingWrite:        ScopeManaged,
			PermAccountingResolve:      ScopeManaged, // TEC-174 (000052)
			PermAccountingDispute:      ScopeManaged,
			PermPricingPurchaseRead:    ScopeManaged,
			PermPricingSaleRead:        ScopeManaged,
			PermPricingSaleWrite:       ScopeManaged,
			PermPricingRecommendedRead: ScopeManaged,
			PermOrdersRead:             ScopeManaged,
		}),
	},
	{
		Slug: RoleDealerOwner, Name: "Dealer owner", OrgType: OrgTypeDealer,
		Description: "Dealer owner: sets final prices (K8), sees accounting and dealer stock",
		Grants: grants(grants(baseGrants, ownerTenantGrants), map[string]Scope{
			PermVehicleCatalogRead:     ScopeManaged,
			PermCatalogRead:            ScopeManaged,
			PermOrganizationsRead:      ScopeManaged,
			PermMembersRead:            ScopeManaged,
			PermMembersWrite:           ScopeManaged,
			PermServicesRead:           ScopeManaged,
			PermServicesWrite:          ScopeManaged,
			PermCustomersRead:          ScopeManaged,
			PermCustomersWrite:         ScopeManaged,
			PermVehiclesRead:           ScopeManaged,
			PermVehiclesWrite:          ScopeManaged,
			PermPricingPurchaseRead:    ScopeManaged,
			PermPricingSaleRead:        ScopeManaged,
			PermPricingSaleWrite:       ScopeManaged,
			PermPricingRecommendedRead: ScopeManaged,
			// TEC-171: read and dispute only (TEC-99 decision 7).
			PermAccountingRead:    ScopeManaged,
			PermAccountingDispute: ScopeManaged,
			PermWarehouseRead:     ScopeManaged,
			PermStockRead:         ScopeManaged,
			PermModulesRead:       ScopeManaged,
			// TEC-165 (000049): buyer side and K13 transfer requests.
			PermOrdersRead:       ScopeManaged,
			PermOrdersWrite:      ScopeManaged,
			PermOrdersReceive:    ScopeManaged,
			PermOrdersCancel:     ScopeManaged,
			PermTransfersRequest: ScopeManaged,
			// TEC-178 (000050).
			PermServicesComplete: ScopeManaged,
			// TEC-185 (000051).
			PermWarrantiesRead:   ScopeManaged,
			PermVehiclesTransfer: ScopeManaged,
		}),
	},
	{
		Slug: RoleDealerStaff, Name: "Dealer staff", OrgType: OrgTypeDealer,
		Description: "Dealer staff: reads the dealer's services, writes own records; no prices or accounting",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead: ScopeManaged,
			PermCatalogRead:        ScopeManaged,
			PermServicesRead:       ScopeManaged,
			PermServicesWrite:      ScopeOwn,
			PermCustomersRead:      ScopeManaged,
			PermCustomersWrite:     ScopeOwn,
			PermVehiclesRead:       ScopeManaged,
			PermVehiclesWrite:      ScopeOwn,
			PermOrdersRead:         ScopeManaged,
			PermOrdersReceive:      ScopeManaged,
			PermServicesComplete:   ScopeOwn,
			PermWarrantiesRead:     ScopeManaged,
			PermVehiclesTransfer:   ScopeOwn,
		}),
	},
	{
		Slug: RoleDealerAccounting, Name: "Dealer accounting", OrgType: OrgTypeDealer,
		Description: "Dealer accounting",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead: ScopeManaged,
			PermCatalogRead:        ScopeManaged,
			// TEC-171: read and dispute only (TEC-99 decision 7).
			PermAccountingRead:         ScopeManaged,
			PermAccountingDispute:      ScopeManaged,
			PermPricingPurchaseRead:    ScopeManaged,
			PermPricingSaleRead:        ScopeManaged,
			PermPricingRecommendedRead: ScopeManaged,
			PermOrdersRead:             ScopeManaged,
		}),
	},
	{
		Slug: RoleCustomer, Name: "Customer", OrgType: OrgTypeCustomer,
		Description: "Portal customer: own services and warranties",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead: ScopeCustomer,
			PermServicesRead:       ScopeCustomer,
			PermCustomersRead:      ScopeCustomer,
			PermVehiclesRead:       ScopeCustomer,
			PermWarrantiesRead:     ScopeCustomer,
		}),
	},
	{
		Slug: RoleFleet, Name: "Fleet", OrgType: OrgTypeFleet,
		Description: "Fleet account (read-only)",
		Grants: grants(baseGrants, map[string]Scope{
			PermVehicleCatalogRead: ScopeCustomer,
			PermServicesRead:       ScopeCustomer,
			PermCustomersRead:      ScopeCustomer,
			PermVehiclesRead:       ScopeCustomer,
			PermWarrantiesRead:     ScopeCustomer,
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
