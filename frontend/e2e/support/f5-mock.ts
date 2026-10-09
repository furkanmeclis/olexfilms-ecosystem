import type { Page } from "@playwright/test";

import {
  ORG,
  PERMISSIONS,
  SERVICE_UUID,
  USER,
  membership,
  mockApi,
  type Json,
  type MockApi,
  type MockCall,
} from "./mock-api";

/**
 * F5 gate mocks (TEC-516). One in-memory "module world" answers both the
 * platform admin and the tenant module calls, so a switch made in one panel
 * shows up in the next: the resolver below mirrors
 * backend/internal/platform/features/resolve.go (system → admin override →
 * level above → own value → dealer standard → default) and the
 * GET /v1/features handler (modules with visible=false are left out).
 * The short tours (showcase, stock forecast, performance, photo step,
 * e-invoice, recommended prices) add their own fixtures on top.
 */

export const F5 = {
  distributor: "0b9c4c1e-0000-4000-8000-000000005161",
  newDealer: "0b9c4c1e-0000-4000-8000-000000005162",
  request: "0b9c4c1e-0000-4000-8000-000000005163",
  fleet: "0b9c4c1e-0000-4000-8000-000000005164",
  fleetLink: "0b9c4c1e-0000-4000-8000-000000005165",
  vehicles: [
    "0b9c4c1e-0000-4000-8000-000000005166",
    "0b9c4c1e-0000-4000-8000-000000005167",
  ],
  product: "0b9c4c1e-0000-4000-8000-000000005168",
  order: "0b9c4c1e-0000-4000-8000-000000005169",
  invoice: "0b9c4c1e-0000-4000-8000-00000000516a",
  source: "0b9c4c1e-0000-4000-8000-00000000516b",
  dealerCode: "olex-kadikoy",
} as const;

const NOW = "2026-10-09T09:00:00Z";
const DIST_USER = {
  uuid: "0b9c4c1e-0000-4000-8000-00000000516c",
  name: "Deniz Dağıtım",
};
const ADMIN_USER = {
  uuid: "0b9c4c1e-0000-4000-8000-00000000516d",
  name: "Platform Admin",
};

/** 1×1 PNG for photo and map tile urls. */
export const PNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=",
  "base64",
);

const page1 = <T>(items: T[], limit = 20) => ({
  items,
  total: items.length,
  limit,
  offset: 0,
});

type Level = "core" | "standard" | "addon";
type Module = {
  key: string;
  level: Level;
  enabled: boolean;
  default_enabled: boolean;
  paid: boolean;
};

const CORE = [
  "organizations",
  "regions",
  "catalog",
  "stock",
  "warehouse",
  "orders",
  "services",
  "customers",
  "notifications",
  "accounting",
  "search",
  "import_export",
  "tasks",
  "system_settings",
];
const STANDARD = [
  "intake_contracts",
  "measurements",
  "leads",
  "appointments",
  "announcements",
  "warranty_claims",
  "dealer_accounting",
  "service_catalog",
  "dealer_transfers",
  "reviews",
];
const ADDONS = [
  "ai_assistant",
  "whatsapp_gateway",
  "mcp",
  "dealer_showcase",
  "fleet",
  "certificates",
  "stock_forecast",
  "performance",
  "efficiency",
  "campaigns",
  "photo_standard",
  "e_invoice",
  "short_url",
];

/** The module catalog (backend features.Modules: 14 / 10 / 13). */
function catalog(): Module[] {
  const mod = (key: string, level: Level): Module => ({
    key,
    level,
    enabled: true,
    default_enabled: level !== "addon",
    paid: level === "addon" && key !== "photo_standard",
  });
  return [
    ...CORE.map((k) => mod(k, "core")),
    ...STANDARD.map((k) => mod(k, "standard")),
    ...ADDONS.map((k) => mod(k, "addon")),
  ];
}

type Flag = {
  enabled: boolean;
  /** admin | distributor | service (module bundle subscription). */
  source: string;
  set_by?: { uuid: string; name: string };
};

/** Who looks at the panel; each tenant role is one organization. */
export type F5Role = "platform" | "distributor" | "dealer" | "newDealer";

const DISTRIBUTOR_ORG = {
  uuid: F5.distributor,
  slug: "dist",
  name: "Marmara Distribütör",
  role: "owner",
  logo_url: null,
  status: "active",
  access_ends_at: null,
  type: "distributor",
  brand: { slug: "olex", name: "Olex" },
  parent: null,
};
const NEW_DEALER_ORG = {
  ...membership,
  uuid: F5.newDealer,
  slug: "yeni",
  name: "Yeni Bayi",
};

