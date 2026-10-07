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
	PermServicesRead              = "services.read"
	PermServicesWrite             = "services.write"
	PermCustomersRead             = "customers.read"
	PermCustomersWrite            = "customers.write"
	PermPricingPurchaseRead       = "pricing.purchase.read"
	PermPricingSaleRead           = "pricing.sale.read"
	PermPricingSaleWrite          = "pricing.sale.write"
	PermPricingRecommendedRead    = "pricing.recommended.read"
	PermPricingRecommendedWrite   = "pricing.recommended.write"
	PermAccountingRead            = "accounting.read"
	PermAccountingWrite           = "accounting.write"
	PermWarehouseRead             = "warehouse.read"
	PermWarehouseWrite            = "warehouse.write"
	PermCampaignsRead             = "campaigns.read"
	PermCampaignsWrite            = "campaigns.write"
	PermLeadsRead                 = "leads.read"
	PermLeadsWrite                = "leads.write"
	PermLeadsConvertOrg           = "leads.convert_org"
	PermQuotesRead                = "quotes.read"
	PermQuotesWrite               = "quotes.write"
	PermAppointmentsRead          = "appointments.read"
	PermAppointmentsWrite         = "appointments.write"
	PermAppointmentSettingsManage = "appointment_settings.manage"
	PermSocialRead                = "social.read"
	PermSocialWrite               = "social.write"
	PermPrivacyAnonymize          = "privacy.anonymize"

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

	// Accounting disputes (TEC-171, K24): a lower level never writes its
	// parent's ledger; it opens a dispute on an entry the parent posted.
	PermAccountingDispute = "accounting.dispute"

	// Customers and vehicles (TEC-159, K11/K19). Anonymize and merge are
	// center-only (brand scope) and super_admin.
	PermVehiclesRead       = "vehicles.read"
	PermVehiclesWrite      = "vehicles.write"
	PermCustomersAnonymize = "customers.anonymize"
	PermCustomersMerge     = "customers.merge"

	// Orders and sibling transfers (TEC-165, K6/K7/K13). The seller approves
	// and ships, the buyer receives; transfers are requested by the giving
	// dealer and approved by the common distributor.
	PermOrdersRead       = "orders.read"
	PermOrdersWrite      = "orders.write"
	PermOrdersApprove    = "orders.approve"
	PermOrdersShip       = "orders.ship"
	PermOrdersReceive    = "orders.receive"
	PermOrdersCancel     = "orders.cancel"
	PermTransfersRequest = "transfers.request"
	PermTransfersApprove = "transfers.approve"

	// TEC-178 (000050): service completion and center-only cancel.
	PermServicesComplete        = "services.complete"
	PermServicesCancel          = "services.cancel"
	PermServicesCancelCompleted = "services.cancel_completed"

	// TEC-350 (000091): extra review questions and review reading.
	PermReviewsQuestionsManage = "reviews.questions.manage"
	PermReviewsRead            = "reviews.read"

	// TEC-185 (000051): warranties (void is center-only) and vehicle
	// ownership transfer with two codes (TEC-98 decision 6).
	PermWarrantiesRead   = "warranties.read"
	PermWarrantiesVoid   = "warranties.void"
	PermVehiclesTransfer = "vehicles.transfer"

	// TEC-334 (000092): warranty claims. Dealers and distributors open
	// them, the distributor reviews the subtree, the center decides.
	PermWarrantyClaimsRead   = "warranty_claims.read"
	PermWarrantyClaimsWrite  = "warranty_claims.write"
	PermWarrantyClaimsReview = "warranty_claims.review"
	PermWarrantyClaimsDecide = "warranty_claims.decide"

	// TEC-174 (000052): the parent resolves a dispute opened with
	// accounting.dispute (reversal, revision or rejection, K24).
	PermAccountingResolve = "accounting.resolve"

	// TEC-214 (000058): center tasks about distributors and dealers; held
	// by center roles only (brand scope) and super_admin.
	PermTasksRead  = "tasks.read"
	PermTasksWrite = "tasks.write"

	// TEC-266 (000075): Glorian hub connection and sync state (K2). Held by
	// super_admin only until the Glorian settings screen lands.
	PermIntegrationsGlorianView   = "integrations.glorian.view"
	PermIntegrationsGlorianManage = "integrations.glorian.manage"

	// TEC-233 (000076): minimal measurement upload from the mobile app
	// (K28); F3-02 (TEC-113) adds the read side.
	PermMeasurementsWrite = "measurements.write"

	// TEC-285 (000083): vehicle intake / service sale contracts (F3-01).
	// Templates and void are center-only; read and write follow the roles
	// that open services.
	PermContractsTemplatesManage = "contracts.templates.manage"
	PermContractsRead            = "contracts.read"
	PermContractsWrite           = "contracts.write"
	PermContractsVoid            = "contracts.void"

	// TEC-305 (000084): non-product service catalog (center only) and
	// service subscriptions; distributors assign to their subtree without a
	// margin, dealers and distributors request early cancellation, the
	// center approves.
	PermServiceCatalogManage              = "service_catalog.manage"
	PermServiceCatalogRead                = "service_catalog.read"
	PermServiceSubscriptionsAssign        = "service_subscriptions.assign"
	PermServiceSubscriptionsRead          = "service_subscriptions.read"
	PermServiceSubscriptionsCancelRequest = "service_subscriptions.cancel_request"
	PermServiceSubscriptionsCancelApprove = "service_subscriptions.cancel_approve"
	// TEC-293 (000085): measurement read side, before/after service links
	// and the measuring device registry (K28).
	PermMeasurementsRead         = "measurements.read"
	PermMeasurementsLink         = "measurements.link"
	PermMeasurementDevicesManage = "measurement_devices.manage"
	// TEC-329 (000086): announcements and the document library (F3-05).
	PermAnnouncementsRead  = "announcements.read"
	PermAnnouncementsWrite = "announcements.write"
	PermLibraryRead        = "library.read"
	PermLibraryManage      = "library.manage"
	// TEC-341 (000093): full dealer accounting (F3-07). Dealer owner and
	// dealer accounting only; dealer_staff holds none of them.
	PermDealerPricingWrite = "dealer_pricing.write"
	PermProductSalesWrite  = "product_sales.write"
	PermSuppliersManage    = "suppliers.manage"
	PermPurchasesWrite     = "purchases.write"
	PermStaffManage        = "staff.manage"
	PermStaffPaymentsWrite = "staff_payments.write"
	// TEC-479 (000111): certificates addon (F5-03). Center defines and
	// approves; distributor verifies subtree; dealer/distributor owners
	// upload for their staff.
	PermCertificateTypesManage     = "certificate_types.manage"
	PermCertificatesRead           = "certificates.read"
	PermCertificatesWrite          = "certificates.write"
	PermCertificatesVerify         = "certificates.verify"
	PermCertificatesApproveService = "certificates.approve_service"
	// TEC-383 (000101): AI assistant (F4-01). ai.use and ai.actions.confirm
	// are granted together to every panel role; customers are gated by realm,
	// not by a permission.
	PermAIUse            = "ai.use"
	PermAIUsageRead      = "ai.usage.read"
	PermAISettingsManage = "ai.settings.manage"
	PermAIActionsConfirm = "ai.actions.confirm"
	// TEC-393 (000103): WhatsApp conversations (F4-02). Only the platform
	// admin sees WhatsApp conversations (F4 user answer S2), so all three are
	// super_admin only.
	PermConversationsRead   = "conversations.read"
	PermConversationsReply  = "conversations.reply"
	PermConversationsManage = "conversations.manage"

	// TEC-400 (000104): MCP OAuth (F4-03). mcp.connect goes to the owner and
	// center roles; the customer MCP endpoint is gated by realm, not by a
	// permission.
	PermMCPConnect       = "mcp.connect"
	PermMCPClientsManage = "mcp.clients.manage"

	// TEC-404 (000107): campaign approval (F4-04). The center approves
	// distributor and distributor-less dealer campaigns, a distributor those
	// of its dealers (F4 user answer S4).
	PermCampaignsApprove = "campaigns.approve"

	// TEC-472 (000112): fleets (F5-02). Dealers and their distributor read
	// and manage the fleets linked to them, the center its brand; dealer
	// owner and staff plan bulk services. The fleet role reads its own
	// fleet in the portal.
	PermFleetsRead      = "fleets.read"
	PermFleetsManage    = "fleets.manage"
	PermFleetsPlan      = "fleets.plan"
	PermFleetPortalRead = "fleet.portal.read"
	// TEC-466 (000113): dealer showcase (F5-01). The dealer owner edits its
	// own showcase, the distributor owner those of its subtree; the center
	// reviews them when showcase.approval_required is on.
	PermShowcaseRead           = "showcase.read"
	PermShowcaseWrite          = "showcase.write"
	PermPlatformShowcaseReview = "platform.showcase.review"
	// TEC-487 (000115): efficiency and waste analytics (F5-06a). Dealers
	// read their own data, distributors read subtree, center manages
	// expected consumption definitions.
	PermEfficiencyRead               = "efficiency.read"
	PermEfficiencyExpectationsManage = "efficiency.expectations.manage"
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
