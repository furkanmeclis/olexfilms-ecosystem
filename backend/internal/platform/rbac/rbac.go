package rbac

// Role slugs (seeded; see catalog.go for the packages).
const (
	RoleSuperAdmin = "super_admin"

	RoleCenterStaff      = "center_staff"
	RoleCenterWarehouse  = "center_warehouse"
	RoleCenterAccounting = "center_accounting"
	RoleCenterSocial     = "center_social"

	RoleDistributorOwner          = "distributor_owner"
	RoleDistributorStaff          = "distributor_staff"
	RoleDistributorWarehouseStaff = "distributor_warehouse_staff"
	RoleDistributorAccounting     = "distributor_accounting"

	RoleDealerOwner      = "dealer_owner"
	RoleDealerStaff      = "dealer_staff"
	RoleDealerAccounting = "dealer_accounting"

	RoleCustomer = "customer"
	RoleFleet    = "fleet"
)

// Role org types: which kind of subject a role is granted to. Platform,
// customer and fleet roles are global (user_roles); center, distributor and
// dealer roles are organization roles (organization_member_roles).
const (
	OrgTypePlatform    = "platform"
	OrgTypeCenter      = "center"
	OrgTypeDistributor = "distributor"
	OrgTypeDealer      = "dealer"
	OrgTypeCustomer    = "customer"
	OrgTypeFleet       = "fleet"
)