/** Permissions of a dealer owner across the F5 add-ons. */
const DEALER_PERMISSIONS = [
  ...PERMISSIONS,
  "modules.read",
  "certificates.read",
  "fleets.read",
  "fleets.plan",
  "efficiency.read",
  "stock.read",
  "stock_forecast.read",
  "performance.read",
];

type RequestRow = Json & {
  uuid: string;
  organization_uuid: string;
  module_key: string;
  status: string;
};

/**
 * Module chain of one brand: center → Marmara Distribütör → Acme Bayi
 * (and a new dealer without own values).
 */
export class F5World {
  modules = catalog();
  /** Own values of each tenant organization, by org uuid then key. */
  flags: Record<string, Record<string, Flag>> = {
    [F5.distributor]: {},
    [ORG]: {},
    [F5.newDealer]: {},
  };
  /** The distributor's dealer standard (values for dealers without one). */
  standard: Record<string, Flag> = {};
  requests: RequestRow[] = [];
  role: F5Role = "dealer";

  constructor(readonly api: MockApi) {
    // MockApi answers GET /v1/features from these fields: resolve them per
    // request so every switch made meanwhile is seen.
    Object.defineProperty(api, "featureItems", {
      get: () => this.items(this.orgUuid()),
      set: () => undefined,
    });
    Object.defineProperty(api, "features", {
      get: () => this.enabled(this.orgUuid()),
      set: () => undefined,
    });
    api.extra.push((call) => this.handle(call));
  }

  module(key: string): Module {
    const m = this.modules.find((x) => x.key === key);
    if (!m) throw new Error(`unknown module ${key}`);
    return m;
  }

  orgUuid(): string {
    if (this.role === "distributor") return F5.distributor;
    if (this.role === "newDealer") return F5.newDealer;
    return ORG;
  }

  /** Switches the signed-in user (memberships, active org, permissions). */
  as(role: F5Role) {
    this.role = role;
    const api = this.api;
    if (role === "platform") {
      // Platform staff: no tenant membership (like installF4Platform).
      api.memberships = [];
      api.activeOrg = null as unknown as string;
      api.permissions = ["platform.modules.read", "platform.modules.write"];
    } else if (role === "distributor") {
      api.memberships = [DISTRIBUTOR_ORG];
      api.activeOrg = F5.distributor;
      api.permissions = ["modules.read", "modules.manage"];
    } else if (role === "newDealer") {
      api.memberships = [NEW_DEALER_ORG];
      api.activeOrg = F5.newDealer;
      api.permissions = [...DEALER_PERMISSIONS];
    } else {
      api.memberships = [membership];
      api.activeOrg = ORG;
      api.permissions = [...DEALER_PERMISSIONS];
    }
  }

  /** resolve.go for one organization of the chain. */
  state(org: string, m: Module) {
    const st = {
      key: m.key,
      level: m.level,
      paid: m.paid,
      default_enabled: m.default_enabled,
      enabled: false,
      visible: false,
      upstream_enabled: false,
      admin_override: false,
      source: "default",
      set_by: undefined as Flag["set_by"],
    };
    if (m.level === "core") {
      return {
        ...st,
        enabled: true,
        visible: true,
        upstream_enabled: true,
        source: "core",
      };
    }
    if (!m.enabled) return { ...st, source: "system", set_by: ADMIN_USER };
    const own = this.flags[org]?.[m.key];
    st.admin_override = own?.source === "admin";
    if (org === F5.distributor) {
      st.upstream_enabled = st.visible = true;
      if (own)
        Object.assign(st, {
          enabled: own.enabled,
          source: own.source,
          set_by: own.set_by,
        });
      else st.enabled = m.default_enabled;
      return st;
    }
    const parent = this.flags[F5.distributor][m.key];
    const parentOn = parent ? parent.enabled : m.default_enabled;
    st.upstream_enabled = parentOn;
    if (own && st.admin_override) {
      return {
        ...st,
        enabled: own.enabled,
        source: own.source,
        set_by: own.set_by,
        visible: true,
      };
    }
    if (!parentOn) return { ...st, source: "upstream" };
    if (own) {
      return {
        ...st,
        enabled: own.enabled,
        source: own.source,
        set_by: own.set_by,
        visible: true,
      };
    }
    const std = this.standard[m.key];
    if (std) {
      return {
        ...st,
        enabled: std.enabled,
        source: "standard",
        set_by: std.set_by,
        visible: true,
      };
    }
    return { ...st, enabled: m.default_enabled, visible: true };
  }

