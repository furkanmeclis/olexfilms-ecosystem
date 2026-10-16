import { expect, test, type Page, type Route } from "@playwright/test";

import { signIn } from "./support/mock-api";

/**
 * TEC-765: a distributor shelves the units of a received order against a
 * mocked BFF — the order detail shows the "Place on shelf" panel with the
 * unplaced count, the user scans the target location QR, scans two units,
 * places them, and the panel turns to "All units are placed". The fixed
 * barcode of the order is explained, not placed.
 */

const SLUG = "acme";
const ORG = "0b9c4c1e-0000-4000-8000-000000000001";
const USER = "0b9c4c1e-0000-4000-8000-000000000002";
const ORDER = "0b9c4c1e-0000-4000-8000-000000000765";
const LOCATION = "0b9c4c1e-0000-4000-8000-0000000007c1";
const NOW = "2026-10-16T09:00:00Z";
const PRODUCT = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000007f1",
  sku: "PPF-190",
  name: "PPF Gloss 190",
  unit_type: "roll_meter",
};
const BARCODES = ["OLX-00000101", "OLX-00000102"];
const PERMISSIONS = [
  "orders.read",
  "orders.receive",
  "warehouse.read",
  "warehouse.write",
];

type Json = Record<string, unknown>;
const envelope = (data: unknown) => ({ success: true, data, meta: {} });

class PlacementMock {
  placed = new Set<string>();
  bodies: Json[] = [];

  private membership() {
    return {
      uuid: ORG,
      slug: SLUG,
      name: "Olex Distribütör",
      role: "owner",
      logo_url: null,
      status: "active",
      access_ends_at: null,
      type: "distributor",
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
        surname: "Warehouse",
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

  private unit(barcode: string, i: number) {
    return {
      unit_uuid: `0b9c4c1e-0000-4000-8000-00000000076${i}`,
      barcode,
      unit_kind: "serial",
      product: { uuid: PRODUCT.uuid, sku: PRODUCT.sku, name: PRODUCT.name },
      meters: "25.00",
      quantity: null,
      unplaced: !this.placed.has(barcode),
    };
  }

  private placement() {
    const units = BARCODES.map((b, i) => this.unit(b, i));
    const unplaced = units.filter((u) => u.unplaced).length;
    return {
      order_uuid: ORDER,
      order_no: "ORD-00000765",
      status: "received",
      unplaced_count: unplaced,
      placed_count: units.length - unplaced,
      units,
      fixed: [
        {
          unit_uuid: "0b9c4c1e-0000-4000-8000-0000000007f9",
          barcode: "OLX-FIX-765",
          unit_kind: "fixed",
          product: { uuid: "f", sku: "KIT-001", name: "Olex Bakım Kiti" },
          meters: null,
          quantity: 4,
          unplaced: false,
        },
      ],
    };
  }

  private order() {
    return {
      uuid: ORDER,
      order_no: "ORD-00000765",
      status: "received",
      status_label: "Received",
      role: "buyer",
      seller: { uuid: "s", name: "Olex Merkez", type: "center" },
      buyer: { uuid: ORG, name: "Olex Distribütör", type: "distributor" },
      currency: "EUR",
      subtotal: "200.00",
      tax_total: "0.00",
      total: "200.00",
      rate_snapshot: null,
      try_rate: null,
      note: null,
      cancel_reason: null,
      submitted_at: NOW,
      approved_at: NOW,
      ready_at: NOW,
      shipped_at: NOW,
      cancelled_at: null,
      created_at: NOW,
      updated_at: NOW,
      available_transitions: [],
      items: [
        {
          uuid: "0b9c4c1e-0000-4000-8000-0000000007a1",
          product: PRODUCT,
          quantity: null,
          meters: "50.00",
          unit_price: "4.0000",
          price_source: "list",
          line_total: "200.00",
          note: null,
          assigned: "50.00",
          units: BARCODES.map((b, i) => ({
            unit_uuid: `0b9c4c1e-0000-4000-8000-00000000076${i}`,
            barcode: b,
            unit_kind: "serial",
            quantity: 1,
            meters: "25.00",
            shipped: true,
            assigned_at: NOW,
          })),
        },
      ],
      history: [],
    };
  }

  async handle(route: Route) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api/, "");
    const method = req.method();
    const body = req.postData() ? (req.postDataJSON() as Json) : undefined;
    const ok = (data: unknown) =>
      route.fulfill({ status: 200, json: envelope(data) });

    if (
      method === "GET" &&
      path === `/v1/public/organizations/by-slug/${SLUG}`
    ) {
      return ok({
        uuid: ORG,
        slug: SLUG,
        name: "Olex Distribütör",
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
        organization_type: "distributor",
        items: [],
        enabled: ["orders", "warehouse"],
      });
    }
    if (method === "GET" && path === "/v1/me/organizations") {
      return ok({ items: [this.membership()] });
    }
    if (method === "GET" && path === `/v1/orders/${ORDER}`) {
      return ok(this.order());
    }
    if (method === "GET" && path === `/v1/warehouse/orders/${ORDER}/unplaced`) {
      return ok(this.placement());
    }
    if (method === "GET" && path === "/v1/warehouse/warehouses") {
      return ok({
        items: [
          {
            uuid: "0b9c4c1e-0000-4000-8000-0000000007d1",
            code: "D1",
            name: "Distribütör deposu",
            active: true,
          },
        ],
      });
    }
    if (method === "POST" && path === "/v1/warehouse/scan") {
      const code = String(body?.code ?? "");
      if (code !== "OFW:LOC:D1-R1-A-01") {
        return route.fulfill({
          status: 404,
          json: {
            success: false,
            error: { code: "SCAN_LOCATION_NOT_FOUND", message: "not found" },
          },
        });
      }
      return ok({
        type: "location",
        matched_by: "location_qr",
        code: "D1-R1-A-01",
        location: {
          uuid: LOCATION,
          type: "bin",
          code: "01",
          full_code: "D1-R1-A-01",
          name: "Göz 01",
          active: true,
          path: [],
        },
        unit: null,
        product: null,
      });
    }
    if (method === "POST" && path === `/v1/warehouse/orders/${ORDER}/place`) {
      this.bodies.push(body ?? {});
      const codes = (body?.barcodes ?? []) as string[];
      codes.forEach((c) => this.placed.add(c));
      return ok({
        location: { uuid: LOCATION, code: "01", full_code: "D1-R1-A-01" },
        units: codes.map((c, i) => ({
          unit_uuid: `u-${i}`,
          barcode: c,
          from_location: null,
          movement_uuid: `m-${i}`,
        })),
      });
    }
    if (path.startsWith("/v1/realtime") || path.startsWith("/v1/search")) {
      return route.fulfill({ status: 404, json: { success: false } });
    }
    return route.fulfill({
      status: 404,
      json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
    });
  }
}

