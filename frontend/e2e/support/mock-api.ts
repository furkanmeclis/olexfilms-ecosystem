import type { BrowserContext, Page, Route } from "@playwright/test";
import { encode } from "next-auth/jwt";

import { E2E_AUTH_SECRET } from "./constants";

/**
 * Mocked BFF for the service e2e (TEC-183). The browser only talks to the
 * Next.js BFF (`/api/v1/**`); `page.route` answers those calls from this
 * in-memory state, so the production build runs without the Go API.
 */

export const SLUG = "acme";
export const ORG = "0b9c4c1e-0000-4000-8000-000000000001";
export const USER = "0b9c4c1e-0000-4000-8000-000000000002";
export const SERVICE_UUID = "0b9c4c1e-0000-4000-8000-0000000000a1";
const CUSTOMER = "0b9c4c1e-0000-4000-8000-0000000000c1";
const VEHICLE = "0b9c4c1e-0000-4000-8000-0000000000d1";
const CATEGORY = "0b9c4c1e-0000-4000-8000-0000000000e1";
const NOW = "2026-10-01T09:00:00Z";

export const PERMISSIONS = [
  "services.read",
  "services.write",
  "services.complete",
  "customers.read",
  "vehicles.read",
  "catalog.read",
];

export type Json = Record<string, unknown>;
type Item = Json & { uuid: string };
type Log = {
  from_status: string | null;
  to_status: string;
  note: string | null;
  by_other_organization: boolean;
  created_at: string;
};

export const customer = {
  uuid: CUSTOMER,
  name: "Ayşe",
  surname: "Yılmaz",
  email: null,
  phone: "+905551234567",
  status: "active",
  anonymized: false,
  type: "individual",
  company_name: null,
  locale: null,
  created_at: NOW,
  linked_at: NOW,
  first_service_at: null,
};

const vehicle = {
  uuid: VEHICLE,
  customer_uuid: CUSTOMER,
  organization_uuid: ORG,
  plate: "34ABC123",
  plate_normalized: "34ABC123",
  plate_country: "TR",
  vin: null,
  model_year: 2022,
  car_brand: { uuid: "0b9c4c1e-0000-4000-8000-0000000000b1", name: "BMW" },
  car_model: { uuid: "0b9c4c1e-0000-4000-8000-0000000000b2", name: "320i" },
  created_at: NOW,
  updated_at: NOW,
  warnings: [],
};

export const rollUnit = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f1",
  barcode: "OLX-ROLL-001",
  unit_kind: "serial",
  product: {
    uuid: "0b9c4c1e-0000-4000-8000-0000000000f2",
    sku: "PPF-190",
    name: "Olex PPF 190",
    unit_type: "roll_meter",
    available_parts: ["body_kaput", "body_tavan", "body_on_tampon"],
  },
  quantity_on_hand: 1,
  initial_meters: "50.00",
  remaining_meters: "50.00",
};

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

async function panelSessionCookie(orgUuid: string) {
  const value = await encode({
    token: {
      sub: USER,
      email: "e2e@example.com",
      accessToken: "e2e-access",
      refreshToken: "e2e-refresh",
      expiresIn: 3600,
      organizationUuid: orgUuid,
    },
    secret: E2E_AUTH_SECRET,
    salt: "panel-session",
  });
  return `panel-session=${value}; Path=/; SameSite=Lax`;
}

export const membership = {
  uuid: ORG,
  slug: SLUG,
  name: "Acme Bayi",
  role: "owner",
  logo_url: null,
  status: "active",
  access_ends_at: null,
  type: "dealer",
  brand: { slug: "olex", name: "Olex" },
  parent: null,
};