  enabled(org: string): string[] {
    return this.modules
      .map((m) => this.state(org, m))
      .filter((st) => st.enabled)
      .map((st) => st.key);
  }

  /** GET /v1/features items (handler.go List). */
  items(org: string): Json[] {
    return this.modules
      .map((m) => this.state(org, m))
      .filter((st) => st.visible)
      .map((st) => {
        const free = st.default_enabled;
        const price =
          st.key === "fleet"
            ? {
                amount: "250.00",
                currency: "TRY",
                recurrence: "monthly",
                item_uuid: "0b9c4c1e-0000-4000-8000-00000000516e",
                item_name: "Filo paketi",
              }
            : null;
        const last = this.requests
          .filter((r) => r.organization_uuid === org && r.module_key === st.key)
          .at(-1);
        return {
          ...st,
          description: `${st.key} description`,
          free_default: free,
          price,
          contact_for_price: st.paid && !free && price === null,
          request: last
            ? {
                uuid: last.uuid,
                status: last.status,
                note: last.note,
                decision_note: last.decision_note,
                created_at: last.created_at,
                decided_at: last.decided_at,
              }
            : null,
        };
      });
  }

  /** Admin sets a module for one organization (source admin, override). */
  adminSet(org: string, key: string, enabled: boolean) {
    this.flags[org][key] = { enabled, source: "admin", set_by: ADMIN_USER };
  }

  /** A module bundle subscription switched the module on (source service). */
  subscribe(org: string, key: string) {
    this.flags[org][key] = { enabled: true, source: "service" };
  }

  /** The subscription ended: ExpireDue → ClearByService drops the value. */
  expire(org: string, key: string) {
    if (this.flags[org][key]?.source === "service") delete this.flags[org][key];
  }

  private platformModule(m: Module) {
    return { ...m, set_by: m.enabled ? undefined : ADMIN_USER };
  }

  private standardEntries() {
    return this.modules
      .map((m) => this.state(F5.distributor, m))
      .filter((st) => st.level !== "core" && st.source !== "system")
      .map((st) => {
        const std = this.standard[st.key];
        return {
          key: st.key,
          level: st.level,
          paid: st.paid,
          distributor_enabled: st.enabled,
          enabled: std ? std.enabled : st.default_enabled,
          explicit: Boolean(std),
          ...(std?.set_by ? { set_by: std.set_by } : {}),
        };
      });
  }

