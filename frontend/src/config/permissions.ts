/**
 * Permission catalog aligned with platform RBAC seed permissions.
 */
export const Permission = {
  PlatformUsersRead: "platform.users.read",
  PlatformUsersWrite: "platform.users.write",
  PlatformUsersExport: "platform.users.export",
  PlatformUsersImport: "platform.users.import",
  PlatformRolesRead: "platform.roles.read",
  PlatformRolesWrite: "platform.roles.write",
  PlatformRolesExport: "platform.roles.export",
  PlatformRolesImport: "platform.roles.import",
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
  PlatformUsersBulkDisable: "platform.users.bulk.disable",
  PlatformUsersBulkEnable: "platform.users.bulk.enable",
  PlatformUsersImpersonate: "platform.users.impersonate",
  PlatformRolesBulkDelete: "platform.roles.bulk.delete",
  PlatformBulkRead: "platform.bulk.read",
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
  PlatformVehicleBrandsRead: "platform.vehicle_brands.read",
  PlatformVehicleBrandsWrite: "platform.vehicle_brands.write",
  PlatformContractPresetsRead: "platform.contract_presets.read",
  PlatformContractPresetsWrite: "platform.contract_presets.write",
  PlatformAIRead: "platform.ai.read",
  PlatformAIWrite: "platform.ai.write",

  TenantFinanceRead: "tenant.finance.read",
  TenantFinanceWrite: "tenant.finance.write",
  TenantFinanceExport: "tenant.finance.export",
  TenantFinanceImport: "tenant.finance.import",
  TenantSettingsRead: "tenant.settings.read",
  TenantSettingsWrite: "tenant.settings.write",
  TenantImportsRead: "tenant.imports.read",

  TenantCustomersRead: "tenant.customers.read",
  TenantCustomersWrite: "tenant.customers.write",

  TenantCariRead: "tenant.cari.read",
  TenantCariWrite: "tenant.cari.write",
  TenantCariExport: "tenant.cari.export",
  TenantJobsRead: "tenant.jobs.read",
  TenantJobsWrite: "tenant.jobs.write",
  TenantJobsExport: "tenant.jobs.export",
  TenantStaffRead: "tenant.staff.read",
  TenantStaffWrite: "tenant.staff.write",
  TenantSalesRead: "tenant.sales.read",
  TenantSalesWrite: "tenant.sales.write",
  TenantSalesExport: "tenant.sales.export",
  TenantSuppliersRead: "tenant.suppliers.read",
  TenantSuppliersWrite: "tenant.suppliers.write",
  TenantSuppliersExport: "tenant.suppliers.export",
  TenantPurchasesRead: "tenant.purchases.read",
  TenantPurchasesWrite: "tenant.purchases.write",
  TenantPurchasesExport: "tenant.purchases.export",
  TenantReportsRead: "tenant.reports.read",
  TenantReportsExport: "tenant.reports.export",
  TenantContractsRead: "tenant.contracts.read",
  TenantContractsWrite: "tenant.contracts.write",
  TenantAIUse: "tenant.ai.use",
  TenantTodosRead: "tenant.todos.read",
  TenantTodosWrite: "tenant.todos.write",
  TenantMessagingRead: "tenant.messaging.read",
  TenantMessagingWrite: "tenant.messaging.write",
  TenantLeadsRead: "tenant.leads.read",
  TenantLeadsWrite: "tenant.leads.write",
  TenantQuotesRead: "tenant.quotes.read",
  TenantQuotesWrite: "tenant.quotes.write",
  TenantBillingRead: "tenant.billing.read",
  TenantBillingWrite: "tenant.billing.write",
  PlatformBillingRead: "platform.billing.read",
  PlatformBillingWrite: "platform.billing.write",
  PlatformBillingSettings: "platform.billing.settings",

  TenantCatalogRead: "tenant.catalog.read",
  TenantCatalogWrite: "tenant.catalog.write",
  TenantCatalogExport: "tenant.catalog.export",
  TenantCatalogImport: "tenant.catalog.import",
  TenantCatalogProductsBulkActivate: "tenant.catalog.products.bulk.activate",
  TenantCatalogProductsBulkDeactivate:
    "tenant.catalog.products.bulk.deactivate",
  TenantCatalogProductsBulkDelete: "tenant.catalog.products.bulk.delete",
  TenantCatalogProductsBulkRaiseSalePrice:
    "tenant.catalog.products.bulk.raise_sale_price",
  TenantCatalogProductsBulkRaiseCostPrice:
    "tenant.catalog.products.bulk.raise_cost_price",
  TenantCatalogProductsBulkAdjustStock:
    "tenant.catalog.products.bulk.adjust_stock",
  TenantCatalogServicesBulkActivate: "tenant.catalog.services.bulk.activate",
  TenantCatalogServicesBulkDeactivate:
    "tenant.catalog.services.bulk.deactivate",
  TenantCatalogServicesBulkDelete: "tenant.catalog.services.bulk.delete",
  TenantCatalogServicesBulkRaisePrice:
    "tenant.catalog.services.bulk.raise_price",

  AuthSession: "auth.session",

  NotificationsRead: "notifications.read",
  NotificationsManage: "notifications.manage",
} as const;

