import type { Page, Route } from "@playwright/test";

/**
 * Mocked BFF for the TEC-763 stock operations e2e: a center organization
 * whose warehouse user scans a roll, opens its history, splits it and
 * requests a product fix; `actAs("approver")` switches /v1/auth/me to a
 * second center user who approves the request after a password step-up.
 */

export const OPS_SLUG = "olex";
const ORG = "0b9c4c1e-0000-4000-8000-000000000001";
export const REQUESTER = "0b9c4c1e-0000-4000-8000-000000000002";
export const APPROVER = "0b9c4c1e-0000-4000-8000-000000000003";
const NOW = "2026-10-16T09:00:00Z";
export const ROLL_BARCODE = "OLX-ROLL-001";
export const NEW_BARCODE = "OLX-ROLL-001-S1";
const ENTRY = "0b9c4c1e-0000-4000-8000-0000000000e1";
const RECLASS = "0b9c4c1e-0000-4000-8000-0000000000c1";

const PERMISSIONS = [
  "stock.read",
  "stock.write",
  "stock.reclassify",
  "warehouse.read",
  "catalog.read",
];

type Json = Record<string, unknown>;

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

const PRODUCT = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f2",
  sku: "PPF-190",
  name: "Olex PPF 190",
  unit_type: "roll_meter",
  uses_fixed_barcode: false,
};
export const TARGET = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f6",
  sku: "PPF-200",
  name: "Olex PPF 200 Matte",
  unit_type: "roll_meter",
  uses_fixed_barcode: false,
};
const CENTER = { uuid: ORG, name: "Olex Merkez", type: "center" };
const LOCATION = {
  type: "warehouse_location",
  organization: CENTER,
  location: { uuid: "loc-1", code: "WH1-R1-A-S1-01", name: "Bin 01" },
  masked: false,
};

export class StockOpsMock {
  user = REQUESTER;
  remaining = "50.00";
  stepUpVerified = false;
  calls: string[] = [];
  bodies: Record<string, unknown[]> = {};
  unknown: string[] = [];
  reclass: Json | null = null;

  actAs(who: "requester" | "approver") {
    this.user = who === "approver" ? APPROVER : REQUESTER;
    this.stepUpVerified = false;
  }

  private membership() {
    return {
      uuid: ORG,
      slug: OPS_SLUG,
      name: CENTER.name,
      role: "staff",
      logo_url: null,
      status: "active",
      access_ends_at: null,
      type: "center",
      brand: { slug: "olex", name: "Olex" },
      parent: null,
    };
  }

  private me() {
    const approver = this.user === APPROVER;
    return {
      effective_locale: "en",
      effective_timezone: "Europe/Istanbul",
      user: {
        uuid: this.user,
        email: approver ? "approver@example.com" : "e2e@example.com",
        name: approver ? "Ayla" : "Deniz",
        surname: approver ? "Onay" : "Depo",
        status: "active",
        is_super_admin: false,
        email_verified: true,
        locale: "en",
        timezone: "Europe/Istanbul",
      },
      roles: [],
      permissions: PERMISSIONS,
      grants: Object.fromEntries(PERMISSIONS.map((p) => [p, "all"])),
      active_organization_uuid: ORG,
      organization_roles: ["staff"],
      organizations: [this.membership()],
      links: {
        profile: "/v1/auth/profile",
        change_password: "/v1/auth/password/change",
        notification_preferences: "/v1/notification-preferences",
      },
      channels: { user: `user:${this.user}` },
      realtime: { enabled: false, user_channel: `user:${this.user}` },
    };
  }

  private unit(barcode: string) {
    const isNew = barcode === NEW_BARCODE;
    return {
      uuid: isNew ? "u-new" : "u-roll",
      barcode,
      unit_kind: "serial",
      source: "generated",
      status: "placed",
      initial_meters: isNew ? "12.50" : "50.00",
      remaining_meters: isNew ? "12.50" : this.remaining,
      product: PRODUCT,
      created_at: NOW,
    };
  }