  private async handle({ method, path, url, body, ok }: MockCall) {
    if (method === "GET" && path === "/v1/platform/modules") {
      await ok({ items: this.modules.map((m) => this.platformModule(m)) });
      return true;
    }
    const platformKey = path.match(/^\/v1\/platform\/modules\/([a-z_]+)$/);
    if (method === "PATCH" && platformKey) {
      const m = this.module(platformKey[1]);
      Object.assign(m, body);
      await ok(this.platformModule(m));
      return true;
    }
    if (method === "GET" && path === "/v1/platform/modules/requests") {
      await ok(page1([]));
      return true;
    }
    const featureRequest = path.match(/^\/v1\/features\/([a-z_]+)\/request$/);
    if (featureRequest && method === "POST") {
      const org = this.orgUuid();
      const row: RequestRow = {
        uuid: F5.request,
        organization_uuid: org,
        organization_name: this.api.memberships[0]?.name as string,
        organization_type: "dealer",
        module_key: featureRequest[1],
        note: (body?.note as string | undefined) ?? "",
        status: "pending",
        requested_by_uuid: USER,
        requested_by_name: "E2E Dealer",
        decided_by_uuid: null,
        decided_by_name: "",
        decision_note: "",
        created_at: NOW,
        decided_at: null,
      };
      this.requests.push(row);
      await ok({ status: "requested", recipients: 1, request: row }, 202);
      return true;
    }
    if (method === "GET" && path === "/v1/tenant/modules/requests") {
      const status = url.searchParams.get("status");
      const rows = this.requests.filter((r) => !status || r.status === status);
      await ok(page1(rows));
      return true;
    }
    const decide = path.match(
      /^\/v1\/tenant\/modules\/requests\/([^/]+)\/(approve|reject)$/,
    );
    if (method === "POST" && decide) {
      const row = this.requests.find((r) => r.uuid === decide[1]);
      if (!row) return false;
      const approved = decide[2] === "approve";
      Object.assign(row, {
        status: approved ? "approved" : "rejected",
        decided_by_uuid: DIST_USER.uuid,
        decided_by_name: DIST_USER.name,
        decision_note: (body?.note as string | undefined) ?? "",
        decided_at: NOW,
      });
      if (approved) {
        this.flags[row.organization_uuid][row.module_key] = {
          enabled: true,
          source: "distributor",
          set_by: DIST_USER,
        };
      }
      await ok(row);
      return true;
    }
    if (method === "GET" && path === "/v1/tenant/modules/dealer-standard") {
      await ok({ items: this.standardEntries() });
      return true;
    }
    const std = path.match(
      /^\/v1\/tenant\/modules\/dealer-standard\/([a-z_]+)$/,
    );
    if (std && (method === "PUT" || method === "DELETE")) {
      if (method === "PUT") {
        this.standard[std[1]] = {
          enabled: Boolean(body?.enabled),
          source: "distributor",
          set_by: DIST_USER,
        };
      } else {
        delete this.standard[std[1]];
      }
      await ok({ items: this.standardEntries() });
      return true;
    }
    if (method === "GET" && path === "/v1/tenant/modules/dealers") {
      const dealers = [
        { org: ORG, name: "Acme Bayi", slug: "acme" },
        { org: F5.newDealer, name: "Yeni Bayi", slug: "yeni" },
      ];
      await ok(
        page1(
          dealers.map((d) => ({
            uuid: d.org,
            name: d.name,
            slug: d.slug,
            modules: this.modules
              .filter((m) => m.level !== "core")
              .map((m) => {
                const st = this.state(d.org, m);
                return {
                  key: st.key,
                  enabled: st.enabled,
                  visible: st.visible,
                  source: st.source,
                  admin_override: st.admin_override,
                };
              }),
          })),
        ),
      );
      return true;
    }
    return false;
  }
}

/** Mocked BFF with the F5 module world (dealer signed in). */
export async function installF5(page: Page) {
  const api = await mockApi(page);
  const world = new F5World(api);
  world.as("dealer");
  return world;
}

/**
 * Step-up through the dialog (password): GET /v1/auth/step-up says "not
 * verified" until POST /v1/auth/step-up/password succeeds. A later
 * page.route wins over the MockApi one, which always says "not verified".
 */
export async function installStepUp(page: Page, api: MockApi) {
  const state = { verified: false, passwords: [] as string[] };
  await page.route("**/api/v1/auth/step-up", (route) =>
    route.fulfill({
      json: {
        success: true,
        data: state.verified
          ? {
              valid: true,
              expires_at: "2026-10-09T09:15:00Z",
              methods: ["password"],
            }
          : { valid: false, expires_at: null, methods: ["password"] },
        meta: {},
      },
    }),
  );
  api.extra.push(async ({ method, path, body, ok }) => {
    if (method !== "POST" || path !== "/v1/auth/step-up/password") return false;
    state.passwords.push(String(body?.password));
    state.verified = true;
    await ok({
      valid: true,
      expires_at: "2026-10-09T09:15:00Z",
      method: "password",
    });
    return true;
  });
  return state;
}

/** Turns the signed-in organization into the brand center. */
export function asCenter(api: MockApi) {
  api.memberships = api.memberships.map((m) => ({
    ...m,
    type: "center",
    name: "Olex Merkez",
  }));
}

