/**
 * Permission catalog — mirrors the backend catalog
 * (`backend/internal/platform/rbac/catalog.go`, TEC-85). Business modules that
 * have no routes yet (pricing, accounting, services, warehouse, campaigns…)
 * are listed so role packages and guards can reference them.
 */
export const Permission = {
  AuthSession: "auth.session",
  NotificationsRead: "notifications.read",
  NotificationsManage: "notifications.manage",

  PlatformUsersRead: "platform.users.read",
  PlatformUsersWrite: "platform.users.write",
  PlatformUsersExport: "platform.users.export",
  PlatformUsersImport: "platform.users.import",
  PlatformUsersBulkDisable: "platform.users.bulk.disable",
  PlatformUsersBulkEnable: "platform.users.bulk.enable",
  PlatformUsersImpersonate: "platform.users.impersonate",
  PlatformRolesRead: "platform.roles.read",
  PlatformRolesWrite: "platform.roles.write",
  PlatformRolesExport: "platform.roles.export",
  PlatformRolesImport: "platform.roles.import",
  PlatformRolesBulkDelete: "platform.roles.bulk.delete",
  PlatformBulkRead: "platform.bulk.read",
  PlatformNotificationsRead: "platform.notifications.read",
  PlatformNotificationsReadAll: "platform.notifications.read_all",
  PlatformNotificationsExport: "platform.notifications.export",
  PlatformSettingsRead: "platform.settings.read",
  PlatformSettingsWrite: "platform.settings.write",
  PlatformActivityRead: "platform.activity.read",
  PlatformImportsRead: "platform.imports.read",
  PlatformExportsRead: "platform.exports.read",
  PlatformStorageRead: "platform.storage.read",
  PlatformStorageWrite: "platform.storage.write",
  PlatformLogsRead: "platform.logs.read",
  PlatformLogsWrite: "platform.logs.write",
  PlatformAccessRead: "platform.access.read",
  PlatformAccessWrite: "platform.access.write",
  PlatformIntegrationsGitHubRead: "platform.integrations.github.read",
  PlatformIntegrationsGitHubWrite: "platform.integrations.github.write",
  PlatformIntegrationsGoogleRead: "platform.integrations.google.read",
  PlatformIntegrationsGoogleWrite: "platform.integrations.google.write",
  PlatformIntegrationsFacebookRead: "platform.integrations.facebook.read",
  PlatformIntegrationsFacebookWrite: "platform.integrations.facebook.write",
  PlatformIntegrationsAppleRead: "platform.integrations.apple.read",
  PlatformIntegrationsAppleWrite: "platform.integrations.apple.write",
  PlatformAuthSettingsRead: "platform.auth.settings.read",
  PlatformAuthSettingsWrite: "platform.auth.settings.write",
  PlatformOrganizationsRead: "platform.organizations.read",
  PlatformOrganizationsWrite: "platform.organizations.write",
  PlatformDocumentTemplatesRead: "platform.documents.templates.read",
  PlatformDocumentTemplatesWrite: "platform.documents.templates.write",
  PlatformGeoWrite: "platform.geo.write",
  PlatformTerritoriesRead: "platform.territories.read",
  PlatformTerritoriesWrite: "platform.territories.write",
  PlatformRatesRead: "platform.rates.read",
  PlatformRatesWrite: "platform.rates.write",

  TenantSettingsRead: "tenant.settings.read",
  TenantSettingsWrite: "tenant.settings.write",
  TenantImportsRead: "tenant.imports.read",
  TenantExportsRead: "tenant.exports.read",

  OrganizationsRead: "organizations.read",
  OrganizationsWrite: "organizations.write",
  OrganizationsSupplierWrite: "organizations.supplier.write",
  MembersRead: "members.read",
  MembersWrite: "members.write",

  ServicesRead: "services.read",
  ServicesWrite: "services.write",
  ServicesComplete: "services.complete",
  ServicesCancel: "services.cancel",
  CustomersRead: "customers.read",
  CustomersWrite: "customers.write",
  CustomersAnonymize: "customers.anonymize",
  CustomersMerge: "customers.merge",
  VehiclesRead: "vehicles.read",
  VehiclesWrite: "vehicles.write",
  VehiclesTransfer: "vehicles.transfer",
  WarrantiesRead: "warranties.read",
  WarrantiesVoid: "warranties.void",
  PricingPurchaseRead: "pricing.purchase.read",
  PricingSaleRead: "pricing.sale.read",
  PricingSaleWrite: "pricing.sale.write",
  PricingRecommendedRead: "pricing.recommended.read",
  PricingRecommendedWrite: "pricing.recommended.write",
  AccountingRead: "accounting.read",
  AccountingWrite: "accounting.write",
  AccountingDispute: "accounting.dispute",
  AccountingResolve: "accounting.resolve",
  /** TEC-349: staff cards (TEC-345) and salary / advance / bonus payments. */
  StaffManage: "staff.manage",
  StaffPaymentsWrite: "staff_payments.write",
  WarehouseRead: "warehouse.read",
  WarehouseWrite: "warehouse.write",
  CampaignsRead: "campaigns.read",
  CampaignsWrite: "campaigns.write",
  LeadsRead: "leads.read",
  LeadsWrite: "leads.write",
  LeadsConvertOrg: "leads.convert_org",
  QuotesRead: "quotes.read",
  QuotesWrite: "quotes.write",
  SocialRead: "social.read",
  SocialWrite: "social.write",
  PrivacyAnonymize: "privacy.anonymize",

  ModulesRead: "modules.read",
  ModulesManage: "modules.manage",
  PlatformModulesRead: "platform.modules.read",
  PlatformModulesWrite: "platform.modules.write",

  WhatsAppManage: "whatsapp.manage",
  IntegrationsGlorianView: "integrations.glorian.view",
  IntegrationsGlorianManage: "integrations.glorian.manage",

  NotificationTemplatesManage: "notifications.templates.manage",
  NotificationDeliveriesRead: "notification_deliveries.read",

  PlatformLegalTextsWrite: "platform.legal_texts.write",

  CatalogRead: "catalog.read",
  CatalogWrite: "catalog.write",

  VehicleCatalogRead: "vehicle_catalog.read",
  VehicleCatalogWrite: "vehicle_catalog.write",
  ServiceCatalogRead: "service_catalog.read",
  ServiceCatalogManage: "service_catalog.manage",
  ServiceSubscriptionsAssign: "service_subscriptions.assign",
  ServiceSubscriptionsRead: "service_subscriptions.read",
  ServiceSubscriptionsCancelRequest: "service_subscriptions.cancel_request",
  ServiceSubscriptionsCancelApprove: "service_subscriptions.cancel_approve",

  StockRead: "stock.read",
  StockWrite: "stock.write",
  StockAdjust: "stock.adjust",
  StockReclassify: "stock.reclassify",
  StockImport: "stock.import",

  OrdersRead: "orders.read",
  OrdersWrite: "orders.write",
  OrdersApprove: "orders.approve",
  OrdersShip: "orders.ship",
  OrdersReceive: "orders.receive",
  OrdersCancel: "orders.cancel",
  TransfersRequest: "transfers.request",
  TransfersApprove: "transfers.approve",
  TasksRead: "tasks.read",
  TasksWrite: "tasks.write",
  AnnouncementsRead: "announcements.read",
  AnnouncementsWrite: "announcements.write",
  LibraryRead: "library.read",
  LibraryManage: "library.manage",
  /** TEC-290: contract template editor (brand scoped, intake_contracts). */
  ContractsTemplatesManage: "contracts.templates.manage",
  /** TEC-291: intake contract signing in the service wizard. */
  ContractsRead: "contracts.read",
  ContractsWrite: "contracts.write",
  WarrantyClaimsRead: "warranty_claims.read",
  /** TEC-339: open a claim, forward to the center, decide. */
  WarrantyClaimsWrite: "warranty_claims.write",
  WarrantyClaimsReview: "warranty_claims.review",
  WarrantyClaimsDecide: "warranty_claims.decide",
  /** TEC-299: measurement pages and NexPTG devices. */
  MeasurementsRead: "measurements.read",
  MeasurementsLink: "measurements.link",
  MeasurementsWrite: "measurements.write",
  MeasurementDevicesManage: "measurement_devices.manage",
  /** TEC-326: appointment calendar and capacity settings. */
  AppointmentsRead: "appointments.read",
  AppointmentsWrite: "appointments.write",
  AppointmentSettingsManage: "appointment_settings.manage",
  /** TEC-348: dealer sale prices, quick sales, suppliers and purchases. */
  DealerPricingWrite: "dealer_pricing.write",
  ProductSalesWrite: "product_sales.write",
  SuppliersManage: "suppliers.manage",
  PurchasesWrite: "purchases.write",
  /** TEC-353: admin review questions and service review reports. */
  ReviewsQuestionsManage: "reviews.questions.manage",
  ReviewsRead: "reviews.read",
  /** TEC-383: AI assistant (chat, usage report, platform settings, confirmations). */
  AiUse: "ai.use",
  AiUsageRead: "ai.usage.read",
  AiSettingsManage: "ai.settings.manage",
  AiActionsConfirm: "ai.actions.confirm",
  /** TEC-393: WhatsApp conversations (platform admin only). */
  ConversationsRead: "conversations.read",
  ConversationsReply: "conversations.reply",
  ConversationsManage: "conversations.manage",
  /** TEC-403: MCP connections (consent) and the platform client list. */
  McpConnect: "mcp.connect",
  McpClientsManage: "mcp.clients.manage",
} as const;

