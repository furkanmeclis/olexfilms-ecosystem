import type { Page, Route } from "@playwright/test";

/**
 * Mocked BFF for the "My stock" e2e (TEC-224): the units and the product
 * projection of the signed-in organization. `actAs("distributor")` turns
 * the membership into a distributor with one dealer in its subtree, whose
 * stock the distributor reads through the same endpoints (TEC-216).
 */

export const STOCK_SLUG = "acme";
const ORG = "0b9c4c1e-0000-4000-8000-000000000001";
const USER = "0b9c4c1e-0000-4000-8000-000000000002";
export const DEALER_UUID = "0b9c4c1e-0000-4000-8000-0000000000d2";
const NOW = "2026-10-01T09:00:00Z";

const PERMISSIONS = ["stock.read", "organizations.read", "catalog.read"];

type Json = Record<string, unknown>;
type OrgType = "dealer" | "distributor";

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

export const ROLL = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f1",
  barcode: "OLX-ROLL-001",
  unit_kind: "serial",
  status: "available",
  quantity: 1,
  initial_meters: "50.00",
  remaining_meters: "37.50",
  product: {
    uuid: "0b9c4c1e-0000-4000-8000-0000000000f2",
    sku: "PPF-190",
    name: "Olex PPF 190",
    unit_type: "roll_meter",
    uses_fixed_barcode: false,
  },
  location: null,
  purchase_price: null,
  updated_at: NOW,
};

export const KIT = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f3",
  barcode: "OLX-KIT-010",
  unit_kind: "fixed",
  status: "available",
  quantity: 3,
  initial_meters: null,
  remaining_meters: null,
  product: {
    uuid: "0b9c4c1e-0000-4000-8000-0000000000f4",
    sku: "KIT-CLEAN",
    name: "Olex Cleaning Kit",
    unit_type: "piece",
    uses_fixed_barcode: true,
  },
  location: null,
  purchase_price: null,
  updated_at: NOW,
};

export const USED = {
  ...ROLL,
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f5",
  barcode: "OLX-ROLL-002",
  status: "used",
  remaining_meters: "0.00",
};

export const PRODUCTS = [
  {
    product: {
      ...ROLL.product,
      active: true,
      category: { uuid: "c1", name: "PPF" },
    },
    quantity: 0,
    meters: "37.50",
    fixed_barcodes: [],
    updated_at: NOW,
  },
  {
    product: {
      ...KIT.product,
      active: true,
      category: { uuid: "c2", name: "Care" },
    },
    quantity: 3,
    meters: "0.00",
    fixed_barcodes: [
      { unit_uuid: KIT.uuid, barcode: KIT.barcode, quantity: 3 },
    ],
    updated_at: NOW,
  },
];

export class StockMock {
  type: OrgType = "dealer";
  /** Units by holding organization uuid. */
  units: Record<string, Json[]> = { [ORG]: [ROLL, KIT, USED] };
  products: Record<string, Json[]> = { [ORG]: PRODUCTS };
  calls: string[] = [];
  unknown: string[] = [];

  actAs(type: OrgType) {
    this.type = type;
  }

  private membership() {
    return {
      uuid: ORG,
      slug: STOCK_SLUG,
      name: this.type === "distributor" ? "Olex Distribütör" : "Acme Bayi",
      role: "owner",
      logo_url: null,
      status: "active",
      access_ends_at: null,
      type: this.type,
      brand: { slug: "olex", name: "Olex" },
      parent: null,
    };
  }

  private me() {
    return {
      effective_locale: "en",
      effective_timezone: "Europe/Istanbul",
      user: {
        uuid: USER,
        email: "e2e@example.com",
        name: "E2E",
        surname: "Stock",
        status: "active",
        is_super_admin: false,
        email_verified: true,
        locale: "en",
        timezone: "Europe/Istanbul",
      },
      roles: [],
      permissions: PERMISSIONS,
      grants: Object.fromEntries(PERMISSIONS.map((p) => [p, "subtree"])),
      active_organization_uuid: ORG,
      organization_roles: ["owner"],
      organizations: [this.membership()],
      links: {
        profile: "/v1/auth/profile",
        change_password: "/v1/auth/password/change",
        notification_preferences: "/v1/notification-preferences",
      },
      channels: { user: `user:${USER}` },
      realtime: { enabled: false, user_channel: `user:${USER}` },
    };
  }

  async handle(route: Route) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api/, "");
    const method = req.method();
    this.calls.push(`${method} ${path}${url.search}`);
    const ok = (data: unknown, status = 200) =>
      route.fulfill({ status, json: envelope(data) });
    const page = (items: unknown[]) =>
      ok({ items, total: items.length, limit: 20, offset: 0 });

    if (
      method === "GET" &&
      path === `/v1/public/organizations/by-slug/${STOCK_SLUG}`
    ) {
      return ok({
        uuid: ORG,
        slug: STOCK_SLUG,
        name: this.membership().name,
        status: "active",
        logo_url: null,
        access_ok: true,
      });
    }
    if (method === "GET" && path === "/v1/auth/me") return ok(this.me());
    if (method === "GET" && path === "/v1/auth/step-up") {
      return ok({ valid: false, expires_at: null, methods: [] });
    }
    if (method === "GET" && path === "/v1/features") {
      return ok({
        organization_type: this.type,
        items: [],
        enabled: ["stock", "catalog"],
      });
    }
    if (method === "GET" && path === "/v1/me/organizations") {
      return ok({ items: [this.membership()] });
    }
    if (method === "GET" && path === "/v1/tenant/organizations") {
      return page(
        this.type === "distributor"
          ? [{ uuid: DEALER_UUID, name: "Bayi Kadıköy", type: "dealer" }]
          : [],
      );
    }
    const units = path.match(/^\/v1\/stock\/organizations\/([^/]+)\/units$/);
    if (method === "GET" && units) {
      const status = url.searchParams.get("status");
      const barcode = url.searchParams.get("barcode");
      const q = url.searchParams.get("q")?.toLowerCase();
      const productUuid = url.searchParams.get("product_uuid");
      const rows = (this.units[units[1]] ?? []).filter((u) => {
        const product = u.product as Json;
        if (status ? u.status !== status : u.status === "used") return false;
        if (barcode && u.barcode !== barcode) return false;
        if (productUuid && product.uuid !== productUuid) return false;
        if (
          q &&
          ![u.barcode, product.sku, product.name].some((v) =>
            String(v).toLowerCase().includes(q),
          )
        ) {
          return false;
        }
        return true;
      });
      return page(rows);
    }
    const products = path.match(
      /^\/v1\/stock\/organizations\/([^/]+)\/products$/,
    );
    if (method === "GET" && products) {
      return page(this.products[products[1]] ?? []);
    }

    this.unknown.push(`${method} ${path}`);
    return route.fulfill({
      status: 404,
      json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
    });
  }
}

/** Routes every BFF call except Auth.js itself to the stock mock. */
export async function mockStock(page: Page): Promise<StockMock> {
  const api = new StockMock();
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => api.handle(route),
  );
  return api;
}