/** Fleet fixtures (TEC-477): one fleet linked to Acme Bayi, two vehicles. */
export function mockFleet(api: MockApi) {
  const link = {
    uuid: F5.fleetLink,
    status: "active",
    dealer_uuid: ORG,
    dealer_name: "Acme Bayi",
    started_at: NOW,
    ended_at: null,
    created_at: NOW,
  };
  const vehicle = (uuid: string, i: number) => ({
    uuid,
    plate: `34 FLT 0${i + 1}`,
    plate_country: "TR",
    vin: null,
    model_year: 2024,
    car_brand: { uuid: "b", name: "Renault" },
    car_model: { uuid: "m", name: "Clio" },
    last_service_at: null,
    active_warranty_count: 0,
    created_at: NOW,
  });
  const bodies: Json[] = [];
  api.extra.push(async ({ method, path, body, ok }) => {
    if (method === "GET" && path === "/v1/fleets") {
      await ok(
        page1([
          {
            uuid: F5.fleet,
            name: "Ege Kiralama Filo",
            legal_name: "Ege Kiralama A.Ş.",
            tax_number: "1234567890",
            status: "active",
            vehicle_count: 2,
            last_service_at: null,
            link,
          },
        ]),
      );
      return true;
    }
    if (method === "GET" && path === `/v1/fleets/${F5.fleet}`) {
      await ok({
        uuid: F5.fleet,
        name: "Ege Kiralama Filo",
        status: "active",
        profile: {
          tax_number: "1234567890",
          tax_office: null,
          legal_name: "Ege Kiralama A.Ş.",
          contact_name: null,
          contact_phone: null,
          billing_email: null,
          report_frequency: "monthly",
          report_locale: "tr",
        },
        has_primary_user: true,
        vehicle_count: 2,
        active_warranty_count: 0,
        service_count: 0,
        recent_services: [],
        links: [link],
        cari: null,
        created_at: NOW,
      });
      return true;
    }
    if (method === "GET" && path === `/v1/fleets/${F5.fleet}/vehicles`) {
      await ok(page1(F5.vehicles.map(vehicle), 100));
      return true;
    }
    if (
      method === "POST" &&
      path === `/v1/fleets/${F5.fleet}/service-plans/preview`
    ) {
      bodies.push(body ?? {});
      await ok({
        fleet_uuid: F5.fleet,
        dealer_uuid: ORG,
        service_type: body?.service_type,
        note: "",
        appointments: F5.vehicles.map((uuid, i) => ({
          vehicle_uuid: uuid,
          starts_at: `2026-10-12T0${6 + i}:00:00Z`,
        })),
        warnings: [],
      });
      return true;
    }
    return false;
  });
  return bodies;
}

/**
 * Showcase editor (TEC-482) for Acme Bayi with the code the upstream mock
 * serves on /bayi/{code}; the public lead form config is passed through to
 * the upstream mock, the lead itself is captured here.
 */
export function mockShowcase(api: MockApi) {
  const state = { status: "draft", leads: [] as Json[] };
  const showcase = () => ({
    uuid: "0b9c4c1e-0000-4000-8000-00000000516f",
    organization: {
      uuid: ORG,
      code: F5.dealerCode,
      name: "Olex Kadıköy",
      type: "dealer",
      city: "İstanbul",
    },
    status: state.status,
    content: {
      en: {
        headline: "Premium PPF studio",
        about: "Full body PPF since 2015.",
      },
    },
    working_hours: {},
    social_links: {},
    seo_keywords: [],
    google_place_id: null,
    google_rating: null,
    google_review_count: null,
    google_rating_source: null,
    google_rating_updated_at: null,
    published_content: null,
    published_at: state.status === "published" ? NOW : null,
    submitted_at: null,
    reviewed_at: null,
    review_note: null,
    updated_at: NOW,
    approval_required: false,
    max_photos: 12,
    services: [
      {
        uuid: "0b9c4c1e-0000-4000-8000-000000005170",
        kind: "custom",
        category: null,
        title: { en: "Full body PPF" },
        description: { en: "Self-healing film" },
        visible: true,
        sort_order: 1,
      },
    ],
    photos: [],
    places_configured: false,
    manual_rating_allowed: true,
    google_place_id_suggestion: null,
  });
  api.extra.push(async ({ method, path, body, ok, route }) => {
    if (method === "GET" && path === "/v1/showcase") {
      await ok(showcase());
      return true;
    }
    if (method === "POST" && path === "/v1/showcase/submit") {
      state.status = "published";
      await ok(showcase());
      return true;
    }
    if (path === `/v1/public/dealers/${F5.dealerCode}/lead-form/config`) {
      await route.continue();
      return true;
    }
    if (
      method === "POST" &&
      path === `/v1/public/dealers/${F5.dealerCode}/leads`
    ) {
      state.leads.push(body ?? {});
      await ok({ received: true }, 202);
      return true;
    }
    return false;
  });
  return state;
}

/**
 * Stock forecast list (TEC-486). The paths are the ones the screen calls
 * (stock-forecast.service.ts); see the PR note on the API contract.
 */