function me(memberships: Json[], activeOrg: string, perms: string[]) {
  return {
    effective_locale: "en",
    effective_timezone: "Europe/Istanbul",
    user: {
      uuid: USER,
      email: "e2e@example.com",
      name: "E2E",
      surname: "Dealer",
      status: "active",
      is_super_admin: false,
      email_verified: true,
      locale: "en",
      timezone: "Europe/Istanbul",
    },
    roles: [],
    permissions: perms,
    grants: Object.fromEntries(perms.map((p) => [p, "organization"])),
    active_organization_uuid: activeOrg,
    organization_roles: ["owner"],
    organizations: memberships,
    links: {
      profile: "/v1/auth/profile",
      change_password: "/v1/auth/password/change",
      notification_preferences: "/v1/notification-preferences",
    },
    channels: { user: `user:${USER}` },
    realtime: { enabled: false, user_channel: `user:${USER}` },
  };
}

/** One BFF call as an extra handler sees it (TEC-304). */
export type MockCall = {
  method: string;
  path: string;
  url: URL;
  body: Json | undefined;
  ok: (data: unknown, status?: number) => Promise<void>;
  route: Route;
};

export class MockApi {
  service: Json | null = null;
  /** Caller permissions and enabled modules (TEC-304: a spec may add). */
  permissions: string[] = [...PERMISSIONS];
  features: string[] = ["services", "catalog"];
  /** VIN of the picked vehicle; the draft snapshots it (TEC-304). */
  vehicleVin: string | null = null;
  /** contracts.intake_required of the organization (TEC-291). */
  contractRequired = false;
  /** The service's intake contract summary (ServiceContractSummary). */
  contract: Json | null = null;
  /**
   * Extra routes tried before the 404 fallback (TEC-304); a handler
   * returns true once it answered the call.
   */
  extra: ((call: MockCall) => Promise<boolean> | boolean)[] = [];
  /** Called after a status transition (TEC-304), e.g. auto links. */
  onTransition: ((to: string) => void) | null = null;
  items: Item[] = [];
  logs: Log[] = [];
  warranties: Json[] = [];
  /** GET /v1/features items (TEC-509: the Özellikler page). */
  featureItems: Json[] = [];
  /** Panel memberships of the caller (org switcher, by-slug lookup). */
  memberships: Json[] = [membership];
  activeOrg = ORG;
  private nextItem = 0;
  /** Units the stock picker lists (TEC-218); barcode is the key. */
  stockUnits: Json[] = [rollUnit];
  /** Every BFF call, for assertions: "METHOD /path?query". */
  calls: string[] = [];
  /** Bodies of write calls by "METHOD /path". */
  bodies: Record<string, unknown[]> = {};
  unknown: string[] = [];

  private view(): Json {
    if (!this.service) throw new Error("no service");
    const status = this.service.status as string;
    const draft = status === "draft";
    return {
      ...this.service,
      status_label: status[0].toUpperCase() + status.slice(1),
      editable: draft,
      items_editable: draft,
      available_transitions: draft ? ["pending", "completed", "cancelled"] : [],
      items: this.items,
      images: [],
      status_logs: this.logs,
      warranties: this.warranties,
      contract_required: this.contractRequired,
      contract: this.contract,
    };
  }

  /** A fresh draft service (what POST /v1/services creates). */
  seedDraft(km: number | null = null) {
    this.service = {
      uuid: SERVICE_UUID,
      service_no: "DSE2E00001",
      status: "draft",
      organization: { uuid: ORG, name: "Acme Bayi", type: "dealer" },
      customer: {
        uuid: CUSTOMER,
        name: customer.name,
        surname: customer.surname,
        phone: customer.phone,
        anonymized: false,
      },
      vehicle_uuid: VEHICLE,
      car_brand: vehicle.car_brand,
      car_model: vehicle.car_model,
      model_year: vehicle.model_year,
      plate: vehicle.plate,
      plate_country: vehicle.plate_country,
      vin: this.vehicleVin,
      km,
      package: null,
      notes: null,
      has_measurement: false,
      cancel_reason: null,
      completed_at: null,
      cancelled_at: null,
      created_at: NOW,
      updated_at: NOW,
    };
    this.logs = [
      {
        from_status: null,
        to_status: "draft",
        note: null,
        by_other_organization: false,
        created_at: NOW,
      },
    ];
  }