export type PermissionSlug = (typeof Permission)[keyof typeof Permission];

export const ALL_PERMISSIONS: readonly PermissionSlug[] = Object.values(
  Permission,
).sort((a, b) => a.localeCompare(b));

const knownPermissionSet = new Set<string>(ALL_PERMISSIONS);

export function isKnownPermission(slug: string): slug is PermissionSlug {
  return knownPermissionSet.has(slug);
}

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
  },
  authSettings: {
    read: Permission.PlatformAuthSettingsRead,
    write: Permission.PlatformAuthSettingsWrite,
  },
  organizations: {
    read: Permission.PlatformOrganizationsRead,
    write: Permission.PlatformOrganizationsWrite,
  },
  vehicleBrands: {
    read: Permission.PlatformVehicleBrandsRead,
    write: Permission.PlatformVehicleBrandsWrite,
  },
  contractPresets: {
    read: Permission.PlatformContractPresetsRead,
    write: Permission.PlatformContractPresetsWrite,
  },
  customers: {
    read: Permission.TenantCustomersRead,
    write: Permission.TenantCustomersWrite,
  },
  cari: {
    read: Permission.TenantCariRead,
    write: Permission.TenantCariWrite,
    export: Permission.TenantCariExport,
  },
  jobs: {
    read: Permission.TenantJobsRead,
    write: Permission.TenantJobsWrite,
    export: Permission.TenantJobsExport,
  },
  staff: {
    read: Permission.TenantStaffRead,
    write: Permission.TenantStaffWrite,
  },
  sales: {
    read: Permission.TenantSalesRead,
    write: Permission.TenantSalesWrite,
    export: Permission.TenantSalesExport,
  },
  suppliers: {
    read: Permission.TenantSuppliersRead,
    write: Permission.TenantSuppliersWrite,
    export: Permission.TenantSuppliersExport,
  },
  purchases: {
    read: Permission.TenantPurchasesRead,
    write: Permission.TenantPurchasesWrite,
    export: Permission.TenantPurchasesExport,
  },
  reports: {
    read: Permission.TenantReportsRead,
    export: Permission.TenantReportsExport,
  },
  contracts: {
    read: Permission.TenantContractsRead,
    write: Permission.TenantContractsWrite,
  },
  finance: {
    read: Permission.TenantFinanceRead,
    write: Permission.TenantFinanceWrite,
    export: Permission.TenantFinanceExport,
    import: Permission.TenantFinanceImport,
  },
  catalog: {
    read: Permission.TenantCatalogRead,
    write: Permission.TenantCatalogWrite,
    export: Permission.TenantCatalogExport,
    import: Permission.TenantCatalogImport,
    productsBulkActivate: Permission.TenantCatalogProductsBulkActivate,
    productsBulkDeactivate: Permission.TenantCatalogProductsBulkDeactivate,
    productsBulkDelete: Permission.TenantCatalogProductsBulkDelete,
    productsBulkRaiseSalePrice:
      Permission.TenantCatalogProductsBulkRaiseSalePrice,
    productsBulkRaiseCostPrice:
      Permission.TenantCatalogProductsBulkRaiseCostPrice,
    productsBulkAdjustStock: Permission.TenantCatalogProductsBulkAdjustStock,
    servicesBulkActivate: Permission.TenantCatalogServicesBulkActivate,
    servicesBulkDeactivate: Permission.TenantCatalogServicesBulkDeactivate,
    servicesBulkDelete: Permission.TenantCatalogServicesBulkDelete,
    servicesBulkRaisePrice: Permission.TenantCatalogServicesBulkRaisePrice,
  },
  ai: {
    read: Permission.PlatformAIRead,
    write: Permission.PlatformAIWrite,
    use: Permission.TenantAIUse,
  },
  leads: {
    read: Permission.TenantLeadsRead,
    write: Permission.TenantLeadsWrite,
  },
  quotes: {
    read: Permission.TenantQuotesRead,
    write: Permission.TenantQuotesWrite,
  },
  billing: {
    read: Permission.TenantBillingRead,
    write: Permission.TenantBillingWrite,
  },
  platformBilling: {
    read: Permission.PlatformBillingRead,
    write: Permission.PlatformBillingWrite,
    settings: Permission.PlatformBillingSettings,
  },
  todos: {
    read: Permission.TenantTodosRead,
    write: Permission.TenantTodosWrite,
  },
  messaging: {
    read: Permission.TenantMessagingRead,
    write: Permission.TenantMessagingWrite,
  },
  auth: {
    session: Permission.AuthSession,
  },
} as const;
