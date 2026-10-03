import { expect, test, type Page, type Route } from "@playwright/test";

import { MockApi, ORG, SERVICE_UUID, USER, signIn } from "./support/mock-api";
import {
  FIXED_BARCODE,
  ORDER_SLUG,
  ORDER_UUID,
  OrderMock,
  PRODUCT,
} from "./support/order-mock";
import { StockMock } from "./support/stock-mock";

/**
 * TEC-225 (TEC-104 acceptance): the dealer receives a shipped order, the
 * received units show up on "My stock", and the service wizard's stock
 * picker offers them. The three existing mocks (orders, stock, services)
 * share one dealer session; receiving the order moves its assigned units
 * into the dealer's stock, as the Go API does (order received → stock
 * movement into the buyer organization).
 */

const SHIPPED_AT = "2026-10-01T12:00:00Z";
const RECEIVED_QTY = 2;
const UNIT_UUID = "0b9c4c1e-0000-4000-8000-0000000000a9";

const PERMISSIONS = [
  "orders.read",
  "orders.receive",
  "stock.read",
  "services.read",
  "services.write",
  "services.complete",
  "customers.read",
  "vehicles.read",
  "catalog.read",
];

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

const membership = {
  uuid: ORG,
  slug: ORDER_SLUG,
  name: "Acme Bayi",
  role: "owner",
  logo_url: null,
  status: "active",
  access_ends_at: null,
  type: "dealer",
  brand: { slug: "olex", name: "Olex" },
  parent: null,
};

function me() {
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
    permissions: PERMISSIONS,
    grants: Object.fromEntries(PERMISSIONS.map((p) => [p, "organization"])),
    active_organization_uuid: ORG,
    organization_roles: ["owner"],
    organizations: [membership],
    links: {
      profile: "/v1/auth/profile",
      change_password: "/v1/auth/password/change",
      notification_preferences: "/v1/notification-preferences",
    },
    channels: { user: `user:${USER}` },
    realtime: { enabled: false, user_channel: `user:${USER}` },
  };
}

/** One dealer session over the order, stock and service mocks. */
class DealerFlow {
  orders = new OrderMock();
  stock = new StockMock();
  services = new MockApi();
  private received = false;

  constructor() {
    // The dealer holds nothing yet; the order is on its way.
    this.stock.units = { [ORG]: [] };
    this.stock.products = { [ORG]: [] };
    this.services.stockUnits = [];
    this.services.seedDraft(1000);
    this.orders.actAs("buyer");
    this.orders.quantity = RECEIVED_QTY;
    this.orders.units = [
      {
        unit_uuid: UNIT_UUID,
        barcode: FIXED_BARCODE,
        unit_kind: "fixed",
        quantity: RECEIVED_QTY,
        meters: null,
        shipped: true,
        assigned_at: SHIPPED_AT,
      },
    ];
    this.orders.order = {
      uuid: ORDER_UUID,
      order_no: "ORD-00000007",
      status: "shipped",
      seller: { uuid: "s", name: "Olex Distribütör", type: "distributor" },
      buyer: { uuid: ORG, name: "Acme Bayi", type: "dealer" },
      currency: "EUR",
      tax_total: "0.00",
      rate_snapshot: {
        base: "EUR",
        quote: "TRY",
        rate: "35.1234",
        rate_date: "2026-10-01",
        source: "tcmb",
      },
      try_rate: "35.1234",
      note: null,
      cancel_reason: null,
      submitted_at: "2026-10-01T09:00:00Z",
      approved_at: "2026-10-01T10:00:00Z",
      ready_at: "2026-10-01T11:00:00Z",
      shipped_at: SHIPPED_AT,
      cancelled_at: null,
      created_at: "2026-10-01T09:00:00Z",
      updated_at: SHIPPED_AT,
    };
    this.orders.history = (
      [
        [null, "draft"],
        ["draft", "submitted"],
        ["submitted", "approved"],
        ["approved", "preparing"],
        ["preparing", "ready"],
        ["ready", "shipped"],
      ] as const
    ).map(([from_status, to_status]) => ({
      from_status,
      to_status,
      reason: null,
      created_at: SHIPPED_AT,
    }));
  }

  /** Received: the shipped units land in the dealer's stock. */
  private syncReceived() {
    if (this.received || this.orders.order?.status !== "received") return;
    this.received = true;
    for (const u of this.orders.units) {
      this.stock.units[ORG].push({
        uuid: u.unit_uuid,
        barcode: u.barcode,
        unit_kind: u.unit_kind,
        status: "available",
        quantity: u.quantity,
        initial_meters: null,
        remaining_meters: null,
        product: { ...PRODUCT, uses_fixed_barcode: true },
        location: null,
        purchase_price: null,
        updated_at: new Date().toISOString(),
      });
      this.services.stockUnits.push({
        uuid: u.unit_uuid,
        barcode: u.barcode,
        unit_kind: u.unit_kind,
        product: { ...PRODUCT, available_parts: [] },
        quantity_on_hand: u.quantity,
        initial_meters: null,
        remaining_meters: null,
      });
    }
  }