async function mockPlacement(page: Page) {
  const api = new PlacementMock();
  await page.route(
    (url) =>
      url.hostname === "127.0.0.1" &&
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => api.handle(route),
  );
  return api;
}

test.beforeEach(async ({ context, baseURL }) => {
  await signIn(context, baseURL ?? "");
});

async function scan(page: Page, id: string, code: string) {
  await page.getByTestId(`${id}-input`).fill(code);
  await page.getByTestId(`${id}-input`).press("Enter");
}

test("order placement: received order → scan bin → scan units → place → done", async ({
  page,
}) => {
  const api = await mockPlacement(page);
  await page.goto(`/t/${SLUG}/orders/${ORDER}`);

  const panel = page.getByTestId("order-placement");
  await expect(panel).toBeVisible();
  await expect(panel.getByTestId("order-unplaced-badge")).toHaveText(
    "2 units not placed",
  );
  await expect(panel.getByTestId("placement-unit")).toHaveCount(2);
  await expect(panel.getByTestId("placement-fixed-note")).toContainText(
    "Fixed barcode products are not placed in this step",
  );
  await expect(panel.getByTestId("placement-submit")).toBeDisabled();

  // A location of another warehouse is explained.
  await scan(page, "placement-location", "OFW:LOC:OTHER-01");
  await expect(panel.getByTestId("placement-error")).toContainText(
    "Location not found",
  );
  await scan(page, "placement-location", "OFW:LOC:D1-R1-A-01");
  await expect(panel.getByTestId("placement-target")).toContainText(
    "D1-R1-A-01",
  );
  await expect(panel.getByTestId("placement-error")).toHaveCount(0);

  await scan(page, "placement-unit", "OLX-99999999");
  await expect(panel.getByTestId("placement-error")).toContainText(
    "is not on this order",
  );
  await scan(page, "placement-unit", `OFW:UNIT:${BARCODES[0]}`);
  await scan(page, "placement-unit", BARCODES[1]);
  await expect(panel.getByTestId("placement-submit")).toHaveText(
    /Place selected \(2\)/,
  );
  await panel.getByTestId("placement-submit").click();

  await expect(panel.getByTestId("order-placement-done")).toHaveText(
    "All units are placed (2).",
  );
  await expect(panel.getByTestId("order-unplaced-badge")).toHaveCount(0);
  expect(api.bodies).toEqual([{ location_uuid: LOCATION, barcodes: BARCODES }]);
});