export type PermissionSlug = (typeof Permission)[keyof typeof Permission];

export const ALL_PERMISSIONS: readonly PermissionSlug[] = Object.values(
  Permission,
).sort((a, b) => a.localeCompare(b));

const knownPermissionSet = new Set<string>(ALL_PERMISSIONS);

export function isKnownPermission(slug: string): slug is PermissionSlug {
  return knownPermissionSet.has(slug);
}

/**
 * Grant scopes, broadest internal scope first. `customer` (portal) only
 * covers itself.
 */
export const PERMISSION_SCOPES = [
  "all",
  "brand",
  "subtree",
  "managed",
  "assigned",
  "own",
  "customer",
] as const;

export type PermissionScope = (typeof PERMISSION_SCOPES)[number];

const SCOPE_RANK: Record<PermissionScope, number> = {
  all: 6,
  brand: 5,
  subtree: 4,
  managed: 3,
  assigned: 2,
  own: 1,
  customer: 0,
};

export function isPermissionScope(value: string): value is PermissionScope {
  return (PERMISSION_SCOPES as readonly string[]).includes(value);
}

/** Whether a grant with scope `have` satisfies a check needing `need`. */
export function scopeCovers(have: PermissionScope, need: PermissionScope) {
  if (have === "customer" || need === "customer") return have === need;
  return SCOPE_RANK[need] > 0 && SCOPE_RANK[have] >= SCOPE_RANK[need];
}