  async handle(route: Route) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api/, "");
    const method = req.method();
    const ok = (data: unknown) =>
      route.fulfill({ status: 200, json: envelope(data) });

    // Shell: one dealer membership with every module of the flow.
    if (method === "GET") {
      if (path === `/v1/public/organizations/by-slug/${ORDER_SLUG}`) {
        return ok({
          uuid: ORG,
          slug: ORDER_SLUG,
          name: membership.name,
          status: "active",
          logo_url: null,
          access_ok: true,
        });
      }
      if (path === "/v1/auth/me") return ok(me());
      if (path === "/v1/auth/step-up") {
        return ok({ valid: false, expires_at: null, methods: [] });
      }
      if (path === "/v1/features") {
        return ok({
          organization_type: "dealer",
          items: [],
          enabled: ["orders", "stock", "services", "catalog"],
        });
      }
      if (path === "/v1/me/organizations") {
        return ok({ items: [membership] });
      }
    }

    if (
      path.startsWith("/v1/orders") ||
      path === "/v1/catalog/products" ||
      path.startsWith("/v1/tenant/pricing/")
    ) {
      await this.orders.handle(route);
      this.syncReceived();
      return;
    }
    if (
      path.startsWith("/v1/stock/") ||
      path.startsWith("/v1/search/") ||
      path === "/v1/tenant/organizations"
    ) {
      return this.stock.handle(route);
    }
    return this.services.handle(route);
  }
}

async function mockDealerFlow(page: Page): Promise<DealerFlow> {
  const api = new DealerFlow();
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => api.handle(route),
  );
  return api;
}

/** Opens the draft's wizard and walks to the stock step (hood picked). */
async function openStockStep(page: Page) {
  await page.goto(`/t/${ORDER_SLUG}/services/${SERVICE_UUID}/wizard`);
  await expect(page.getByTestId("parts-step")).toBeVisible();
  await page.locator('path[data-part="body_kaput"]').click();
  await page.getByTestId("parts-next").click();
  await expect(page.getByTestId("measurement-step")).toBeVisible();
  await page.getByTestId("measurement-no").click();
  await page.getByTestId("measurement-save").click();
  await expect(page.getByTestId("stock-step")).toBeVisible();
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

test("dealer: receive order → my stock → service stock picker", async ({
  page,
}) => {
  const api = await mockDealerFlow(page);

  // Before receiving, nothing is on hand.
  await page.goto(`/t/${ORDER_SLUG}/stock`);
  await expect(page.getByRole("heading", { name: "My stock" })).toBeVisible();
  await expect(page.getByTestId("stock-empty")).toBeVisible();
  await expect(page.getByTestId("stock-row")).toHaveCount(0);

  // 1. The dealer marks the shipped order as received.
  await page.goto(`/t/${ORDER_SLUG}/orders/${ORDER_UUID}`);
  await expect(
    page.getByRole("heading", { name: "ORD-00000007" }),
  ).toBeVisible();
  await expect(page.locator('[data-action="receive"]')).toBeVisible();
  await page.locator('[data-action="receive"]').click();
  await page.getByTestId("action-confirm").click();
  await expect(page.locator("[data-action]")).toHaveCount(0);
  await expect(page.getByTestId("history-row")).toHaveCount(7);
  expect(
    api.orders.bodies[`POST /v1/orders/${ORDER_UUID}/transitions`],
  ).toEqual([{ status: "received" }]);

  // 2. The received units are on "My stock".
  await page.goto(`/t/${ORDER_SLUG}/stock`);
  const rows = page.getByTestId("stock-row");
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toContainText(FIXED_BARCODE);
  await expect(rows.first()).toContainText(PRODUCT.name);
  await expect(rows.first()).toContainText(String(RECEIVED_QTY));
  expect(
    api.stock.calls.some((c) =>
      c.startsWith(`GET /v1/stock/organizations/${ORG}/units`),
    ),
  ).toBe(true);

  // 3. The service wizard's stock picker offers them.
  await openStockStep(page);
  const units = page.getByTestId("stock-units").locator("li");
  await expect(units).toHaveCount(1);
  const unit = page.locator(`[data-unit="${FIXED_BARCODE}"]`);
  await expect(unit).toContainText(PRODUCT.name);
  await expect(unit).toContainText(String(RECEIVED_QTY));

  await unit.getByTestId("pick-unit").click();
  const form = page.getByTestId("add-unit-form");
  await expect(form).toContainText(PRODUCT.name);
  const qty = form.locator('input[name="quantity"]');
  await qty.fill(String(RECEIVED_QTY + 1));
  await expect(page.getByTestId("quantity-error")).toHaveText(
    `Only ${RECEIVED_QTY} pcs are on hand.`,
  );
  await expect(page.getByTestId("add-unit-submit")).toBeDisabled();
  await qty.fill(String(RECEIVED_QTY));
  await page.getByTestId("add-unit-submit").click();
  await expect(page.getByTestId("service-item")).toHaveCount(1);
  await expect(page.getByTestId("service-item")).toContainText(FIXED_BARCODE);
  expect(
    api.services.bodies[`POST /v1/services/${SERVICE_UUID}/items`]?.[0],
  ).toMatchObject({
    barcode: FIXED_BARCODE,
    kind: "full",
    quantity: RECEIVED_QTY,
  });

  // Every call of the flow was answered by a fixture.
  const flow = /\/v1\/(orders|stock|services)/;
  expect(
    [
      ...api.orders.unknown,
      ...api.stock.unknown,
      ...api.services.unknown,
    ].filter((c) => flow.test(c)),
  ).toEqual([]);
});
