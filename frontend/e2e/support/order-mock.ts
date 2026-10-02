import type { Page, Route } from "@playwright/test";

/**
 * Mocked BFF for the order e2e (TEC-170): one in-memory order moving
 * through the TEC-166..168 state machine. The signed-in user switches
 * between the buyer (a distributor) and its supplier (the center) with
 * `actAs`; the order's role and available_transitions follow the side,
 * like the Go API.
 */

export const ORDER_SLUG = "acme";
const ORG = "0b9c4c1e-0000-4000-8000-000000000001";
const USER = "0b9c4c1e-0000-4000-8000-000000000002";
export const ORDER_UUID = "0b9c4c1e-0000-4000-8000-0000000000a7";
const ITEM_UUID = "0b9c4c1e-0000-4000-8000-0000000000a8";
const NOW = "2026-10-01T09:00:00Z";

export const PRODUCT = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f5",
  sku: "KIT-001",
  name: "Olex Bakım Kiti",
  unit_type: "piece",
};
export const FIXED_BARCODE = "OLX-FIX-001";

const PERMISSIONS = [
  "orders.read",
  "orders.write",
  "orders.approve",
  "orders.ship",
  "orders.receive",
  "orders.cancel",
  "catalog.read",
  "pricing.purchase.read",
];

type Side = "buyer" | "seller";
type Json = Record<string, unknown>;

const NEXT: Record<Side, Record<string, string[]>> = {
  buyer: {
    draft: ["submitted", "cancelled"],
    submitted: ["cancelled"],
    approved: ["cancelled"],
    preparing: ["cancelled"],
    ready: ["cancelled"],
    shipped: ["received", "cancelling"],
  },
  seller: {
    submitted: ["approved", "cancelled"],
    approved: ["preparing", "processing", "cancelled"],
    preparing: ["ready", "cancelled"],
    ready: ["shipped", "cancelled"],
    shipped: ["cancelling"],
    cancelling: ["cancelled"],
  },
};

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

export class OrderMock {
  side: Side = "buyer";
  order: Json | null = null;
  quantity = 0;
  units: Json[] = [];
  history: Json[] = [];
  calls: string[] = [];
  bodies: Record<string, unknown[]> = {};
  unknown: string[] = [];

  actAs(side: Side) {
    this.side = side;
  }

  private membership() {
    const center = this.side === "seller";
    return {
      uuid: ORG,
      slug: ORDER_SLUG,
      name: center ? "Olex Merkez" : "Olex Distribütör",
      role: "owner",
      logo_url: null,
      status: "active",
      access_ends_at: null,
      type: center ? "center" : "distributor",
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
        surname: "Orders",
        status: "active",
        is_super_admin: false,
        email_verified: true,
        locale: "en",
        timezone: "Europe/Istanbul",
      },
      roles: [],
      permissions: PERMISSIONS,
      grants: Object.fromEntries(PERMISSIONS.map((p) => [p, "managed"])),
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

  private view(): Json {
    if (!this.order) throw new Error("no order");
    const status = this.order.status as string;
    const assigned = this.units.reduce(
      (n, u) => n + Number(u.quantity ?? 0),
      0,
    );
    const lineTotal = (this.quantity * 80).toFixed(2);
    return {
      ...this.order,
      status_label: status[0].toUpperCase() + status.slice(1),
      role: this.side,
      subtotal: lineTotal,
      total: lineTotal,
      available_transitions: NEXT[this.side][status] ?? [],
      items: [
        {
          uuid: ITEM_UUID,
          product: PRODUCT,
          quantity: this.quantity,
          meters: null,
          unit_price: "80.0000",
          price_source: "list",
          line_total: lineTotal,
          note: null,
          assigned: String(assigned),
          units: this.units,
        },
      ],
      history: this.history,
    };
  }

  private move(to: string, reason: string | null = null) {
    if (!this.order) return;
    const from = this.order.status as string;
    const at = new Date().toISOString();
    this.history.push({
      from_status: from,
      to_status: to,
      reason,
      created_at: at,
    });
    this.order.status = to;
    this.order.updated_at = at;
    if (to === "submitted") this.order.submitted_at = at;
    if (to === "approved") {
      this.order.approved_at = at;
      this.order.rate_snapshot = {
        base: "EUR",
        quote: "TRY",
        rate: "35.1234",
        rate_date: "2026-10-01",
        source: "tcmb",
      };
      this.order.try_rate = "35.1234";
    }
    if (to === "shipped") {
      this.order.shipped_at = at;
      this.units = this.units.map((u) => ({ ...u, shipped: true }));
    }
  }

  async handle(route: Route) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api/, "");
    const method = req.method();
    this.calls.push(`${method} ${path}${url.search}`);
    const body = req.postData() ? (req.postDataJSON() as Json) : undefined;
    if (body !== undefined) {
      (this.bodies[`${method} ${path}`] ??= []).push(body);
    }
    const ok = (data: unknown, status = 200) =>
      route.fulfill({ status, json: envelope(data) });
    const conflict = (code: string) =>
      route.fulfill({
        status: 409,
        json: { success: false, error: { code, message: code } },
      });
    const ord = `/v1/orders/${ORDER_UUID}`;