export function mockStockForecast(api: MockApi) {
  const drafts: Json[] = [];
  const row = (over: Json) => ({
    uuid: "f-ok",
    product: {
      uuid: F5.product,
      sku: "PPF-190",
      name: "Olex PPF 190",
      category: { uuid: "c1", name: "PPF" },
      unit_type: "piece",
    },
    on_hand_qty: 4,
    avg_daily_30: "1.0",
    avg_daily_90: "0.8",
    seasonality_factor: "1.0",
    days_left: "4",
    depletion_date: "2026-10-13",
    data_days: 120,
    min_data_days: 90,
    status: "critical",
    suggested_qty: 10,
    suggested_meters: null,
    warning_days: 14,
    cover_days: 30,
    vehicles_left: null,
    ...over,
  });
  api.extra.push(async ({ method, path, body, ok }) => {
    if (method === "GET" && path === "/v1/stock-forecasts") {
      await ok(
        page1([
          row({}),
          row({
            uuid: "f-new",
            product: {
              uuid: "0b9c4c1e-0000-4000-8000-000000005171",
              sku: "PPF-150",
              name: "Olex PPF 150",
            },
            status: "insufficient_data",
            data_days: 40,
            suggested_qty: null,
          }),
        ]),
      );
      return true;
    }
    if (method === "POST" && path === "/v1/stock-forecasts/order-drafts") {
      drafts.push(body ?? {});
      await ok({ uuid: F5.order, order_no: "ORD-2026-0516" }, 201);
      return true;
    }
    return false;
  });
  return drafts;
}

/** Performance panel (TEC-496) for the center: ranking and region map. */
export async function mockPerformance(page: Page, api: MockApi) {
  // OSM tiles are not under /api; answer them so the run stays offline.
  await page.route("https://tile.openstreetmap.org/**", (route) =>
    route.fulfill({ contentType: "image/png", body: PNG }),
  );
  api.extra.push(async ({ method, path, url, ok }) => {
    if (method === "GET" && path === "/v1/performance/dashboard") {
      await ok({
        period: url.searchParams.get("period") ?? "2026-10",
        metrics: { services_count: { current: { value: "42" } } },
        trend: [],
        targets: [],
        subtree: [],
      });
      return true;
    }
    if (method === "GET" && path === "/v1/performance/ranking") {
      const distributorsOnly = url.searchParams.get("type") === "distributor";
      await ok(
        page1(
          distributorsOnly
            ? [
                {
                  rank: 1,
                  organization_uuid: F5.distributor,
                  name: "Marmara Distribütör",
                  type: "distributor",
                  metrics: { services_count: { value: "60" } },
                },
              ]
            : [
                {
                  rank: 1,
                  organization_uuid: ORG,
                  name: "Kadıköy Bayi",
                  type: "dealer",
                  distributor: {
                    uuid: F5.distributor,
                    name: "Marmara Distribütör",
                  },
                  province_id: 34,
                  province_name: "İstanbul",
                  currency: "TRY",
                  metrics: { services_count: { value: "42" } },
                },
                {
                  rank: 2,
                  organization_uuid: F5.newDealer,
                  name: "Bursa Bayi",
                  type: "dealer",
                  distributor: {
                    uuid: F5.distributor,
                    name: "Marmara Distribütör",
                  },
                  province_id: 16,
                  province_name: "Bursa",
                  currency: "TRY",
                  metrics: { services_count: { value: "18" } },
                },
              ],
        ),
      );
      return true;
    }
    if (method === "GET" && path === "/v1/geo/countries/TR/provinces") {
      await ok({ items: [] });
      return true;
    }
    if (method === "GET" && path === "/v1/performance/map") {
      await ok({
        level: url.searchParams.get("level") ?? "province",
        period: url.searchParams.get("period") ?? "2026-10",
        metric: "services_count",
        items: [
          {
            level: "province",
            id: 34,
            code: "34",
            name: "İstanbul",
            country_iso2: "TR",
            country_name: "Türkiye",
            dealer_count: 4,
            missing_coordinates: 0,
            latitude: 41,
            longitude: 29,
            metric_avg: 8,
            distributor: { uuid: F5.distributor, name: "Marmara Distribütör" },
          },
        ],
        empty_regions: [
          {
            level: "province",
            id: 6,
            code: "06",
            name: "Ankara",
            country_iso2: "TR",
            empty_reason: "unassigned_territory",
          },
        ],
        missing_coordinates: 0,
      });
      return true;
    }
    return false;
  });
}

/**
 * Intake photo step (TEC-500): two required angles, only "front" taken,
 * and POST …/contract answers 422 PHOTO_STANDARD_INCOMPLETE for "rear".
 */