  private history(barcode: string) {
    const movements: Json[] = [
      {
        uuid: "m-1",
        type: "entry",
        quantity_delta: 1,
        meters_delta: "50.00",
        from_status: "printed",
        to_status: "available",
        from_owner: null,
        to_owner: { ...LOCATION, location: null, type: "organization" },
        organization: CENTER,
        organization_masked: false,
        reference_type: "stock_entry_line",
        document: { type: "stock_entry", uuid: ENTRY },
        actor: { uuid: REQUESTER, name: "Deniz Depo" },
        reason: null,
        created_at: "2026-10-15T08:00:00Z",
      },
      {
        uuid: "m-2",
        type: "placement",
        quantity_delta: 0,
        meters_delta: "0.00",
        from_status: "available",
        to_status: "placed",
        from_owner: { ...LOCATION, location: null, type: "organization" },
        to_owner: LOCATION,
        organization: CENTER,
        organization_masked: false,
        reference_type: "stock_entry_line",
        document: { type: "stock_entry", uuid: ENTRY },
        actor: { uuid: REQUESTER, name: "Deniz Depo" },
        reason: null,
        created_at: "2026-10-15T08:05:00Z",
      },
    ];
    if (barcode === ROLL_BARCODE && this.remaining !== "50.00") {
      movements.push({
        ...movements[1],
        uuid: "m-3",
        type: "split",
        meters_delta: `-${(50 - Number(this.remaining)).toFixed(2)}`,
        from_status: "placed",
        from_owner: LOCATION,
        reference_type: "stock_split",
        document: null,
        created_at: NOW,
      });
    }
    return {
      unit: this.unit(barcode),
      current: {
        owner: LOCATION,
        holder: CENTER,
        status: "placed",
        updated_at: NOW,
      },
      holdings: [],
      movements: {
        items: movements,
        total: movements.length,
        limit: 100,
        offset: 0,
      },
    };
  }

