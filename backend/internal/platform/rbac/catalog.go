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
	{
		Slug: PermLeadsRead, Name: "Read leads", Module: "leads", Scopes: scopesTree,
		Description: "Read leads of the managed organization.",
	},
	{
		Slug: PermLeadsWrite, Name: "Write leads", Module: "leads", Scopes: scopesTree,
		Description: "Create and update leads of the managed organization.",
	},
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

	// TEC-266: Glorian sync (K2). Appended last; migration 000075 seeds
	// them. super_admin only for now; manage covers the API key, so it is
	// sensitive.
	{
		Slug: PermIntegrationsGlorianView, Name: "View Glorian integration", Module: "integrations", Scopes: scopesSupplier,
		Description: "Glorian hub connection, location map, sync runs, dealers and order outbound state (K2).",
	},
	{
		Slug: PermIntegrationsGlorianManage, Name: "Manage Glorian integration", Module: "integrations", Scopes: scopesSupplier,
		Sensitive:   true,
		Description: "Edit the Glorian hub connection and API key, location map; start syncs and retry outbound orders.",
	},

	// TEC-233: minimal measurement upload (K28). Appended last; migration
	// 000076 seeds it. Measurements are written into the active
	// organization only, so the single scope is managed.
	{
		Slug: PermMeasurementsWrite, Name: "Upload measurements", Module: "measurements", Scopes: scopesOrg,
		Description: "Upload paint thickness measurements from the mobile app into the active organization (K28).",
	},

	// TEC-285: contracts (F3-01). Appended last; migration 000083 seeds
	// them. Templates and void are center-only, so their scopes start at
	// brand; read and write follow services.read / services.write.
	{
		Slug: PermContractsTemplatesManage, Name: "Manage contract templates", Module: "contracts", Scopes: scopesSupplier,
		Description: "Create and edit contract templates, their locale texts and the default template per kind (center only).",
	},
	{
		Slug: PermContractsRead, Name: "Read contracts", Module: "contracts", Scopes: scopesRecords,
		Description: "Vehicle intake and service sale contracts with their signers, signatures and media.",
	},
	{
		Slug: PermContractsWrite, Name: "Write contracts", Module: "contracts", Scopes: scopesRecordsInt,
		Description: "Create contracts for services, attach media, collect OTP and signatures and execute them.",
	},
	{
		Slug: PermContractsVoid, Name: "Void contracts", Module: "contracts", Scopes: scopesSupplier,
		Description: "Void a contract, also an executed one, with a reason (center only).",
	},

	// TEC-305: service catalog and subscriptions. Appended last; migration
	// 000084 seeds them. manage and cancel_approve are center-only, so their
	// scopes start at brand.
	{
		Slug: PermServiceCatalogManage, Name: "Manage service catalog", Module: "service_catalog", Scopes: scopesSupplier,
		Description: "Define non-product services, module bundles and distributor prices (center only).",
	},
	{
		Slug: PermServiceCatalogRead, Name: "Read service catalog", Module: "service_catalog", Scopes: scopesTree,
		Description: "Non-product services of the brand with their prices.",
	},
	{
		Slug: PermServiceSubscriptionsAssign, Name: "Assign service subscriptions", Module: "service_subscriptions", Scopes: scopesTree,
		Description: "Assign a catalog service to a distributor or dealer (distributors without a margin).",
	},
	{
		Slug: PermServiceSubscriptionsRead, Name: "Read service subscriptions", Module: "service_subscriptions", Scopes: scopesTree,
		Description: "Service subscriptions, their periods and cancellation requests.",
	},
	{
		Slug: PermServiceSubscriptionsCancelRequest, Name: "Request subscription cancellation", Module: "service_subscriptions", Scopes: scopesTree,
		Description: "Request early cancellation of a service subscription of the organization.",
	},
	{
		Slug: PermServiceSubscriptionsCancelApprove, Name: "Approve subscription cancellation", Module: "service_subscriptions", Scopes: scopesSupplier,
		Description: "Approve or reject early cancellation requests; the static cancellation fee applies (center only).",
	},

	// TEC-293: measurement read side, service links and device registry
	// (K28). Appended last; migration 000085 seeds them.
	{
		Slug: PermMeasurementsRead, Name: "Read measurements", Module: "measurements", Scopes: []Scope{ScopeManaged, ScopeSubtree},
		Description: "Read paint thickness measurements, their readings, tires and service links (K28).",
	},
	{
		Slug: PermMeasurementsLink, Name: "Link measurements", Module: "measurements", Scopes: scopesOrg,
		Description: "Link a measurement to a service as its before or after measurement and confirm the link (K28).",
	},
	{
		Slug: PermMeasurementDevicesManage, Name: "Manage measuring devices", Module: "measurements", Scopes: scopesOrg,
		Description: "Register, edit and deactivate the measuring devices of the active organization (K28).",
	},

	// TEC-329: announcements and the document library (F3-05). Appended
	// last; migration 000086 seeds them. Every network role reads in its
	// own organization (managed); the center writes for its brand, the
	// distributor owner writes announcements for its subtree.
	{
		Slug: PermAnnouncementsRead, Name: "Read announcements", Module: "announcements", Scopes: scopesOrg,
		Description: "Read the announcements published to the organization and mark them as read.",
	},
	{
		Slug: PermAnnouncementsWrite, Name: "Write announcements", Module: "announcements",
		Scopes:      []Scope{ScopeSubtree, ScopeBrand, ScopeAll},
		Description: "Write, publish and archive announcements (center: the brand network; distributor: its subtree).",
	},
	{
		Slug: PermLibraryRead, Name: "Read the document library", Module: "library", Scopes: scopesOrg,
		Description: "Browse and download the guides and materials of the document library allowed to the organization.",
	},
	{
		Slug: PermLibraryManage, Name: "Manage the document library", Module: "library", Scopes: scopesSupplier,
		Description: "Create folders, upload documents and new language versions, set access levels (center only).",
	},

	// TEC-312: quotes and lead conversion (F3-03). Appended last; migration
	// 000087 seeds these new rows and updates the old lead grants.
	{
		Slug: PermQuotesRead, Name: "Read quotes", Module: "quotes", Scopes: scopesTree,
		Description: "Read quotes and quote deliveries of the managed organization.",
	},
	{
		Slug: PermQuotesWrite, Name: "Write quotes", Module: "quotes", Scopes: scopesTree,
		Description: "Create, edit, send and decide quotes of the managed organization.",
	},
	{
		Slug: PermLeadsConvertOrg, Name: "Convert lead to organization", Module: "leads",
		Scopes:      []Scope{ScopeSubtree, ScopeBrand, ScopeAll},
		Description: "Convert dealer candidates in the distributor subtree or center brand; distributor candidates are center-only.",
	},

	// TEC-356: completed service cancellation. Appended last; migration
	// 000088 seeds it. Unlike services.cancel, distributor and dealer owners
	// may use this on services they manage; dealer_staff does not get it.
	{
		Slug: PermServicesCancelCompleted, Name: "Cancel completed services", Module: "services", Scopes: scopesRecordsInt,
		Description: "Cancel a completed service with warranty void, stock return and accounting reversal.",
	},

	// TEC-322: appointments and capacity (F3-04). Appended last; migration
	// 000090 seeds them.
	{
		Slug: PermAppointmentsRead, Name: "Read appointments", Module: "appointments",
		Scopes:      []Scope{ScopeManaged, ScopeSubtree, ScopeAll},
		Description: "Read appointment calendars and capacity of managed organizations.",
	},
	{
		Slug: PermAppointmentsWrite, Name: "Write appointments", Module: "appointments",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Create, update and cancel appointments of the managed organization.",
	},
	{
		Slug: PermAppointmentSettingsManage, Name: "Manage appointment settings", Module: "appointments",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Manage appointment capacity, working hours, closures and portal booking settings.",
	},

	// TEC-350: extra review questions (F3-09). Appended last; migration
	// 000091 seeds them.
	{
		Slug: PermReviewsQuestionsManage, Name: "Manage review questions", Module: "reviews", Scopes: scopesSupplier,
		Description: "Define the extra review questions and their translations (center only).",
	},
	{
		Slug: PermReviewsRead, Name: "Read service reviews", Module: "reviews", Scopes: scopesTree,
		Description: "Read service reviews and their answers (dealer: own, distributor: subtree, center: brand).",
	},

	// TEC-334: warranty claims (F3-06). Appended last; migration 000092
	// seeds them.
	{
		Slug: PermWarrantyClaimsRead, Name: "Read warranty claims", Module: "warranty_claims",
		Scopes:      []Scope{ScopeManaged, ScopeSubtree, ScopeBrand, ScopeAll},
		Description: "Read warranty claims, their parts, photos and timeline.",
	},
	{
		Slug: PermWarrantyClaimsWrite, Name: "Write warranty claims", Module: "warranty_claims",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Open warranty claims of the managed organization and add parts, photos and notes.",
	},
	{
		Slug: PermWarrantyClaimsReview, Name: "Review warranty claims", Module: "warranty_claims",
		Scopes:      []Scope{ScopeSubtree, ScopeAll},
		Description: "Review dealer warranty claims of the subtree and forward them to the center.",
	},
	{
		Slug: PermWarrantyClaimsDecide, Name: "Decide warranty claims", Module: "warranty_claims",
		Scopes:      []Scope{ScopeBrand, ScopeAll},
		Description: "Approve or reject warranty claims of the brand.",
	},

	// TEC-341: full dealer accounting (F3-07). Appended last; migration
	// 000093 seeds them.
	{
		Slug: PermDealerPricingWrite, Name: "Write dealer product prices", Module: "dealer_accounting",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Set the dealer's own sale price of products.",
	},
	{
		Slug: PermProductSalesWrite, Name: "Write product sales", Module: "dealer_accounting",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Record quick product sales outside services.",
	},
	{
		Slug: PermSuppliersManage, Name: "Manage suppliers", Module: "dealer_accounting",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Create and update the organization's suppliers.",
	},
	{
		Slug: PermPurchasesWrite, Name: "Write purchases", Module: "dealer_accounting",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Record purchases from suppliers (no stock movement).",
	},
	{
		Slug: PermStaffManage, Name: "Manage staff", Module: "dealer_accounting",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Manage staff cards and salaries of the organization.",
	},
	{
		Slug: PermStaffPaymentsWrite, Name: "Write staff payments", Module: "dealer_accounting",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Record salary, advance and bonus payments to staff.",
	},

	// TEC-383: AI assistant (F4-01). Appended last; migration 000101 seeds
	// them.
	{
		Slug: PermAIUse, Name: "Use AI assistant", Module: "ai", Scopes: scopesSelf,
		Description: "Chat with the AI assistant in the panel; own conversations only.",
	},
	{
		Slug: PermAIUsageRead, Name: "Read AI usage", Module: "ai", Scopes: scopesTree,
		Description: "Read AI token usage and quotas of the organizations in scope.",
	},
	{
		Slug: PermAISettingsManage, Name: "Manage AI settings", Module: "ai",
		Scopes: scopesAll, SuperAdminOnly: true,
		Description: "Manage platform AI settings, models, quotas and the knowledge text.",
	},
	{
		Slug: PermAIActionsConfirm, Name: "Confirm AI actions", Module: "ai", Scopes: scopesSelf,
		Description: "Confirm or cancel write actions proposed by the AI assistant for oneself.",
	},

	// TEC-393: WhatsApp conversations (F4-02). Appended last; migration
	// 000103 seeds them. Platform admin only (F4 user answer S2).
	{
		Slug: PermConversationsRead, Name: "Read conversations", Module: "conversations",
		Scopes: scopesAll, SuperAdminOnly: true,
		Description: "Read WhatsApp conversations, their messages and AI runs.",
	},
	{
		Slug: PermConversationsReply, Name: "Reply to conversations", Module: "conversations",
		Scopes: scopesAll, SuperAdminOnly: true,
		Description: "Send staff replies and attachments in WhatsApp conversations.",
	},
	{
		Slug: PermConversationsManage, Name: "Manage conversations", Module: "conversations",
		Scopes: scopesAll, SuperAdminOnly: true,
		Description: "Assign, close and reopen WhatsApp conversations and change their AI mode.",
	},
	// TEC-400: MCP OAuth (F4-03). Appended last; migration 000104 seeds them.
	{
		Slug: PermMCPConnect, Name: "Connect MCP clients", Module: "mcp", Scopes: scopesSelf,
		Description: "Connect AI agents (MCP clients) to the panel MCP endpoints with one's own permissions.",
	},
	{
		Slug: PermMCPClientsManage, Name: "Manage MCP clients", Module: "mcp",
		Scopes: scopesAll, SuperAdminOnly: true,
		Description: "List and revoke registered MCP (OAuth) clients platform wide.",
	},
	// TEC-404: campaign approval (F4-04). Appended last; migration 000107
	// seeds it.
	{
		Slug: PermCampaignsApprove, Name: "Approve campaigns", Module: "campaigns", Scopes: scopesTree,
		Description: "Approve, reject or send back campaigns submitted to the organization for approval.",
	},

	// TEC-479: certificates addon (F5-03). Appended last; migration 000111
	// seeds them.
	{
		Slug: PermCertificateTypesManage, Name: "Manage certificate types", Module: "certificates",
		Scopes:      []Scope{ScopeBrand, ScopeAll},
		Description: "Create and update certificate types and their product/category bindings.",
	},
	{
		Slug: PermCertificatesRead, Name: "Read certificates", Module: "certificates",
		Scopes:      []Scope{ScopeManaged, ScopeSubtree, ScopeBrand, ScopeAll},
		Description: "Read user certificates and service certificate warnings in scope.",
	},
	{
		Slug: PermCertificatesWrite, Name: "Write certificates", Module: "certificates",
		Scopes:      []Scope{ScopeManaged, ScopeSubtree, ScopeAll},
		Description: "Upload certificates for staff in the managed organization or subtree.",
	},
	{
		Slug: PermCertificatesVerify, Name: "Verify certificates", Module: "certificates",
		Scopes:      []Scope{ScopeSubtree, ScopeBrand, ScopeAll},
		Description: "Verify or reject certificates of the brand or distributor subtree.",
	},
	{
		Slug: PermCertificatesApproveService, Name: "Approve certificate service warnings", Module: "certificates",
		Scopes:      []Scope{ScopeBrand, ScopeAll},
		Description: "Approve or reject service status changes blocked by certificate warnings.",
	},

	// TEC-472: fleets (F5-02). Appended last; migration 000112 seeds them.
	{
		Slug: PermFleetsRead, Name: "Read fleets", Module: "fleet", Scopes: scopesTree,
		Description: "Fleets linked to the organization, their vehicles, services and reports.",
	},
	{
		Slug: PermFleetsManage, Name: "Manage fleets", Module: "fleet", Scopes: scopesTree,
		Description: "Open fleets, invite fleet users, add vehicles and manage dealer links.",
	},
	{
		Slug: PermFleetsPlan, Name: "Plan fleet services", Module: "fleet",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Bulk service plans and bulk vehicle intake for a linked fleet.",
	},
	{
		Slug: PermFleetPortalRead, Name: "Read own fleet (portal)", Module: "fleet", Scopes: scopesSelf,
		Description: "Fleet portal: vehicles, services, warranties, accounts and reports of one's own fleet.",
	},

	// TEC-466: dealer showcase (F5-01). Appended last; migration 000113 seeds
	// them.
	{
		Slug: PermShowcaseRead, Name: "Read showcase", Module: "dealer_showcase", Scopes: scopesTree,
		Description: "Read the dealer showcase (profile, working hours, services, photos) of the organizations in scope.",
	},
	{
		Slug: PermShowcaseWrite, Name: "Write showcase", Module: "dealer_showcase", Scopes: scopesTree,
		Description: "Edit the dealer showcase and submit it for publication.",
	},
	{
		Slug: PermPlatformShowcaseReview, Name: "Review showcases", Module: "dealer_showcase", Scopes: scopesSupplier,
		Description: "Approve or reject dealer showcases submitted for publication.",
	},

	// TEC-483: stock forecast schema and thresholds (F5-04a). Appended last;
	// migration 000115 seeds them. Dealer/distributor owners manage only
	// their own thresholds; network demand is center-only.
	{
		Slug: PermStockForecastRead, Name: "Read stock forecasts", Module: "stock_forecast",
		Scopes:      []Scope{ScopeManaged, ScopeSubtree, ScopeBrand, ScopeAll},
		Description: "Read stock forecast snapshots and product history.",
	},
	{
		Slug: PermStockForecastManage, Name: "Manage stock forecast thresholds", Module: "stock_forecast",
		Scopes:      []Scope{ScopeManaged, ScopeBrand, ScopeAll},
		Description: "Edit stock forecast warning and cover thresholds.",
	},
	{
		Slug: PermStockForecastNetworkRead, Name: "Read network stock demand forecasts", Module: "stock_forecast",
		Scopes:      []Scope{ScopeBrand, ScopeAll},
		Description: "Read center network demand forecasts for the brand.",
	},

	// TEC-487: efficiency and waste analytics (F5-06a). Appended last;
	// migration 000116 seeds them.
	{
		Slug: PermEfficiencyRead, Name: "Read efficiency analytics", Module: "efficiency", Scopes: scopesTree,
		Description: "Read part consumption, roll efficiency and waste analytics in scope.",
	},
	{
		Slug: PermEfficiencyExpectationsManage, Name: "Manage efficiency expectations", Module: "efficiency",
		Scopes:      []Scope{ScopeBrand, ScopeAll},
		Description: "Create and update expected part consumption definitions.",
	},

	// TEC-490: performance and targets (F5-05a). Appended last; migration
	// 000123 seeds them.
	{
		Slug: PermPerformanceRead, Name: "Read performance", Module: "performance", Scopes: scopesTree,
		Description: "Read monthly performance metrics, rankings and target achievement in scope.",
	},
	{
		Slug: PermPerformanceTargetsManage, Name: "Manage performance targets", Module: "performance",
		Scopes:      []Scope{ScopeSubtree, ScopeBrand, ScopeAll},
		Description: "Set service count and order volume targets for the organizations below.",
	},
	{
		Slug: PermPerformanceStaffTargetsManage, Name: "Manage staff targets", Module: "performance",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Set monthly individual targets for the staff of the organization.",
	},
	{
		Slug: PermPerformanceBonusManage, Name: "Manage staff bonuses", Module: "performance",
		Scopes:      []Scope{ScopeManaged, ScopeAll},
		Description: "Define target-based bonus rules and approve bonus accruals of the organization.",
	},
	{
		Slug: PermPerformanceRulesManage, Name: "Manage weak dealer rules", Module: "performance",
		Scopes:      []Scope{ScopeSubtree, ScopeBrand, ScopeAll},
		Description: "Define weak dealer rules that open tasks or send notifications.",
	},

	// TEC-505: recommended price versions (F5-09a). Appended last; migration
	// 000124 seeds it.
	{
		Slug: PermPricingDisciplineRead, Name: "Read price discipline", Module: "pricing",
		Scopes:      []Scope{ScopeSubtree, ScopeBrand, ScopeAll},
		Description: "Read dealer and distributor end-customer prices against the recommended price.",
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
	RoleCenterSocial: {
		PermLeadsRead: true, PermLeadsWrite: true,
		PermAppointmentsRead: true,
	},
	RoleCenterStaff: {
		PermAppointmentsRead: true,
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
			PermServicesComplete:        ScopeBrand,
			PermServicesCancel:          ScopeBrand,
			PermServicesCancelCompleted: ScopeBrand,
			// TEC-185 (000051).
			PermWarrantiesRead:   ScopeBrand,
			PermWarrantiesVoid:   ScopeBrand,
			PermVehiclesTransfer: ScopeBrand,
			// TEC-214 (000058).
			PermTasksRead:  ScopeBrand,
			PermTasksWrite: ScopeBrand,
			// TEC-228 (000070): decides returns sent to the center.
			PermTransfersApprove: ScopeBrand,
			// TEC-233 (000076).
			PermMeasurementsWrite: ScopeManaged,
			// TEC-285 (000083).
			PermContractsTemplatesManage: ScopeBrand,
			PermContractsRead:            ScopeBrand,
			PermContractsWrite:           ScopeBrand,
			PermContractsVoid:            ScopeBrand,
			// TEC-305 (000084).
			PermServiceCatalogManage:              ScopeBrand,
			PermServiceCatalogRead:                ScopeBrand,
			PermServiceSubscriptionsAssign:        ScopeBrand,
			PermServiceSubscriptionsRead:          ScopeBrand,
			PermServiceSubscriptionsCancelApprove: ScopeBrand,
			// TEC-293 (000085).
			PermMeasurementsRead:         ScopeSubtree,
			PermMeasurementsLink:         ScopeManaged,
			PermMeasurementDevicesManage: ScopeManaged,
			// TEC-329 (000086).
			PermAnnouncementsRead:  ScopeManaged,
			PermAnnouncementsWrite: ScopeBrand,
			PermLibraryRead:        ScopeManaged,
			PermLibraryManage:      ScopeBrand,
			// TEC-312 (000087).
			PermLeadsRead:       ScopeManaged,
			PermLeadsWrite:      ScopeManaged,
			PermQuotesRead:      ScopeManaged,
			PermQuotesWrite:     ScopeManaged,
			PermLeadsConvertOrg: ScopeBrand,
			// TEC-322 (000090).
			PermAppointmentsRead:          ScopeAll,
			PermAppointmentsWrite:         ScopeManaged,
			PermAppointmentSettingsManage: ScopeManaged,
			// TEC-350 (000091).
			PermReviewsQuestionsManage: ScopeBrand,
			PermReviewsRead:            ScopeBrand,
			// TEC-334 (000092).
			PermWarrantyClaimsRead:   ScopeBrand,
			PermWarrantyClaimsDecide: ScopeBrand,
			// TEC-479 (000111).
			PermCertificateTypesManage:     ScopeBrand,
			PermCertificatesRead:           ScopeBrand,
			PermCertificatesVerify:         ScopeBrand,
			PermCertificatesApproveService: ScopeBrand,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			PermAIUsageRead:      ScopeBrand,
			// TEC-400 (000104).
			PermMCPConnect: ScopeOwn,
			// TEC-472 (000112).
			PermFleetsRead:   ScopeBrand,
			PermFleetsManage: ScopeBrand,
			// TEC-466 (000113).
			PermShowcaseRead:           ScopeBrand,
			PermPlatformShowcaseReview: ScopeBrand,
			// TEC-483 (000115).
			PermStockForecastRead:        ScopeBrand,
			PermStockForecastManage:      ScopeBrand,
			PermStockForecastNetworkRead: ScopeBrand,
			// TEC-487 (000116).
			PermEfficiencyRead:               ScopeBrand,
			PermEfficiencyExpectationsManage: ScopeBrand,
			// TEC-490 (000123).
			PermPerformanceRead:          ScopeBrand,
			PermPerformanceTargetsManage: ScopeBrand,
			PermPerformanceRulesManage:   ScopeBrand,
			// TEC-505 (000124).
			PermPricingDisciplineRead: ScopeBrand,
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
			// TEC-228 (000070): decides and receives returns sent to the
			// center.
			PermTransfersApprove: ScopeBrand,
			// TEC-329 (000086).
			PermAnnouncementsRead: ScopeManaged,
			PermLibraryRead:       ScopeManaged,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			// TEC-400 (000104).
			PermMCPConnect: ScopeOwn,
			// TEC-483 (000115).
			PermStockForecastRead:        ScopeBrand,
			PermStockForecastManage:      ScopeBrand,
			PermStockForecastNetworkRead: ScopeBrand,
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
			// TEC-305 (000084).
			PermServiceCatalogManage:              ScopeBrand,
			PermServiceCatalogRead:                ScopeBrand,
			PermServiceSubscriptionsAssign:        ScopeBrand,
			PermServiceSubscriptionsRead:          ScopeBrand,
			PermServiceSubscriptionsCancelApprove: ScopeBrand,
			// TEC-329 (000086).
			PermAnnouncementsRead: ScopeManaged,
			PermLibraryRead:       ScopeManaged,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			PermAIUsageRead:      ScopeBrand,
			// TEC-400 (000104).
			PermMCPConnect: ScopeOwn,
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
			PermLeadsRead:          ScopeAll,
			PermLeadsWrite:         ScopeAll,
			PermSocialRead:         ScopeBrand,
			PermSocialWrite:        ScopeBrand,
			PermTasksRead:          ScopeBrand, // TEC-214 (000058)
			PermTasksWrite:         ScopeBrand,
			// TEC-329 (000086).
			PermAnnouncementsRead:  ScopeManaged,
			PermAnnouncementsWrite: ScopeBrand,
			PermLibraryRead:        ScopeManaged,
			PermLibraryManage:      ScopeBrand,
			// TEC-312 (000087).
			PermQuotesRead:      ScopeManaged,
			PermQuotesWrite:     ScopeManaged,
			PermLeadsConvertOrg: ScopeBrand,
			// TEC-322 (000090).
			PermAppointmentsRead:  ScopeAll,
			PermAppointmentsWrite: ScopeManaged,
			// TEC-350 (000091).
			PermReviewsRead: ScopeBrand,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			// TEC-400 (000104).
			PermMCPConnect: ScopeOwn,
			// TEC-404 (000107).
			PermCampaignsApprove: ScopeBrand,
			// TEC-466 (000113).
			PermShowcaseRead:           ScopeBrand,
			PermPlatformShowcaseReview: ScopeBrand,
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
			// TEC-216 (000061): reads dealer stock; never moves it.
			PermStockRead:     ScopeSubtree,
			PermStockWrite:    ScopeManaged,
			PermStockAdjust:   ScopeManaged,
			PermModulesRead:   ScopeSubtree,
			PermModulesManage: ScopeSubtree,
			// TEC-165 (000049): seller to its dealers, buyer from the center.
			PermOrdersRead:       ScopeManaged,
			PermOrdersWrite:      ScopeManaged,
			PermOrdersApprove:    ScopeManaged,
			PermOrdersShip:       ScopeManaged,
			PermOrdersReceive:    ScopeManaged,
			PermOrdersCancel:     ScopeManaged,
			PermTransfersApprove: ScopeManaged,
			// TEC-178 (000050).
			PermServicesComplete:        ScopeSubtree,
			PermServicesCancelCompleted: ScopeSubtree,
			// TEC-185 (000051).
			PermWarrantiesRead:   ScopeSubtree,
			PermVehiclesTransfer: ScopeSubtree,
			// TEC-197 (000055): transfers to sibling distributors.
			PermTransfersRequest: ScopeManaged,
			// TEC-233 (000076).
			PermMeasurementsWrite: ScopeManaged,
			// TEC-285 (000083).
			PermContractsRead:  ScopeSubtree,
			PermContractsWrite: ScopeSubtree,
			// TEC-305 (000084): assigns to its subtree without a margin.
			PermServiceCatalogRead:                ScopeManaged,
			PermServiceSubscriptionsAssign:        ScopeSubtree,
			PermServiceSubscriptionsRead:          ScopeSubtree,
			PermServiceSubscriptionsCancelRequest: ScopeManaged,
			// TEC-293 (000085).
			PermMeasurementsRead:         ScopeSubtree,
			PermMeasurementsLink:         ScopeManaged,
			PermMeasurementDevicesManage: ScopeManaged,
			// TEC-329 (000086).
			PermAnnouncementsRead:  ScopeManaged,
			PermAnnouncementsWrite: ScopeSubtree,
			PermLibraryRead:        ScopeManaged,
			// TEC-312 (000087).
			PermLeadsRead:       ScopeManaged,
			PermLeadsWrite:      ScopeManaged,
			PermQuotesRead:      ScopeManaged,
			PermQuotesWrite:     ScopeManaged,
			PermLeadsConvertOrg: ScopeSubtree,
			// TEC-322 (000090).
			PermAppointmentsRead:          ScopeSubtree,
			PermAppointmentsWrite:         ScopeManaged,
			PermAppointmentSettingsManage: ScopeManaged,
			// TEC-350 (000091).
			PermReviewsRead: ScopeSubtree,
			// TEC-334 (000092).
			PermWarrantyClaimsRead:   ScopeSubtree,
			PermWarrantyClaimsWrite:  ScopeManaged,
			PermWarrantyClaimsReview: ScopeSubtree,
			// TEC-479 (000111).
			PermCertificatesRead:   ScopeSubtree,
			PermCertificatesWrite:  ScopeSubtree,
			PermCertificatesVerify: ScopeSubtree,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			PermAIUsageRead:      ScopeManaged,
			// TEC-400 (000104).
			PermMCPConnect: ScopeOwn,
			// TEC-404 (000107): own campaigns; approves those of the
			// dealers below it.
			PermCampaignsRead:    ScopeSubtree,
			PermCampaignsWrite:   ScopeManaged,
			PermCampaignsApprove: ScopeSubtree,
			// TEC-472 (000112): fleets linked to the subtree; a serving
			// distributor links a fleet itself.
			PermFleetsRead:   ScopeSubtree,
			PermFleetsManage: ScopeSubtree,
			// TEC-466 (000113): own showcase and those of its dealers.
			PermShowcaseRead:  ScopeSubtree,
			PermShowcaseWrite: ScopeSubtree,
			// TEC-483 (000115).
			PermStockForecastRead:   ScopeSubtree,
			PermStockForecastManage: ScopeManaged,
			// TEC-487 (000116): subtree comparison and roll analytics.
			PermEfficiencyRead: ScopeSubtree,
			// TEC-490 (000123): targets and weak dealer rules for the dealers
			// below (the usecase excludes the distributor itself).
			PermPerformanceRead:          ScopeSubtree,
			PermPerformanceTargetsManage: ScopeSubtree,
			PermPerformanceRulesManage:   ScopeSubtree,
			// TEC-505 (000124): deviations of the subtree.
			PermPricingDisciplineRead: ScopeSubtree,
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
			PermMeasurementsWrite:  ScopeManaged, // TEC-233 (000076)
			PermContractsRead:      ScopeSubtree, // TEC-285 (000083)
			PermContractsWrite:     ScopeSubtree,
			PermMeasurementsRead:   ScopeSubtree, // TEC-293 (000085)
			PermMeasurementsLink:   ScopeManaged, // TEC-293 (000085)
			// TEC-329 (000086).
			PermAnnouncementsRead: ScopeManaged,
			PermLibraryRead:       ScopeManaged,
			// TEC-312 (000087).
			PermLeadsRead:   ScopeManaged,
			PermLeadsWrite:  ScopeManaged,
			PermQuotesRead:  ScopeManaged,
			PermQuotesWrite: ScopeManaged,
			// TEC-322 (000090).
			PermAppointmentsRead:  ScopeSubtree,
			PermAppointmentsWrite: ScopeManaged,
			// TEC-350 (000091).
			PermReviewsRead: ScopeSubtree,
			// TEC-334 (000092).
			PermWarrantyClaimsRead:   ScopeSubtree,
			PermWarrantyClaimsWrite:  ScopeManaged,
			PermWarrantyClaimsReview: ScopeSubtree,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			// TEC-483 (000115).
			PermStockForecastRead: ScopeSubtree,
			// TEC-490 (000123).
			PermPerformanceRead: ScopeSubtree,
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
			PermStockRead:          ScopeSubtree, // TEC-216 (000061)
			PermStockWrite:         ScopeManaged,
			PermStockAdjust:        ScopeManaged,
			PermOrdersRead:         ScopeManaged,
			PermOrdersShip:         ScopeManaged,
			PermOrdersReceive:      ScopeManaged,
			// TEC-329 (000086).
			PermAnnouncementsRead: ScopeManaged,
			PermLibraryRead:       ScopeManaged,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			// TEC-483 (000115).
			PermStockForecastRead: ScopeSubtree,
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
			// TEC-305 (000084).
			PermServiceSubscriptionsRead: ScopeManaged,
			// TEC-329 (000086).
			PermAnnouncementsRead: ScopeManaged,
			PermLibraryRead:       ScopeManaged,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
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
			// TEC-201 (000059): no warehouse.read; locations and counts
			// belong to the center and the distributor (K12).
			PermStockRead:   ScopeManaged,
			PermModulesRead: ScopeManaged,
			// TEC-165 (000049): buyer side and K13 transfer requests.
			PermOrdersRead:       ScopeManaged,
			PermOrdersWrite:      ScopeManaged,
			PermOrdersReceive:    ScopeManaged,
			PermOrdersCancel:     ScopeManaged,
			PermTransfersRequest: ScopeManaged,
			// TEC-178 (000050).
			PermServicesComplete:        ScopeManaged,
			PermServicesCancelCompleted: ScopeManaged,
			// TEC-185 (000051).
			PermWarrantiesRead:   ScopeManaged,
			PermVehiclesTransfer: ScopeManaged,
			// TEC-233 (000076).
			PermMeasurementsWrite: ScopeManaged,
			// TEC-285 (000083).
			PermContractsRead:  ScopeManaged,
			PermContractsWrite: ScopeManaged,
			// TEC-305 (000084).
			PermServiceSubscriptionsRead:          ScopeManaged,
			PermServiceSubscriptionsCancelRequest: ScopeManaged,
			// TEC-293 (000085).
			PermMeasurementsRead:         ScopeManaged,
			PermMeasurementsLink:         ScopeManaged,
			PermMeasurementDevicesManage: ScopeManaged,
			// TEC-329 (000086).
			PermAnnouncementsRead: ScopeManaged,
			PermLibraryRead:       ScopeManaged,
			// TEC-312 (000087).
			PermLeadsRead:   ScopeManaged,
			PermLeadsWrite:  ScopeManaged,
			PermQuotesRead:  ScopeManaged,
			PermQuotesWrite: ScopeManaged,
			// TEC-322 (000090).
			PermAppointmentsRead:          ScopeManaged,
			PermAppointmentsWrite:         ScopeManaged,
			PermAppointmentSettingsManage: ScopeManaged,
			// TEC-350 (000091).
			PermReviewsRead: ScopeManaged,
			// TEC-334 (000092).
			PermWarrantyClaimsRead:  ScopeManaged,
			PermWarrantyClaimsWrite: ScopeManaged,
			// TEC-479 (000111).
			PermCertificatesRead:  ScopeManaged,
			PermCertificatesWrite: ScopeManaged,
			// TEC-341 (000093).
			PermAccountingWrite:    ScopeManaged,
			PermDealerPricingWrite: ScopeManaged,
			PermProductSalesWrite:  ScopeManaged,
			PermSuppliersManage:    ScopeManaged,
			PermPurchasesWrite:     ScopeManaged,
			PermStaffManage:        ScopeManaged,
			PermStaffPaymentsWrite: ScopeManaged,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			PermAIUsageRead:      ScopeManaged,
			// TEC-400 (000104).
			PermMCPConnect: ScopeOwn,
			// TEC-404 (000107): own campaigns, approved upstream.
			PermCampaignsRead:  ScopeManaged,
			PermCampaignsWrite: ScopeManaged,
			// TEC-472 (000112).
			PermFleetsRead:   ScopeManaged,
			PermFleetsManage: ScopeManaged,
			PermFleetsPlan:   ScopeManaged,
			// TEC-466 (000113).
			PermShowcaseRead:  ScopeManaged,
			PermShowcaseWrite: ScopeManaged,
			// TEC-483 (000115).
			PermStockForecastRead:   ScopeManaged,
			PermStockForecastManage: ScopeManaged,
			// TEC-487 (000116).
			PermEfficiencyRead: ScopeManaged,
			// TEC-490 (000123).
			PermPerformanceRead:               ScopeManaged,
			PermPerformanceStaffTargetsManage: ScopeManaged,
			PermPerformanceBonusManage:        ScopeManaged,
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
			PermMeasurementsWrite:  ScopeManaged, // TEC-233 (000076)
			PermContractsRead:      ScopeManaged, // TEC-285 (000083)
			PermContractsWrite:     ScopeOwn,
			PermMeasurementsRead:   ScopeManaged, // TEC-293 (000085)
			PermMeasurementsLink:   ScopeManaged, // TEC-293 (000085)
			// TEC-329 (000086).
			PermAnnouncementsRead: ScopeManaged,
			PermLibraryRead:       ScopeManaged,
			// TEC-312 (000087).
			PermLeadsRead:   ScopeManaged,
			PermLeadsWrite:  ScopeManaged,
			PermQuotesRead:  ScopeManaged,
			PermQuotesWrite: ScopeManaged,
			// TEC-322 (000090).
			PermAppointmentsRead:  ScopeManaged,
			PermAppointmentsWrite: ScopeManaged,
			// TEC-350 (000091).
			PermReviewsRead: ScopeManaged,
			// TEC-334 (000092).
			PermWarrantyClaimsRead:  ScopeManaged,
			PermWarrantyClaimsWrite: ScopeManaged,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			// TEC-472 (000112): reads linked fleets and runs bulk plans and
			// intake; opening fleets is the owner's.
			PermFleetsRead: ScopeManaged,
			PermFleetsPlan: ScopeManaged,
			// TEC-466 (000113): reads the showcase, the owner edits it.
			PermShowcaseRead: ScopeManaged,
			// TEC-483 (000115).
			PermStockForecastRead: ScopeManaged,
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
			// TEC-305 (000084).
			PermServiceSubscriptionsRead: ScopeManaged,
			// TEC-329 (000086).
			PermAnnouncementsRead: ScopeManaged,
			PermLibraryRead:       ScopeManaged,
			// TEC-341 (000093).
			PermAccountingWrite:    ScopeManaged,
			PermDealerPricingWrite: ScopeManaged,
			PermProductSalesWrite:  ScopeManaged,
			PermSuppliersManage:    ScopeManaged,
			PermPurchasesWrite:     ScopeManaged,
			PermStaffManage:        ScopeManaged,
			PermStaffPaymentsWrite: ScopeManaged,
			// TEC-383 (000101).
			PermAIUse:            ScopeOwn,
			PermAIActionsConfirm: ScopeOwn,
			// TEC-490 (000123): bonus accruals feed staff payments.
			PermPerformanceBonusManage: ScopeManaged,
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
			// TEC-472 (000112): own fleet organization in the portal.
			PermFleetPortalRead: ScopeOwn,
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