export function mockIntakePhotos(api: MockApi) {
  const svc = `/v1/services/${SERVICE_UUID}`;
  const taken = new Set<string>(["front"]);
  const angle = (key: string, name: string, sort: number) => ({
    uuid: `0b9c4c1e-0000-4000-8000-0000000051${sort}`,
    key,
    name: { en: name, tr: name },
    hint: { en: `Shoot the ${name.toLowerCase()} from 3 m` },
    required: true,
    sort_order: sort,
    active: true,
  });
  const angles = [angle("front", "Front", 10), angle("rear", "Rear", 20)];
  const photo = (key: string) => ({
    uuid: `0b9c4c1e-0000-4000-8000-0000000052${key === "front" ? "10" : "20"}`,
    angle_key: key,
    url: `${svc}/intake-photos/${key}/file`,
    mime: "image/png",
    size: PNG.length,
    sha256: "0".repeat(64),
    created_at: NOW,
  });
  api.extra.push(async ({ method, path, ok, route }) => {
    if (method === "GET" && path === `${svc}/intake-photos`) {
      const rows = angles.map((a) => ({
        angle: a,
        required: true,
        missing: !taken.has(a.key),
        ...(taken.has(a.key) ? { photo: photo(a.key) } : {}),
      }));
      await ok({
        service_uuid: SERVICE_UUID,
        angles: rows,
        missing: rows.filter((r) => r.missing).map((r) => r.angle.key),
      });
      return true;
    }
    const upload = path.match(/^\/v1\/services\/[^/]+\/intake-photos\/(\w+)$/);
    if (method === "POST" && upload) {
      taken.add(upload[1]);
      await ok(photo(upload[1]), 201);
      return true;
    }
    if (method === "GET" && path.endsWith("/file")) {
      await route.fulfill({ status: 200, contentType: "image/png", body: PNG });
      return true;
    }
    if (method === "POST" && path === `${svc}/contract`) {
      taken.delete("rear");
      await route.fulfill({
        status: 422,
        json: {
          success: false,
          error: {
            code: "PHOTO_STANDARD_INCOMPLETE",
            message:
              "Every required intake photo angle must be photographed first",
            details: [
              {
                field: "intake_photos.rear",
                code: "missing",
                message: "required intake photo is missing",
              },
            ],
          },
          data: { missing_angles: ["rear"] },
        },
      });
      return true;
    }
    return false;
  });
}

/** e-Invoice (TEC-504): one billable order → draft → archived with XML. */
export function mockEinvoice(api: MockApi) {
  const state = {
    status: "none" as "none" | "draft" | "archived",
    created: [] as Json[],
  };
  const invoice = () => ({
    uuid: F5.invoice,
    number: state.status === "archived" ? "EAR2026000000043" : null,
    profile: "EARSIVFATURA",
    invoice_type: "SATIS",
    status: state.status === "archived" ? "archived" : "draft",
    validation_status: "valid",
    validation_messages: [],
    source_type: "order",
    source_uuid: F5.source,
    buyer_organization: { uuid: F5.distributor, name: "Marmara Distribütör" },
    buyer: {
      name: "Marmara Distribütör",
      vkn: "1234567890",
      einvoice_registered: false,
    },
    seller: { name: "Olex Merkez", einvoice_registered: true },
    lines: [],
    tax_breakdown: [],
    currency: "TRY",
    line_extension: "300.00",
    tax_exclusive: "300.00",
    tax_total: "60.00",
    payable: "360.00",
    issue_date: "2026-10-09",
    xml_sha256: state.status === "archived" ? "a".repeat(64) : null,
    has_xml: state.status === "archived",
    has_pdf: state.status === "archived",
    error: null,
    voided_at: null,
    void_reason: null,
    created_at: NOW,
    updated_at: NOW,
  });
  api.extra.push(async ({ method, path, body, ok, route }) => {
    const inv = `/v1/einvoices/${F5.invoice}`;
    if (method === "GET" && path === "/v1/einvoices/billable") {
      await ok(
        page1(
          state.status === "none"
            ? [
                {
                  source_type: "order",
                  source_uuid: F5.source,
                  source_no: "ORD-2026-0516",
                  buyer_organization: {
                    uuid: F5.distributor,
                    name: "Marmara Distribütör",
                  },
                  currency: "TRY",
                  line_extension: "300.00",
                  tax_total: "60.00",
                  payable: "360.00",
                  billable_at: NOW,
                  period_start: null,
                  period_end: null,
                },
              ]
            : [],
        ),
      );
      return true;
    }
    if (method === "POST" && path === "/v1/einvoices") {
      state.created.push(body ?? {});
      state.status = "draft";
      await ok(invoice(), 201);
      return true;
    }
    if (method === "GET" && path === "/v1/einvoices") {
      await ok(page1(state.status === "none" ? [] : [invoice()]));
      return true;
    }
    if (method === "GET" && path === inv) {
      await ok(invoice());
      return true;
    }
    if (
      method === "GET" &&
      (path === `${inv}/preview` || path === `${inv}/html`)
    ) {
      await route.fulfill({
        status: 200,
        contentType: "text/html; charset=utf-8",
        body: "<html><body><h1>e-Arşiv</h1></body></html>",
      });
      return true;
    }
    if (method === "POST" && path === `${inv}/archive`) {
      state.status = "archived";
      await ok(invoice());
      return true;
    }
    if (method === "GET" && path === `${inv}/xml`) {
      await route.fulfill({
        status: 200,
        contentType: "application/xml",
        headers: {
          "content-disposition": 'attachment; filename="EAR2026000000043.xml"',
        },
        body: '<?xml version="1.0"?><Invoice/>',
      });
      return true;
    }
    return false;
  });
  return state;
}