  async handle(route: Route) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api/, "");
    const method = req.method();
    this.calls.push(`${method} ${path}${url.search}`);
    // Multipart uploads (TEC-500 intake photos) carry no JSON body.
    let body: Json | undefined;
    try {
      body = req.postData() ? (req.postDataJSON() as Json) : undefined;
    } catch {
      body = undefined;
    }
    if (body !== undefined) {
      (this.bodies[`${method} ${path}`] ??= []).push(body);
    }
    const ok = (data: unknown, status = 200) =>
      route.fulfill({ status, json: envelope(data) });
    const page = (items: unknown[]) =>
      ok({ items, total: items.length, limit: 20, offset: 0 });
    const svc = `/v1/services/${SERVICE_UUID}`;

    const bySlug = path.match(/^\/v1\/public\/organizations\/by-slug\/(.+)$/);
    const org = bySlug && this.memberships.find((m) => m.slug === bySlug[1]);
    if (method === "GET" && org) {
      return ok({
        uuid: org.uuid,
        slug: org.slug,
        name: org.name,
        status: "active",
        logo_url: null,
        access_ok: true,
      });
    }
    if (method === "GET" && path === "/v1/auth/me") {
      return ok(me(this.memberships, this.activeOrg, this.permissions));
    }
    if (method === "POST" && path === "/v1/auth/organization-context") {
      const target = this.memberships.find(
        (m) => m.slug === body?.organization_slug,
      );
      if (!target) {
        return route.fulfill({
          status: 403,
          json: { error: { code: "FORBIDDEN", message: "No membership" } },
        });
      }
      this.activeOrg = target.uuid as string;
      return route.fulfill({
        status: 200,
        headers: { "Set-Cookie": await panelSessionCookie(this.activeOrg) },
        json: envelope({ authenticated: true, expires_in: 3600 }),
      });
    }
    if (method === "GET" && path === "/v1/auth/step-up") {
      return ok({ valid: false, expires_at: null, methods: [] });
    }
    if (method === "GET" && path === "/v1/features") {
      return ok({
        organization_type: "dealer",
        items: this.featureItems,
        enabled: this.features,
      });
    }
    if (method === "GET" && path === "/v1/me/organizations") {
      return ok({ items: this.memberships });
    }
    if (method === "GET" && path === "/v1/customers") {
      return page(url.searchParams.get("q") ? [customer] : []);
    }
    if (method === "GET" && path === "/v1/vehicles") {
      return page([{ ...vehicle, vin: this.vehicleVin }]);
    }
    if (method === "GET" && path === "/v1/catalog/categories") {
      return page([
        {
          uuid: CATEGORY,
          name: "PPF",
          available_parts: ["body_kaput", "body_tavan", "body_on_tampon"],
          sort: 1,
          active: true,
          created_at: NOW,
          updated_at: NOW,
        },
      ]);
    }
    if (method === "POST" && path === "/v1/services") {
      this.seedDraft((body?.km as number | undefined) ?? null);
      return ok(this.view(), 201);
    }
    if (this.service && method === "GET" && path === svc)
      return ok(this.view());
    if (this.service && method === "PATCH" && path === svc) {
      Object.assign(this.service, body, {
        updated_at: new Date().toISOString(),
      });
      return ok(this.view());
    }
    if (this.service && method === "GET" && path === `${svc}/stock-units`) {
      const barcode = url.searchParams.get("barcode");
      const q = url.searchParams.get("q")?.toLowerCase();
      const units = this.stockUnits.filter((u) => {
        const product = u.product as Json;
        if (barcode) return u.barcode === barcode;
        if (!q) return true;
        return [u.barcode, product.sku, product.name].some((v) =>
          String(v).toLowerCase().includes(q),
        );
      });
      return ok({ items: units });
    }
    if (this.service && method === "POST" && path === `${svc}/items`) {
      const unit =
        this.stockUnits.find((u) => u.barcode === body?.barcode) ?? rollUnit;
      const product = unit.product as Json;
      this.items.push({
        uuid: `0b9c4c1e-0000-4000-8000-00000000010${this.nextItem++}`,
        product: {
          uuid: product.uuid,
          sku: product.sku,
          name: product.name,
          unit_type: product.unit_type,
        },
        barcode: body?.barcode,
        unit_kind: unit.unit_kind,
        kind: body?.kind,
        quantity: body?.quantity ?? null,
        meters:
          body?.meters !== undefined ? Number(body.meters).toFixed(2) : null,
        applied_parts: body?.applied_parts ?? [],
        notes: null,
        created_at: new Date().toISOString(),
      });
      return ok(this.view(), 201);
    }
    const itemPath = path.match(/^\/v1\/services\/[^/]+\/items\/([^/]+)$/);
    if (this.service && method === "DELETE" && itemPath) {
      this.items = this.items.filter((it) => it.uuid !== itemPath[1]);
      return ok(this.view());
    }
    if (this.service && method === "POST" && path === `${svc}/transitions`) {
      const to = String(body?.status);
      const at = "2026-10-01T10:30:00Z";
      this.logs.push({
        from_status: this.service.status as string,
        to_status: to,
        note: null,
        by_other_organization: false,
        created_at: at,
      });
      this.service.status = to;
      if (to === "completed") {
        this.service.completed_at = at;
        this.warranties = this.items.map((it, i) => ({
          uuid: `0b9c4c1e-0000-4000-8000-00000000020${i}`,
          public_code: `WE2E${i}`,
          service_item_uuid: it.uuid,
          product_name: (it.product as Json).name,
          item_kind: it.kind,
          status: "active",
          start_at: at,
          end_at: "2036-10-01T20:59:59Z",
          expired_at: null,
          voided_at: null,
        }));
      }
      this.onTransition?.(to);
      return ok(this.view());
    }
    if (method === "GET" && path === "/v1/services") {
      return page(
        this.service
          ? [{ ...this.view(), items: undefined, status_logs: undefined }]
          : [],
      );
    }

    // Dashboard calls on every tenant home. Unmocked they 404 and a
    // "Record not found" toast can cover the header menus (flaky clicks).
    if (method === "GET" && path === "/v1/stats/top-vehicle-models") {
      return ok({
        period: url.searchParams.get("period") ?? "30d",
        group: url.searchParams.get("group") ?? "model",
        since: null,
        items: [],
      });
    }
    if (method === "GET" && path === "/v1/search/specs") {
      return ok({ items: [], enabled: false });
    }
    if (
      method === "POST" &&
      (path === "/v1/realtime/connection-token" ||
        path === "/v1/realtime/subscription-token")
    ) {
      return route.fulfill({
        status: 503,
        json: {
          error: { code: "REALTIME_DISABLED", message: "Realtime disabled" },
        },
      });
    }

    for (const handler of this.extra) {
      if (await handler({ method, path, url, body, ok, route })) return;
    }

    this.unknown.push(`${method} ${path}`);
    return route.fulfill({
      status: 404,
      json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
    });
  }
}

/** Signs a panel session cookie the Next.js proxy accepts (no backend). */
export async function signIn(context: BrowserContext, baseURL: string) {
  const value = await encode({
    token: {
      sub: USER,
      email: "e2e@example.com",
      accessToken: "e2e-access",
      refreshToken: "e2e-refresh",
      expiresIn: 3600,
      organizationUuid: ORG,
    },
    secret: E2E_AUTH_SECRET,
    salt: "panel-session",
  });
  await context.addCookies([{ name: "panel-session", value, url: baseURL }]);
}

/** Routes every BFF call except Auth.js itself to the mock. */
export async function mockApi(page: Page): Promise<MockApi> {
  const api = new MockApi();
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => api.handle(route),
  );
  return api;
}
