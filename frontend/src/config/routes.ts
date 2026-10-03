export const routes = {
  public: {
    root: "/",
    health: "/health",
    share: {
      slug: (slug: string) => `/share/${slug}`,
      signed: (token: string) => `/share/s/${token}`,
    },
  },
  tenant: {
    home: (slug: string) => `/t/${slug}`,
    login: (slug: string) => `/t/${slug}/login`,
    profile: {
      root: (slug: string) => `/t/${slug}/profile`,
      password: (slug: string) => `/t/${slug}/profile/password`,
      preferences: (slug: string) => `/t/${slug}/profile/preferences`,
    },
    exports: {
      root: (slug: string) => `/t/${slug}/exports`,
      detail: (slug: string, uuid: string) => `/t/${slug}/exports/${uuid}`,
    },
    imports: {
      root: (slug: string) => `/t/${slug}/imports`,
      detail: (slug: string, uuid: string) => `/t/${slug}/imports/${uuid}`,
    },
    settings: {
      root: (slug: string) => `/t/${slug}/settings`,
    },
    features: {
      root: (slug: string) => `/t/${slug}/features`,
    },
    /** TEC-147. Search hits link `/catalog/products/{uuid}` (TEC-145). */
    catalog: {
      products: (slug: string) => `/t/${slug}/catalog/products`,
      productCreate: (slug: string) => `/t/${slug}/catalog/products/new`,
      product: (slug: string, uuid: string) =>
        `/t/${slug}/catalog/products/${uuid}`,
      productEdit: (slug: string, uuid: string) =>
        `/t/${slug}/catalog/products/${uuid}/edit`,
      categories: (slug: string) => `/t/${slug}/catalog/categories`,
    },
    /** TEC-176 accounting; statement and disputes: TEC-195. */
    accounting: {
      root: (slug: string) => `/t/${slug}/accounting`,
      accounts: (slug: string) => `/t/${slug}/accounting/accounts`,
      cari: (slug: string) => `/t/${slug}/accounting/cari`,
      cariDetail: (slug: string, uuid: string) =>
        `/t/${slug}/accounting/cari/${uuid}`,
      entries: (slug: string) => `/t/${slug}/accounting/entries`,
      statement: (slug: string, uuid: string) =>
        `/t/${slug}/accounting/cari/${uuid}/statement`,
      disputes: (slug: string) => `/t/${slug}/accounting/disputes`,
      disputeDetail: (slug: string, uuid: string) =>
        `/t/${slug}/accounting/disputes/${uuid}`,
    },
    /** TEC-181 service wizard, TEC-183 list and detail. */
    services: {
      list: (slug: string) => `/t/${slug}/services`,
      detail: (slug: string, uuid: string) => `/t/${slug}/services/${uuid}`,
      create: (slug: string) => `/t/${slug}/services/new`,
      wizard: (slug: string, uuid: string) =>
        `/t/${slug}/services/${uuid}/wizard`,
    },
    /** TEC-170 orders: list, new draft, detail, draft edit. */
    orders: {
      list: (slug: string) => `/t/${slug}/orders`,
      create: (slug: string) => `/t/${slug}/orders/new`,
      detail: (slug: string, uuid: string) => `/t/${slug}/orders/${uuid}`,
      edit: (slug: string, uuid: string) => `/t/${slug}/orders/${uuid}/edit`,
    },
    /** TEC-191 warranty list and detail. */
    warranties: {
      list: (slug: string) => `/t/${slug}/warranties`,
      detail: (slug: string, uuid: string) => `/t/${slug}/warranties/${uuid}`,
    },
    /** TEC-197 stock transfer requests between siblings: list, new, detail. */
    transfers: {
      list: (slug: string) => `/t/${slug}/transfers`,
      create: (slug: string) => `/t/${slug}/transfers/new`,
      createReturn: (slug: string) => `/t/${slug}/transfers/returns/new`,
      detail: (slug: string, uuid: string) => `/t/${slug}/transfers/${uuid}`,
    },
    /** TEC-224 dealer "My stock" (units on hand, consumed in services). */
    stock: {
      root: (slug: string) => `/t/${slug}/stock`,
    },
    /** TEC-163 customers: list, new, detail (with vehicles), edit. */
    customers: {
      list: (slug: string) => `/t/${slug}/customers`,
      create: (slug: string) => `/t/${slug}/customers/new`,
      detail: (slug: string, uuid: string) => `/t/${slug}/customers/${uuid}`,
      edit: (slug: string, uuid: string) => `/t/${slug}/customers/${uuid}/edit`,
    },
    /** TEC-190 vehicle detail with the ownership transfer. */
    vehicles: {
      detail: (slug: string, uuid: string) => `/t/${slug}/vehicles/${uuid}`,
    },
  },
  /** Customer / fleet portal (TEC-90): its own Auth.js instance and BFF. */
  portal: {
    home: "/portal",
    login: "/portal/login",
    forgotPassword: "/portal/forgot-password",
    /** TEC-191: the user's warranties. */
    warranties: "/portal/warranties",
  },
  guest: {
    login: "/platform/login",
    register: "/platform/register",
    forgotPassword: "/platform/forgot-password",
    resetPassword: "/platform/reset-password",
    verifyEmail: "/platform/verify-email",
  },
  cms: {
    root: "/",
    home: "/",
    profile: {
      root: "/profile",
      password: "/profile/password",
      preferences: "/profile/preferences",
    },
  },
  platform: {
    root: "/platform",
    home: "/platform",
    roles: {
      root: "/platform/roles",
      create: "/platform/roles/create",
      detail: (uuid: string) => `/platform/roles/${uuid}`,
      edit: (uuid: string) => `/platform/roles/${uuid}/edit`,
    },
    users: {
      root: "/platform/users",
      create: "/platform/users/create",
      detail: (uuid: string) => `/platform/users/${uuid}`,
      edit: (uuid: string) => `/platform/users/${uuid}/edit`,
    },
    notifications: {
      root: "/platform/notifications",
      detail: (uuid: string) => `/platform/notifications/${uuid}`,
      center: "/platform/notification-center",
    },
    imports: {
      root: "/platform/imports",
      detail: (uuid: string) => `/platform/imports/${uuid}`,
    },
    exports: {
      root: "/platform/exports",
      detail: (uuid: string) => `/platform/exports/${uuid}`,
    },
    settings: {
      root: "/platform/settings",
    },
    access: {
      root: "/platform/access",
    },
    authSettings: {
      root: "/platform/auth/settings",
    },
    integrations: {
      github: "/platform/integrations/github",
      google: "/platform/integrations/google",
      facebook: "/platform/integrations/facebook",
      apple: "/platform/integrations/apple",
      whatsapp: "/platform/integrations/whatsapp",
    },
    legalTexts: "/platform/legal-texts",
    organizations: {
      root: "/platform/organizations",
      create: "/platform/organizations/create",
      detail: (uuid: string) => `/platform/organizations/${uuid}`,
      edit: (uuid: string) => `/platform/organizations/${uuid}/edit`,
    },
    documentTemplates: {
      root: "/platform/document-templates",
      edit: (uuid: string) => `/platform/document-templates/${uuid}`,
    },
    activity: {
      root: "/platform/activity",
    },
    logs: {
      root: "/platform/logs",
    },
    storage: {
      root: "/platform/storage",
    },
    modules: {
      root: "/platform/modules",
    },
    territories: {
      root: "/platform/territories",
    },
    plateFormats: {
      root: "/platform/settings/plate-formats",
    },
    exchangeRates: {
      root: "/platform/settings/exchange-rates",
    },
    /** TEC-150: car brands / models (super_admin). */
    vehicleCatalog: {
      root: "/platform/vehicle-catalog",
      brand: (uuid: string) => `/platform/vehicle-catalog/${uuid}`,
    },
    /** TEC-222: system settings hub (TEC-215 key/value store + topic links). */
    systemSettings: {
      root: "/platform/system-settings",
    },
    profile: {
      root: "/platform/profile",
      password: "/platform/profile/password",
      preferences: "/platform/profile/preferences",
    },
  },
  errors: {
    unauthorized: "/unauthorized",
    forbidden: "/forbidden",
  },
} as const;

type NestedRoutes<T> = T extends string
  ? T
  : T extends (...args: never[]) => infer R
    ? R extends string
      ? R
      : never
    : T extends Record<string, unknown>
      ? NestedRoutes<T[keyof T]>
      : never;

export type AppRoute = NestedRoutes<typeof routes>;