/**
 * Recommended prices (TEC-507): the center publishes, the dealer's sale
 * price list then carries the recommended price and its deviation.
 */
export function mockRecommendedPrices(api: MockApi) {
  const state = { recommended: "1000.00", published: [] as Json[] };
  const sale = 900;
  api.extra.push(async ({ method, path, body, ok }) => {
    if (method === "GET" && path === "/v1/catalog/products") {
      await ok(
        page1([
          {
            uuid: F5.product,
            sku: "PPF-190",
            name: "Olex PPF 190",
            active: true,
            category: { uuid: "c1", name: "PPF" },
          },
        ]),
      );
      return true;
    }
    if (method === "GET" && path === "/v1/tenant/pricing/recommended/current") {
      await ok(
        page1([
          {
            product_uuid: F5.product,
            product_sku: "PPF-190",
            product_name: "Olex PPF 190",
            price: state.recommended,
            currency: "TRY",
            country_iso2: "",
            scope: "currency",
            effective_from: "2026-10-01",
            version_uuid: "v1",
            source: "publish",
            batch_id: null,
          },
        ]),
      );
      return true;
    }
    if (
      method === "GET" &&
      path === "/v1/tenant/pricing/recommended/versions"
    ) {
      await ok(page1([]));
      return true;
    }
    if (
      method === "GET" &&
      path === "/v1/tenant/pricing/recommended/settings"
    ) {
      await ok({ deviation_warning_pct: 15 });
      return true;
    }
    if (method === "GET" && path === "/v1/geo/countries") {
      await ok({
        items: [
          {
            id: 1,
            iso2: "TR",
            iso3: "TUR",
            name_en: "Turkey",
            name_tr: "Türkiye",
            default_currency: "TRY",
          },
        ],
      });
      return true;
    }
    if (
      method === "POST" &&
      path === "/v1/tenant/pricing/recommended/publish"
    ) {
      state.published.push(body ?? {});
      const rows = (body?.rows as Json[] | undefined) ?? [];
      state.recommended = String(rows[0]?.price ?? state.recommended);
      await ok({
        batch_id: "0b9c4c1e-0000-4000-8000-000000005172",
        price_count: rows.length,
        applied_count: rows.length,
        scheduled_count: 0,
        versions: [],
      });
      return true;
    }
    if (method === "GET" && path === "/v1/dealer-prices/catalog") {
      const rec = Number(state.recommended);
      await ok(
        page1([
          {
            product_uuid: F5.product,
            sku: "PPF-190",
            name: "Olex PPF 190",
            uses_fixed_barcode: false,
            currency: "TRY",
            sale_price: sale.toFixed(2),
            recommended_sale_price: state.recommended,
            purchase_price: null,
            estimated_profit: null,
            updated_at: NOW,
            recommended: {
              price: state.recommended,
              currency: "TRY",
              country_iso2: "",
              scope: "currency",
              effective_from: "2026-10-09",
            },
            deviation_pct: (((sale - rec) / rec) * 100).toFixed(2),
          },
        ]),
      );
      return true;
    }
    return false;
  });
  return state;
}