// Permission slugs (seeded).
const (
	PermPlatformUsersRead                 = "platform.users.read"
	PermPlatformUsersWrite                = "platform.users.write"
	PermPlatformUsersExport               = "platform.users.export"
	PermPlatformUsersImport               = "platform.users.import"
	PermPlatformUsersBulkDisable          = "platform.users.bulk.disable"
	PermPlatformUsersBulkEnable           = "platform.users.bulk.enable"
	PermPlatformUsersImpersonate          = "platform.users.impersonate"
	PermPlatformRolesRead                 = "platform.roles.read"
	PermPlatformRolesWrite                = "platform.roles.write"
	PermPlatformRolesExport               = "platform.roles.export"
	PermPlatformRolesImport               = "platform.roles.import"
	PermPlatformRolesBulkDelete           = "platform.roles.bulk.delete"
	PermPlatformBulkRead                  = "platform.bulk.read"
	PermPlatformNotificationsRead         = "platform.notifications.read"
	PermPlatformNotificationsReadAll      = "platform.notifications.read_all"
	PermPlatformNotificationsExport       = "platform.notifications.export"
	PermPlatformSettingsRead              = "platform.settings.read"
	PermPlatformSettingsWrite             = "platform.settings.write"
	PermPlatformActivityRead              = "platform.activity.read"
	PermPlatformImportsRead               = "platform.imports.read"
	PermPlatformExportsRead               = "platform.exports.read"
	PermPlatformStorageRead               = "platform.storage.read"
	PermPlatformStorageWrite              = "platform.storage.write"
	PermPlatformLogsRead                  = "platform.logs.read"
	PermPlatformLogsWrite                 = "platform.logs.write"
	PermPlatformAccessRead                = "platform.access.read"
	PermPlatformAccessWrite               = "platform.access.write"
	PermPlatformIntegrationsGitHubRead    = "platform.integrations.github.read"
	PermPlatformIntegrationsGitHubWrite   = "platform.integrations.github.write"
	PermPlatformIntegrationsGoogleRead    = "platform.integrations.google.read"
	PermPlatformIntegrationsGoogleWrite   = "platform.integrations.google.write"
	PermPlatformIntegrationsFacebookRead  = "platform.integrations.facebook.read"
	PermPlatformIntegrationsFacebookWrite = "platform.integrations.facebook.write"
	PermPlatformIntegrationsAppleRead     = "platform.integrations.apple.read"
	PermPlatformIntegrationsAppleWrite    = "platform.integrations.apple.write"
	PermPlatformAuthSettingsRead          = "platform.auth.settings.read"
	PermPlatformAuthSettingsWrite         = "platform.auth.settings.write"
	PermPlatformOrganizationsRead         = "platform.organizations.read"
	PermPlatformOrganizationsWrite        = "platform.organizations.write"
	PermPlatformDocumentTemplatesRead     = "platform.documents.templates.read"
	PermPlatformDocumentTemplatesWrite    = "platform.documents.templates.write"
	PermAuthSession                       = "auth.session"
	PermNotificationsRead                 = "notifications.read"
	PermNotificationsManage               = "notifications.manage"
	PermTenantSettingsRead                = "tenant.settings.read"
	PermTenantSettingsWrite               = "tenant.settings.write"
	PermTenantImportsRead                 = "tenant.imports.read"
	PermTenantExportsRead                 = "tenant.exports.read"

	// Organization tree and members.
	PermOrganizationsRead          = "organizations.read"
	PermOrganizationsWrite         = "organizations.write"
	PermOrganizationsSupplierWrite = "organizations.supplier.write"
	PermMembersRead                = "members.read"
	PermMembersWrite               = "members.write"

	// Business modules (routes arrive with their modules).
	PermServicesRead            = "services.read"
	PermServicesWrite           = "services.write"
	PermCustomersRead           = "customers.read"
	PermCustomersWrite          = "customers.write"
	PermPricingPurchaseRead     = "pricing.purchase.read"
	PermPricingSaleRead         = "pricing.sale.read"
	PermPricingSaleWrite        = "pricing.sale.write"
	PermPricingRecommendedRead  = "pricing.recommended.read"
	PermPricingRecommendedWrite = "pricing.recommended.write"
	PermAccountingRead          = "accounting.read"
	PermAccountingWrite         = "accounting.write"
	PermWarehouseRead           = "warehouse.read"
	PermWarehouseWrite          = "warehouse.write"
	PermCampaignsRead           = "campaigns.read"
	PermCampaignsWrite          = "campaigns.write"
	PermLeadsRead               = "leads.read"
	PermLeadsWrite              = "leads.write"
	PermSocialRead              = "social.read"
	PermSocialWrite             = "social.write"
	PermPrivacyAnonymize        = "privacy.anonymize"

	// Module packages (TEC-86).
	PermModulesRead          = "modules.read"
	PermModulesManage        = "modules.manage"
	PermPlatformModulesRead  = "platform.modules.read"
	PermPlatformModulesWrite = "platform.modules.write"

	PermWhatsAppManage = "whatsapp.manage"

	// Notification center (TEC-87).
	PermNotificationTemplatesManage = "notifications.templates.manage"
	PermNotificationDeliveriesRead  = "notification_deliveries.read"

	// Geography, territories and exchange rates (TEC-84).
	PermPlatformGeoWrite         = "platform.geo.write"
	PermPlatformTerritoriesRead  = "platform.territories.read"
	PermPlatformTerritoriesWrite = "platform.territories.write"
	PermPlatformRatesRead        = "platform.rates.read"
	PermPlatformRatesWrite       = "platform.rates.write"

	// Portal legal texts (TEC-90): AI guidelines Markdown, per language.
	PermPlatformLegalTextsWrite = "platform.legal_texts.write"

	// Product catalog (TEC-144): written by the center only (K4).
	PermCatalogRead  = "catalog.read"
	PermCatalogWrite = "catalog.write"

	// Vehicle catalog (TEC-149): global car brands/models; super_admin writes.
	PermVehicleCatalogRead  = "vehicle_catalog.read"
	PermVehicleCatalogWrite = "vehicle_catalog.write"

	// Stock ledger (TEC-153): units, movements and projections. The
	// warehouse is brand-independent (K20); barcodes are center-only (K14).
	PermStockRead       = "stock.read"
	PermStockWrite      = "stock.write"
	PermStockAdjust     = "stock.adjust"
	PermStockReclassify = "stock.reclassify"
	PermStockImport     = "stock.import"
)

// IsSystemRole reports whether slug is a protected system role.
func IsSystemRole(slug string) bool {
	_, ok := RoleBySlug(slug)
	return ok
}

// SuperAdminOnly reports whether a permission may only be granted to
// super_admin (impersonation).
func SuperAdminOnly(slug string) bool {
	p, ok := PermissionBySlug(slug)
	return ok && p.SuperAdminOnly
}

// IsOrganizationRoleType reports whether roles of this org type are granted
// through organization membership rather than globally.
func IsOrganizationRoleType(orgType string) bool {
	switch orgType {
	case OrgTypeCenter, OrgTypeDistributor, OrgTypeDealer:
		return true
	default:
		return false
	}
}

// DefaultMemberRole maps an organization type and membership kind
// (owner|staff) to the default organization role slug.
func DefaultMemberRole(orgType, memberRole string) string {
	switch orgType {
	case OrgTypeCenter:
		return RoleCenterStaff
	case OrgTypeDistributor:
		if memberRole == "owner" {
			return RoleDistributorOwner
		}
		return RoleDistributorStaff
	default:
		if memberRole == "owner" {
			return RoleDealerOwner
		}
		return RoleDealerStaff
	}
}