/** Broadest scope of a list (default grant scope for a permission). */
export function broadestScope(
  scopes: readonly string[],
): PermissionScope | undefined {
  let best: PermissionScope | undefined;
  for (const raw of scopes) {
    if (!isPermissionScope(raw)) continue;
    if (!best || SCOPE_RANK[raw] > SCOPE_RANK[best]) best = raw;
  }
  return best;
}

/** System role slugs (backend `rbac.Roles`). */
export const Role = {
  SuperAdmin: "super_admin",
  CenterStaff: "center_staff",
  CenterWarehouse: "center_warehouse",
  CenterAccounting: "center_accounting",
  CenterSocial: "center_social",
  DistributorOwner: "distributor_owner",
  DistributorStaff: "distributor_staff",
  DistributorWarehouseStaff: "distributor_warehouse_staff",
  DistributorAccounting: "distributor_accounting",
  DealerOwner: "dealer_owner",
  DealerStaff: "dealer_staff",
  DealerAccounting: "dealer_accounting",
  Customer: "customer",
  Fleet: "fleet",
} as const;

export const permissions = {
  users: {
    read: Permission.PlatformUsersRead,
    write: Permission.PlatformUsersWrite,
    export: Permission.PlatformUsersExport,
    import: Permission.PlatformUsersImport,
    bulkDisable: Permission.PlatformUsersBulkDisable,
    bulkEnable: Permission.PlatformUsersBulkEnable,
    impersonate: Permission.PlatformUsersImpersonate,
  },
  roles: {
    read: Permission.PlatformRolesRead,
    write: Permission.PlatformRolesWrite,
    export: Permission.PlatformRolesExport,
    import: Permission.PlatformRolesImport,
    bulkDelete: Permission.PlatformRolesBulkDelete,
  },
  notifications: {
    read: Permission.NotificationsRead,
    manage: Permission.NotificationsManage,
    platformRead: Permission.PlatformNotificationsRead,
    platformReadAll: Permission.PlatformNotificationsReadAll,
    export: Permission.PlatformNotificationsExport,
    templatesManage: Permission.NotificationTemplatesManage,
    deliveriesRead: Permission.NotificationDeliveriesRead,
  },
  settings: {
    read: Permission.PlatformSettingsRead,
    write: Permission.PlatformSettingsWrite,
    tenantRead: Permission.TenantSettingsRead,
    tenantWrite: Permission.TenantSettingsWrite,
  },
  activity: {
    read: Permission.PlatformActivityRead,
  },
  imports: {
    read: Permission.PlatformImportsRead,
    tenantRead: Permission.TenantImportsRead,
  },
  exports: {
    read: Permission.PlatformExportsRead,
    tenantRead: Permission.TenantExportsRead,
  },
  storage: {
    read: Permission.PlatformStorageRead,
    write: Permission.PlatformStorageWrite,
  },
  logs: {
    read: Permission.PlatformLogsRead,
    write: Permission.PlatformLogsWrite,
  },
  bulk: {
    read: Permission.PlatformBulkRead,
  },
  access: {
    read: Permission.PlatformAccessRead,
    write: Permission.PlatformAccessWrite,
  },
  integrations: {
    github: {
      read: Permission.PlatformIntegrationsGitHubRead,
      write: Permission.PlatformIntegrationsGitHubWrite,
    },
    google: {
      read: Permission.PlatformIntegrationsGoogleRead,
      write: Permission.PlatformIntegrationsGoogleWrite,
    },
    facebook: {
      read: Permission.PlatformIntegrationsFacebookRead,
      write: Permission.PlatformIntegrationsFacebookWrite,
    },
    apple: {
      read: Permission.PlatformIntegrationsAppleRead,
      write: Permission.PlatformIntegrationsAppleWrite,
    },
    whatsapp: {
      manage: Permission.WhatsAppManage,
    },
    glorian: {
      view: Permission.IntegrationsGlorianView,
      manage: Permission.IntegrationsGlorianManage,
    },
  },
  legalTexts: {
    write: Permission.PlatformLegalTextsWrite,
  },
  authSettings: {
    read: Permission.PlatformAuthSettingsRead,
    write: Permission.PlatformAuthSettingsWrite,
  },
  organizations: {
    read: Permission.PlatformOrganizationsRead,
    write: Permission.PlatformOrganizationsWrite,
    tenantRead: Permission.OrganizationsRead,
    tenantWrite: Permission.OrganizationsWrite,
    supplierWrite: Permission.OrganizationsSupplierWrite,
  },
  documentTemplates: {
    read: Permission.PlatformDocumentTemplatesRead,
    write: Permission.PlatformDocumentTemplatesWrite,
  },
  members: {
    read: Permission.MembersRead,
    write: Permission.MembersWrite,
  },
  services: {
    read: Permission.ServicesRead,
    write: Permission.ServicesWrite,
    complete: Permission.ServicesComplete,
    cancel: Permission.ServicesCancel,
  },
  customers: {
    read: Permission.CustomersRead,
    write: Permission.CustomersWrite,
    anonymize: Permission.CustomersAnonymize,
    merge: Permission.CustomersMerge,
  },
  vehicles: {
    read: Permission.VehiclesRead,
    write: Permission.VehiclesWrite,
    transfer: Permission.VehiclesTransfer,
  },
  warranties: {
    read: Permission.WarrantiesRead,
    void: Permission.WarrantiesVoid,
  },
  orders: {
    read: Permission.OrdersRead,
    write: Permission.OrdersWrite,
    approve: Permission.OrdersApprove,
    ship: Permission.OrdersShip,
    receive: Permission.OrdersReceive,
    cancel: Permission.OrdersCancel,
  },
  transfers: {
    request: Permission.TransfersRequest,
    approve: Permission.TransfersApprove,
  },
  tasks: {
    read: Permission.TasksRead,
    write: Permission.TasksWrite,
  },
  announcements: {
    read: Permission.AnnouncementsRead,
    write: Permission.AnnouncementsWrite,
  },
  library: {
    read: Permission.LibraryRead,
    manage: Permission.LibraryManage,
  },
  catalog: {
    read: Permission.CatalogRead,
    write: Permission.CatalogWrite,
  },
  pricing: {
    purchaseRead: Permission.PricingPurchaseRead,
    saleRead: Permission.PricingSaleRead,
    saleWrite: Permission.PricingSaleWrite,
    recommendedRead: Permission.PricingRecommendedRead,
    recommendedWrite: Permission.PricingRecommendedWrite,
  },
  accounting: {
    read: Permission.AccountingRead,
    write: Permission.AccountingWrite,
    dispute: Permission.AccountingDispute,
    resolve: Permission.AccountingResolve,
  },
  staff: {
    manage: Permission.StaffManage,
    paymentsWrite: Permission.StaffPaymentsWrite,
  },
  warehouse: {
    read: Permission.WarehouseRead,
    write: Permission.WarehouseWrite,
  },
  campaigns: {
    read: Permission.CampaignsRead,
    write: Permission.CampaignsWrite,
  },
  leads: {
    read: Permission.LeadsRead,
    write: Permission.LeadsWrite,
    convertOrg: Permission.LeadsConvertOrg,
  },
  quotes: {
    read: Permission.QuotesRead,
    write: Permission.QuotesWrite,
  },
  social: {
    read: Permission.SocialRead,
    write: Permission.SocialWrite,
  },
  privacy: {
    anonymize: Permission.PrivacyAnonymize,
  },
  modules: {
    read: Permission.ModulesRead,
    manage: Permission.ModulesManage,
    platformRead: Permission.PlatformModulesRead,
    platformWrite: Permission.PlatformModulesWrite,
  },
  auth: {
    session: Permission.AuthSession,
  },
  geo: {
    write: Permission.PlatformGeoWrite,
  },
  territories: {
    read: Permission.PlatformTerritoriesRead,
    write: Permission.PlatformTerritoriesWrite,
  },
  rates: {
    read: Permission.PlatformRatesRead,
    write: Permission.PlatformRatesWrite,
  },
  vehicleCatalog: {
    read: Permission.VehicleCatalogRead,
    write: Permission.VehicleCatalogWrite,
  },
  serviceCatalog: {
    read: Permission.ServiceCatalogRead,
    manage: Permission.ServiceCatalogManage,
  },
  serviceSubscriptions: {
    assign: Permission.ServiceSubscriptionsAssign,
    read: Permission.ServiceSubscriptionsRead,
    cancelRequest: Permission.ServiceSubscriptionsCancelRequest,
    cancelApprove: Permission.ServiceSubscriptionsCancelApprove,
  },
  stock: {
    read: Permission.StockRead,
    write: Permission.StockWrite,
  },
  contractTemplates: {
    manage: Permission.ContractsTemplatesManage,
  },
  contracts: {
    read: Permission.ContractsRead,
    write: Permission.ContractsWrite,
  },
  warrantyClaims: {
    read: Permission.WarrantyClaimsRead,
    write: Permission.WarrantyClaimsWrite,
    review: Permission.WarrantyClaimsReview,
    decide: Permission.WarrantyClaimsDecide,
  },
  measurements: {
    read: Permission.MeasurementsRead,
    link: Permission.MeasurementsLink,
    write: Permission.MeasurementsWrite,
    devicesManage: Permission.MeasurementDevicesManage,
  },
  appointments: {
    read: Permission.AppointmentsRead,
    write: Permission.AppointmentsWrite,
    settingsManage: Permission.AppointmentSettingsManage,
  },
  dealerSales: {
    pricingWrite: Permission.DealerPricingWrite,
    salesWrite: Permission.ProductSalesWrite,
    suppliersManage: Permission.SuppliersManage,
    purchasesWrite: Permission.PurchasesWrite,
  },
  reviews: {
    questionsManage: Permission.ReviewsQuestionsManage,
    read: Permission.ReviewsRead,
  },
  ai: {
    use: Permission.AiUse,
    actionsConfirm: Permission.AiActionsConfirm,
  },
  /** TEC-399: WhatsApp inbox (platform admin only, S2). */
  conversations: {
    read: Permission.ConversationsRead,
    reply: Permission.ConversationsReply,
    manage: Permission.ConversationsManage,
  },
  /** TEC-403: MCP connect (consent) and the platform client list. */
  mcp: {
    connect: Permission.McpConnect,
    clientsManage: Permission.McpClientsManage,
  },
  /** TEC-391: AI administration (platform settings, quotas, usage report). */
  aiAdmin: {
    settingsManage: Permission.AiSettingsManage,
    usageRead: Permission.AiUsageRead,
  },
} as const;