    if (
      method === "GET" &&
      path === `/v1/public/organizations/by-slug/${ORDER_SLUG}`
    ) {
      return ok({
        uuid: ORG,
        slug: ORDER_SLUG,
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
        organization_type: this.membership().type,
        items: [],
        enabled: ["orders", "catalog"],
      });
    }
    if (method === "GET" && path === "/v1/me/organizations") {
      return ok({ items: [this.membership()] });
    }
    if (method === "GET" && path === "/v1/catalog/products") {
      const q = (url.searchParams.get("q") ?? "").toLowerCase();
      const items = PRODUCT.name.toLowerCase().includes(q) ? [PRODUCT] : [];
      return ok({ items, total: items.length, limit: 10, offset: 0 });
    }
    if (
      method === "GET" &&
      path === `/v1/tenant/pricing/products/${PRODUCT.uuid}`
    ) {
      return ok({
        product_uuid: PRODUCT.uuid,
        sku: PRODUCT.sku,
        name: PRODUCT.name,
        viewer: "distributor",
        prices: [
          {
            currency: "EUR",
            purchase_price: "80.0000",
            purchase_price_source: "list",
          },
        ],
      });
    }
    if (method === "POST" && path === "/v1/orders") {
      const items = (body?.items ?? []) as Json[];
      this.quantity = Number(items[0]?.quantity ?? 0);
      this.order = {
        uuid: ORDER_UUID,
        order_no: "ORD-00000007",
        status: "draft",
        seller: { uuid: "s", name: "Olex Merkez", type: "center" },
        buyer: { uuid: "b", name: "Olex Distribütör", type: "distributor" },
        currency: "EUR",
        tax_total: "0.00",
        rate_snapshot: null,
        try_rate: null,
        note: (body?.note as string | undefined) ?? null,
        cancel_reason: null,
        submitted_at: null,
        approved_at: null,
        ready_at: null,
        shipped_at: null,
        cancelled_at: null,
        created_at: NOW,
        updated_at: NOW,
      };
      this.history = [
        {
          from_status: null,
          to_status: "draft",
          reason: null,
          created_at: NOW,
        },
      ];
      return ok(this.view(), 201);
    }
    if (method === "GET" && path === "/v1/orders") {
      const side = url.searchParams.get("side");
      const visible = this.order && (side === null || side === this.side);
      return ok({
        items: visible
          ? [{ ...this.view(), items: undefined, history: undefined }]
          : [],
        total: visible ? 1 : 0,
        limit: 20,
        offset: 0,
      });
    }
    if (this.order && method === "GET" && path === ord) return ok(this.view());
    if (this.order && method === "POST" && path === `${ord}/transitions`) {
      const to = String(body?.status);
      const allowed = NEXT[this.side][this.order.status as string] ?? [];
      if (!allowed.includes(to)) return conflict("ORDER_INVALID_TRANSITION");
      if (to === "ready" && this.units.length === 0) {
        return conflict("ORDER_NOT_FULLY_ASSIGNED");
      }
      this.move(to, (body?.reason as string | undefined) ?? null);
      return ok(this.view());
    }
    if (
      this.order &&
      method === "POST" &&
      path === `${ord}/items/${ITEM_UUID}/units`
    ) {
      if (body?.barcode !== FIXED_BARCODE) {
        return conflict("ORDER_UNIT_NOT_AVAILABLE");
      }
      this.units.push({
        unit_uuid: "0b9c4c1e-0000-4000-8000-0000000000a9",
        barcode: FIXED_BARCODE,
        unit_kind: "fixed",
        quantity: Number(body?.quantity ?? 1),
        meters: null,
        shipped: false,
        assigned_at: new Date().toISOString(),
      });
      return ok(this.view());
    }

    this.unknown.push(`${method} ${path}`);
    return route.fulfill({
      status: 404,
      json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
    });
  }
}

/** Routes every BFF call except Auth.js itself to the order mock. */
export async function mockOrders(page: Page): Promise<OrderMock> {
  const api = new OrderMock();
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => api.handle(route),
  );
  return api;
}
