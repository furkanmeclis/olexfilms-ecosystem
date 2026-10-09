export const routes = {
  public: {
    root: "/",
    health: "/health",
    share: {
      slug: (slug: string) => `/share/${slug}`,
      signed: (token: string) => `/share/s/${token}`,
    },
    /** TEC-247: landing warranty lookup target (TEC-189 public page). */
    warranty: (no: string) => `/garanti/${encodeURIComponent(no)}`,
    /** TEC-250: public dealer showcase. */
    dealer: (code: string) => `/bayi/${encodeURIComponent(code)}`,
    /** TEC-320: public read-only quote (TEC-315 token link). */
    quote: (token: string) => `/teklif/${encodeURIComponent(token)}`,
    /** TEC-320: public dealer application form (TEC-317). */
    dealerApplication: "/bayi-basvuru",
  },
  tenant: {
    home: (slug: string) => `/t/${slug}`,
    login: (slug: string) => `/t/${slug}/login`,
    profile: {
      root: (slug: string) => `/t/${slug}/profile`,
      password: (slug: string) => `/t/${slug}/profile/password`,
      preferences: (slug: string) => `/t/${slug}/profile/preferences`,
    },
    /** TEC-391: AI usage report of the organization (ai.usage.read). */
    aiUsage: (slug: string) => `/t/${slug}/ai-usage`,
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
    showcase: {
      root: (slug: string) => `/t/${slug}/showcase`,
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
      /** TEC-507: center recommended prices (publish + history). */
      recommendedPrices: (slug: string) =>
        `/t/${slug}/catalog/recommended-prices`,
      /** TEC-507: price discipline report (center, distributor). */
      priceDiscipline: (slug: string) => `/t/${slug}/catalog/price-discipline`,
      services: (slug: string) => `/t/${slug}/service-catalog`,
    },
    /** TEC-311 service subscriptions and the center cancellation queue. */
    serviceSubscriptions: {
      list: (slug: string) => `/t/${slug}/service-subscriptions`,
      detail: (slug: string, uuid: string) =>
        `/t/${slug}/service-subscriptions/${uuid}`,
      cancelRequests: (slug: string) =>
        `/t/${slug}/service-subscriptions/cancel-requests`,
    },
    /** TEC-477: fleets (list, card, bulk service plans). */
    fleets: {
      list: (slug: string) => `/t/${slug}/fleets`,
      detail: (slug: string, uuid: string, tab?: string) =>
        `/t/${slug}/fleets/${uuid}${tab ? `?tab=${tab}` : ""}`,
      newPlan: (slug: string, uuid: string, vehicles?: string[]) =>
        `/t/${slug}/fleets/${uuid}/plans/new${
          vehicles?.length ? `?vehicles=${vehicles.join(",")}` : ""
        }`,
      plan: (slug: string, uuid: string, plan: string) =>
        `/t/${slug}/fleets/${uuid}/plans/${plan}`,
    },
    /** TEC-482: staff certificates and center / distributor approvals. */
    certificates: {
      list: (slug: string) => `/t/${slug}/certificates`,
      verification: (slug: string) => `/t/${slug}/certificates/verification`,
      approvals: (slug: string) => `/t/${slug}/certificates/approvals`,
    },
    /** TEC-349 staff cards and payments, own-book reports. */
    staff: {
      list: (slug: string) => `/t/${slug}/staff`,
      detail: (slug: string, uuid: string) => `/t/${slug}/staff/${uuid}`,
    },
    accountingReports: (slug: string) => `/t/${slug}/accounting/reports`,
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
    /** TEC-348 dealer sale prices, quick sale, suppliers, purchases. */
    dealerSales: {
      prices: (slug: string) => `/t/${slug}/dealer-sales/prices`,
      quickSale: (slug: string) => `/t/${slug}/dealer-sales/quick-sale`,
      suppliers: (slug: string) => `/t/${slug}/dealer-sales/suppliers`,
      purchases: (slug: string) => `/t/${slug}/dealer-sales/purchases`,
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
    /** TEC-340 warranty claim report (failure rate, dealer, parts). */
    warrantyClaims: {
      reports: (slug: string) => `/t/${slug}/warranty-claims/reports`,
      /** TEC-339 claim list and detail. */
      list: (slug: string) => `/t/${slug}/warranty-claims`,
      detail: (slug: string, uuid: string) =>
        `/t/${slug}/warranty-claims/${uuid}`,
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
    /** TEC-486: stock forecast and order suggestions add-on. */
    stockForecast: {
      list: (slug: string) => `/t/${slug}/stock-forecast`,
    },
    /** TEC-489: efficiency and waste analysis add-on. */
    efficiency: {
      root: (slug: string) => `/t/${slug}/efficiency`,
    },
    /** TEC-163 customers: list, new, detail (with vehicles), edit. */
    customers: {
      list: (slug: string) => `/t/${slug}/customers`,
      create: (slug: string) => `/t/${slug}/customers/new`,
      detail: (slug: string, uuid: string) => `/t/${slug}/customers/${uuid}`,
      edit: (slug: string, uuid: string) => `/t/${slug}/customers/${uuid}/edit`,
    },
    /** TEC-190 vehicle detail with the ownership transfer; TEC-372 list. */
    vehicles: {
      list: (slug: string) => `/t/${slug}/vehicles`,
      detail: (slug: string, uuid: string) => `/t/${slug}/vehicles/${uuid}`,
    },
    /** TEC-326 appointment calendar (day / week) and settings tab. */
    appointments: {
      calendar: (slug: string) => `/t/${slug}/appointments`,
      /** TEC-328: network occupancy (center / distributor). */
      occupancy: (slug: string) => `/t/${slug}/appointments/occupancy`,
      /** TEC-328: read-only calendar of one organization of the network. */
      organizationCalendar: (
        slug: string,
        organizationUuid: string,
        preset?: { name?: string; date?: string; view?: "day" | "week" },
      ) => {
        const qs = new URLSearchParams();
        if (preset?.name) qs.set("name", preset.name);
        if (preset?.date) qs.set("date", preset.date);
        if (preset?.view) qs.set("view", preset.view);
        const tail = qs.toString();
        return `/t/${slug}/appointments/occupancy/${encodeURIComponent(organizationUuid)}${tail ? `?${tail}` : ""}`;
      },
    },
    /** TEC-299 measurements: list, detail (part map, VIN, PDF), devices. */
    measurements: {
      list: (slug: string) => `/t/${slug}/measurements`,
      detail: (slug: string, uuid: string) => `/t/${slug}/measurements/${uuid}`,
      devices: (slug: string) => `/t/${slug}/measurements/devices`,
    },
    /** TEC-221 center tasks: list, new, detail (edit, status, comments). */
    tasks: {
      list: (slug: string) => `/t/${slug}/tasks`,
      create: (slug: string) => `/t/${slug}/tasks/new`,
      detail: (slug: string, uuid: string) => `/t/${slug}/tasks/${uuid}`,
    },
    /** TEC-332 announcements: feed, composer and read report. */
    announcements: {
      list: (slug: string) => `/t/${slug}/announcements`,
    },
    /** TEC-408 campaigns: list, wizard, detail and approval queue. */
    campaigns: {
      list: (slug: string) => `/t/${slug}/campaigns`,
      create: (slug: string) => `/t/${slug}/campaigns/new`,
      detail: (slug: string, uuid: string) => `/t/${slug}/campaigns/${uuid}`,
      approvals: (slug: string) => `/t/${slug}/campaigns/approvals`,
    },
    /** TEC-333 document library: folders, items, language versions. */
    library: {
      list: (slug: string) => `/t/${slug}/library`,
    },
    /** TEC-390: AI assistant full page (the header opens it as a sheet). */
    assistant: (slug: string) => `/t/${slug}/assistant`,
    /** TEC-403: pending MCP / WhatsApp AI actions (Go mcp.ApprovalsPath). */
    assistantApprovals: (slug: string) => `/t/${slug}/assistant/approvals`,
    /** TEC-318 lead pipeline: list, new, detail + timeline. */
    leads: {
      list: (slug: string) => `/t/${slug}/leads`,
      create: (slug: string) => `/t/${slug}/leads/new`,
      detail: (slug: string, uuid: string) => `/t/${slug}/leads/${uuid}`,
    },
    /**
     * TEC-231 full warehouse (center and distributor, K12): location tree,
     * scan, stock entries, barcodes (center, K14). TEC-232 adds transfers,
     * counts and end of day under the same root.
     */
    warehouse: {
      root: (slug: string) => `/t/${slug}/warehouse`,
      locations: (slug: string) => `/t/${slug}/warehouse/locations`,
      scan: (slug: string) => `/t/${slug}/warehouse/scan`,
      entries: (slug: string) => `/t/${slug}/warehouse/entries`,
      entry: (slug: string, uuid: string) =>
        `/t/${slug}/warehouse/entries/${uuid}`,
      barcodes: (slug: string) => `/t/${slug}/warehouse/barcodes`,
      // TEC-232 (slice 2)
      transfers: (slug: string) => `/t/${slug}/warehouse/transfers`,
      transfer: (slug: string, uuid: string) =>
        `/t/${slug}/warehouse/transfers/${uuid}`,
      counts: (slug: string) => `/t/${slug}/warehouse/counts`,
      count: (slug: string, uuid: string) =>
        `/t/${slug}/warehouse/counts/${uuid}`,
      endOfDay: (slug: string) => `/t/${slug}/warehouse/end-of-day`,
      endOfDayReport: (slug: string, uuid: string) =>
        `/t/${slug}/warehouse/end-of-day/${uuid}`,
    },
  },
  /** Customer / fleet portal (TEC-90): its own Auth.js instance and BFF. */
  portal: {
    home: "/portal",
    login: "/portal/login",
    forgotPassword: "/portal/forgot-password",
    /** TEC-191: the user's warranties. */
    warranties: "/portal/warranties",
    /** TEC-242 dealer finder; linked from the landing page (TEC-247). */
    dealers: "/portal/dealers",
    /** TEC-241: the user's vehicles, a vehicle and a service. */
    vehicles: "/portal/vehicles",
    vehicle: (uuid: string) => `/portal/vehicles/${encodeURIComponent(uuid)}`,
    service: (uuid: string) => `/portal/services/${encodeURIComponent(uuid)}`,
    /** TEC-245: signed contracts and notification preferences. */
    contracts: "/portal/contracts",
    preferences: "/portal/preferences",
    /** TEC-390: AI assistant (customer realm). */
    assistant: "/portal/assistant",
    /** TEC-327: my appointments and the booking flow (optional preselection). */
    appointments: "/portal/appointments",
    /** TEC-478: read-only fleet portal. */
    fleet: {
      vehicles: "/portal/fleet/vehicles",
      vehicle: (uuid: string) =>
        `/portal/fleet/vehicles/${encodeURIComponent(uuid)}`,
      services: "/portal/fleet/services",
      warranties: "/portal/fleet/warranties",
      account: "/portal/fleet/account",
      reports: "/portal/fleet/reports",
    },
    newAppointment: (preset?: {
      dealer?: { uuid: string; name: string };
      vehicle?: string;
    }) => {
      const q = new URLSearchParams();
      if (preset?.dealer) {
        q.set("dealer", preset.dealer.uuid);
        q.set("dealer_name", preset.dealer.name);
      }
      if (preset?.vehicle) q.set("vehicle", preset.vehicle);
      const qs = q.toString();
      return `/portal/appointments/new${qs ? `?${qs}` : ""}`;
    },
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
      glorian: "/platform/integrations/glorian",
    },
    legalTexts: "/platform/legal-texts",
    /** TEC-403: registered MCP (OAuth) clients. */
    mcpClients: "/platform/mcp-clients",
    /** TEC-391: AI settings, organization quotas and usage (super_admin). */
    ai: {
      root: "/platform/ai",
      tab: (tab: string) => `/platform/ai?tab=${encodeURIComponent(tab)}`,
    },
    conversations: {
      root: "/platform/conversations",
      detail: (uuid: string) =>
        `/platform/conversations?c=${encodeURIComponent(uuid)}`,
    },
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
    certificateTypes: {
      root: "/platform/certificate-types",
    },
    showcases: {
      root: "/platform/showcases",
    },
    /** TEC-489: expected part consumption (efficiency add-on). */
    partConsumptionExpectations: {
      root: "/platform/part-consumption-expectations",
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
    serviceCatalog: {
      root: "/platform/service-catalog",
    },
    /** TEC-290: contract template editor (Lexical, 13 languages). */
    contractTemplates: {
      root: "/platform/contract-templates",
      edit: (uuid: string) => `/platform/contract-templates/${uuid}`,
    },
    /** TEC-353: admin review questions (portal review form). */
    reviewQuestions: {
      root: "/platform/review-questions",
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