  private body(key: string, value: unknown) {
    (this.bodies[key] ??= []).push(value);
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
    const json = () => {
      try {
        return req.postDataJSON() as Json;
      } catch {
        return {};
      }
    };

    if (
      method === "GET" &&
      path === `/v1/public/organizations/by-slug/${OPS_SLUG}`
    ) {
      return ok({
        uuid: ORG,
        slug: OPS_SLUG,
        name: CENTER.name,
        status: "active",
        logo_url: null,
        access_ok: true,
      });
    }
    if (method === "GET" && path === "/v1/auth/me") return ok(this.me());
    if (method === "GET" && path === "/v1/auth/step-up") {
      return ok(
        this.stepUpVerified
          ? { valid: true, expires_at: NOW, methods: ["password"] }
          : { valid: false, expires_at: null, methods: ["password"] },
      );
    }
    if (method === "POST" && path === "/v1/auth/step-up/password") {
      this.body("step-up", json());
      this.stepUpVerified = true;
      return ok({ valid: true, expires_at: NOW, method: "password" });
    }
    if (method === "GET" && path === "/v1/features") {
      return ok({
        organization_type: "center",
        items: [],
        enabled: ["stock", "warehouse", "catalog"],
      });
    }
    if (method === "GET" && path === "/v1/me/organizations") {
      return ok({ items: [this.membership()] });
    }

    // Universal scan (TEC-203): the roll's QR.
    if (method === "POST" && path === "/v1/warehouse/scan") {
      const u = this.unit(ROLL_BARCODE);
      return ok({
        status: "ok",
        type: "unit",
        matched_by: "unit_qr",
        code: ROLL_BARCODE,
        location: null,
        product: { ...PRODUCT, active: true },
        unit: {
          uuid: u.uuid,
          barcode: u.barcode,
          unit_kind: "serial",
          status: "placed",
          initial_meters: u.initial_meters,
          remaining_meters: u.remaining_meters,
          owner_type: "warehouse_location",
          quantity_on_hand: null,
          holder: CENTER,
          location: null,
        },
      });
    }

    const history = path.match(/^\/v1\/stock\/units\/by-barcode\/([^/]+)$/);
    if (method === "GET" && history) {
      return ok(this.history(decodeURIComponent(history[1])));
    }

    if (method === "POST" && path === "/v1/stock/splits") {
      const body = json();
      this.body("split", body);
      const cut = Number(body.meters);
      this.remaining = (Number(this.remaining) - cut).toFixed(2);
      return ok(
        {
          uuid: "s-1",
          meters: cut.toFixed(2),
          source: {
            uuid: "u-roll",
            barcode: ROLL_BARCODE,
            status: "placed",
            remaining_meters: this.remaining,
          },
          new_unit: {
            uuid: "u-new",
            barcode: NEW_BARCODE,
            status: "placed",
            remaining_meters: cut.toFixed(2),
            label_url: `/v1/stock/labels/units.pdf?barcode=${NEW_BARCODE}`,
          },
          replayed: false,
          created_at: NOW,
        },
        201,
      );
    }
    if (method === "GET" && path === "/v1/stock/labels/units.pdf") {
      this.body("labels", url.searchParams.getAll("barcode"));
      return route.fulfill({
        status: 200,
        contentType: "application/pdf",
        headers: {
          "content-disposition": `attachment; filename="${url.searchParams.get("barcode")}.pdf"`,
        },
        body: "%PDF-1.4\n%e2e\n",
      });
    }

    if (method === "GET" && path === "/v1/catalog/products") {
      return page([
        { ...PRODUCT, active: true },
        { ...TARGET, active: true },
      ]);
    }

    if (method === "POST" && path === "/v1/stock/reclassifications") {
      const body = json();
      this.body("reclass", body);
      this.reclass = {
        uuid: RECLASS,
        status: "pending",
        unit: { uuid: "u-roll", barcode: body.barcode },
        organization: CENTER,
        from_product: PRODUCT,
        to_product: TARGET,
        reason: body.reason,
        requested_by: { uuid: REQUESTER, name: "Deniz Depo" },
        decided_by: null,
        decided_at: null,
        decision_note: null,
        movement_uuid: null,
        created_at: NOW,
      };
      return ok(this.reclass, 201);
    }
    if (method === "GET" && path === "/v1/stock/reclassifications") {
      return page(this.reclass ? [this.reclass] : []);
    }
    if (method === "GET" && path === `/v1/stock/reclassifications/${RECLASS}`) {
      return this.reclass
        ? ok(this.reclass)
        : route.fulfill({
            status: 404,
            json: { error: { code: "NOT_FOUND", message: "Not found" } },
          });
    }
    if (
      method === "POST" &&
      path === `/v1/stock/reclassifications/${RECLASS}/approve`
    ) {
      this.body("approve", { user: this.user, stepUp: this.stepUpVerified });
      if (!this.stepUpVerified) {
        return route.fulfill({
          status: 403,
          json: {
            error: { code: "STEP_UP_REQUIRED", message: "Step-up required" },
          },
        });
      }
      this.reclass = {
        ...this.reclass,
        status: "approved",
        decided_by: { uuid: APPROVER, name: "Ayla Onay" },
        decided_at: NOW,
        movement_uuid: "m-9",
      };
      return ok(this.reclass);
    }

    this.unknown.push(`${method} ${path}`);
    return route.fulfill({
      status: 404,
      json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
    });
  }
}

/** Routes every BFF call except Auth.js itself to the stock ops mock. */
export async function mockStockOps(page: Page): Promise<StockOpsMock> {
  const api = new StockOpsMock();
  await page.route(
    (url) =>
      url.hostname === "127.0.0.1" &&
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => api.handle(route),
  );
  return api;
}
