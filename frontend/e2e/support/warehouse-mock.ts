import type { Page, Route } from "@playwright/test";

/**
 * Mocked BFF for the warehouse e2e (TEC-231): the TEC-201 tree, the
 * TEC-203 scan and the TEC-204 stock entries of a center organization,
 * kept in memory so a location created in the UI is the one the stock
 * entry places on. Printed labels in `printed` may be linked to an entry.
 */

export const WAREHOUSE_SLUG = "acme";
const ORG = "0b9c4c1e-0000-4000-8000-000000000001";
const USER = "0b9c4c1e-0000-4000-8000-000000000002";
const NOW = "2026-10-01T09:00:00Z";

const PERMISSIONS = [
  "warehouse.read",
  "warehouse.write",
  "stock.read",
  "stock.write",
  "catalog.read",
];

type Json = Record<string, unknown>;
type Node = Json & { uuid: string; code: string; full_code: string };

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

export const PRODUCT = {
  uuid: "0b9c4c1e-0000-4000-8000-0000000000f2",
  sku: "PPF-190",
  name: "Olex PPF 190",
};

export class WarehouseMock {
  type: "center" | "distributor" = "center";
  warehouses: Node[] = [];
  rooms: Node[] = [];
  locations: Node[] = [];
  entries: Json[] = [];
  /** TEC-232: warehouse transfers and where each confirmed unit sits. */
  transfers: Json[] = [];
  placed: Record<string, Node> = {};
  /** Printed labels waiting for a stock entry (barcode -> unit uuid). */
  printed: Record<string, string> = {
    "OLEX-00000001": "0b9c4c1e-0000-4000-8000-0000000000u1",
  };
  calls: string[] = [];
  bodies: Record<string, unknown[]> = {};
  unknown: string[] = [];
  private seq = 0;

  private id(prefix: string) {
    this.seq += 1;
    return `0b9c4c1e-0000-4000-8000-${prefix}${String(this.seq).padStart(
      12 - prefix.length,
      "0",
    )}`;
  }

  private membership() {
    return {
      uuid: ORG,
      slug: WAREHOUSE_SLUG,
      name: "Olex Merkez",
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
        surname: "Warehouse",
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

  private entryView(e: Json) {
    const lines = (e.lines as Json[]) ?? [];
    return {
      ...e,
      line_count: lines.length,
      placed_count: lines.filter((l) => l.location).length,
    };
  }

  private transferView(x: Json, withLines: boolean) {
    const lines = (x.lines as Json[]) ?? [];
    const v: Json = { ...x, line_count: lines.length };
    if (!withLines) delete v.lines;
    return v;
  }

  async handle(route: Route) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api/, "");
    const method = req.method();
    this.calls.push(`${method} ${path}${url.search}`);
    const body = (req.postDataJSON() ?? {}) as Json;
    if (method !== "GET") {
      (this.bodies[`${method} ${path}`] ??= []).push(body);
    }
    const ok = (data: unknown, status = 200) =>
      route.fulfill({ status, json: envelope(data) });
    const fail = (status: number, code: string) =>
      route.fulfill({ status, json: { error: { code, message: code } } });

    if (
      method === "GET" &&
      path === `/v1/public/organizations/by-slug/${WAREHOUSE_SLUG}`
    ) {
      return ok({
        uuid: ORG,
        slug: WAREHOUSE_SLUG,
        name: "Olex Merkez",
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
        enabled: ["warehouse", "stock", "catalog"],
      });
    }
    if (method === "GET" && path === "/v1/me/organizations") {
      return ok({ items: [this.membership()] });
    }
    if (method === "GET" && path === "/v1/search/specs") {
      return ok({ items: [], enabled: false });
    }

    // --- Tree (TEC-201) ---
    if (path === "/v1/warehouse/warehouses") {
      if (method === "GET") return ok({ items: this.warehouses });
      const code = String(body.code).toUpperCase();
      if (this.warehouses.some((w) => w.code === code)) {
        return fail(409, "WAREHOUSE_CODE_TAKEN");
      }
      const w: Node = {
        uuid: this.id("a"),
        code,
        full_code: code,
        name: (body.name as string) || code,
        address: body.address ?? null,
        active: true,
        sort_order: this.warehouses.length,
        created_at: NOW,
        updated_at: NOW,
      };
      this.warehouses.push(w);
      return ok(w, 201);
    }
    const rooms = path.match(/^\/v1\/warehouse\/warehouses\/([^/]+)\/rooms$/);
    if (rooms) {
      const wh = this.warehouses.find((w) => w.uuid === rooms[1]);
      if (!wh) return fail(404, "NOT_FOUND");
      if (method === "GET") {
        return ok({
          items: this.rooms.filter((r) => r.warehouse_uuid === wh.uuid),
        });
      }
      const code = String(body.code).toUpperCase();
      const r: Node = {
        uuid: this.id("b"),
        warehouse_uuid: wh.uuid,
        code,
        full_code: `${wh.code}-${code}`,
        name: (body.name as string) || code,
        active: true,
        sort_order: 0,
        created_at: NOW,
        updated_at: NOW,
      };
      this.rooms.push(r);
      return ok(r, 201);
    }
    const roomLocs = path.match(/^\/v1\/warehouse\/rooms\/([^/]+)\/locations$/);
    if (method === "GET" && roomLocs) {
      return ok({
        items: this.locations.filter((l) => l.room_uuid === roomLocs[1]),
      });
    }
    if (method === "POST" && path === "/v1/warehouse/locations") {
      const room = this.rooms.find((r) => r.uuid === body.room_uuid);
      if (!room) return fail(404, "NOT_FOUND");
      const parent = body.parent_uuid
        ? this.locations.find((l) => l.uuid === body.parent_uuid)
        : null;
      const allowed: Record<string, string[]> = {
        root: ["aisle", "shelf"],
        aisle: ["shelf"],
        shelf: ["bin"],
        bin: [],
      };
      if (
        !allowed[parent ? String(parent.type) : "root"].includes(
          String(body.type),
        )
      ) {
        return fail(400, "VALIDATION_ERROR");
      }
      const code = String(body.code).toUpperCase();
      const l: Node = {
        uuid: this.id("c"),
        warehouse_uuid: room.warehouse_uuid,
        room_uuid: room.uuid,
        parent_uuid: parent?.uuid ?? null,
        type: body.type,
        code,
        full_code: `${parent ? parent.full_code : room.full_code}-${code}`,
        name: (body.name as string) || code,
        active: true,
        sort_order: 0,
        created_at: NOW,
        updated_at: NOW,
      };
      this.locations.push(l);
      return ok(l, 201);
    }

    // --- Stock entries (TEC-204) ---
    if (path === "/v1/warehouse/stock-entries") {
      if (method === "GET") {
        const items = this.entries.map((e) => {
          const v = this.entryView(e) as Json;
          delete v.lines;
          return v;
        });
        return ok({ items, total: items.length, limit: 20, offset: 0 });
      }
      const wh = this.warehouses.find((w) => w.uuid === body.warehouse_uuid);
      if (!wh) return fail(400, "VALIDATION_ERROR");
      const e: Json = {
        uuid: this.id("d"),
        mode: body.mode,
        status: "draft",
        note: body.note ?? null,
        warehouse: { uuid: wh.uuid, code: wh.code, name: wh.name },
        import_batch_uuid: null,
        lines: [],
        label_batches: [],
        created_at: NOW,
        confirmed_at: null,
        cancelled_at: null,
      };
      this.entries.unshift(e);
      return ok(this.entryView(e), 201);
    }
    const entryPath = path.match(
      /^\/v1\/warehouse\/stock-entries\/([^/]+)(?:\/(lines|place|confirm|cancel))?$/,
    );
    if (entryPath) {
      const e = this.entries.find((x) => x.uuid === entryPath[1]);
      if (!e) return fail(404, "NOT_FOUND");
      const lines = e.lines as Json[];
      const action = entryPath[2];
      if (method === "GET" && !action) return ok(this.entryView(e));
      if (method === "POST" && action === "lines") {
        for (const raw of (body.barcodes as string[]) ?? []) {
          const barcode = raw.replace(/^OFW:UNIT:/, "");
          const unit = this.printed[barcode];
          if (!unit) return fail(409, "STOCK_ENTRY_UNIT_NOT_PRINTED");
          if (lines.some((l) => l.barcode === barcode)) {
            return fail(409, "STOCK_ENTRY_UNIT_TAKEN");
          }
          lines.push({
            uuid: this.id("e"),
            unit_uuid: unit,
            barcode,
            unit_kind: "serial",
            unit_status: "printed",
            quantity: 1,
            product: PRODUCT,
            location: null,
            label_url: `/v1/stock/labels/units.pdf?barcode=${barcode}`,
            entry_movement_uuid: null,
            placement_movement_uuid: null,
            undone: false,
          });
        }
        return ok(this.entryView(e));
      }
      if (method === "POST" && action === "place") {
        const code = String(body.location_code ?? "").replace(/^OFW:LOC:/, "");
        const target = body.location_uuid
          ? this.locations.find((l) => l.uuid === body.location_uuid)
          : this.locations.find((l) => l.full_code === code);
        if (!target) return fail(400, "VALIDATION_ERROR");
        const only = (body.line_uuids as string[]) ?? [];
        for (const l of lines) {
          if (only.length === 0 || only.includes(String(l.uuid))) {
            l.location = {
              uuid: target.uuid,
              code: target.code,
              full_code: target.full_code,
            };
          }
        }
        return ok(this.entryView(e));
      }
      if (method === "POST" && action === "confirm") {
        if (e.status !== "draft") return fail(409, "STOCK_ENTRY_NOT_DRAFT");
        if (lines.length === 0) return fail(409, "STOCK_ENTRY_EMPTY");
        if (lines.some((l) => !l.location))
          return fail(409, "STOCK_ENTRY_UNPLACED");
        e.status = "confirmed";
        e.confirmed_at = NOW;
        for (const l of lines) {
          l.unit_status = "placed";
          const loc = this.locations.find(
            (x) => x.uuid === (l.location as Json).uuid,
          );
          if (loc) this.placed[String(l.barcode)] = loc;
        }
        return ok(this.entryView(e));
      }
      if (method === "POST" && action === "cancel") {
        e.status = "cancelled";
        return ok(this.entryView(e));
      }
    }

    // --- Warehouse transfers (TEC-205, TEC-232 e2e) ---
    if (path === "/v1/warehouse/transfers") {
      if (method === "GET") {
        const items = this.transfers.map((x) => this.transferView(x, false));
        return ok({ items, total: items.length, limit: 20, offset: 0 });
      }
      const from = this.warehouses.find(
        (w) => w.uuid === body.from_warehouse_uuid,
      );
      const to = this.warehouses.find((w) => w.uuid === body.to_warehouse_uuid);
      if (!from || !to || from.uuid === to.uuid) {
        return fail(400, "VALIDATION_ERROR");
      }
      this.seq += 1;
      const x: Json = {
        uuid: this.id("f"),
        transfer_no: `WT-${String(this.seq).padStart(6, "0")}`,
        status: "draft",
        note: body.note ?? null,
        from_warehouse: { uuid: from.uuid, code: from.code, name: from.name },
        to_warehouse: { uuid: to.uuid, code: to.code, name: to.name },
        to_location: null,
        lines: [],
        created_at: NOW,
        shipped_at: null,
        completed_at: null,
        cancelled_at: null,
      };
      this.transfers.unshift(x);
      return ok(this.transferView(x, true), 201);
    }
    const trPath = path.match(
      /^\/v1\/warehouse\/transfers\/([^/]+)(?:\/(lines|place|ship|complete|cancel))?(?:\/([^/]+))?$/,
    );
    if (trPath) {
      const x = this.transfers.find((y) => y.uuid === trPath[1]);
      if (!x) return fail(404, "NOT_FOUND");
      const lines = x.lines as Json[];
      const action = trPath[2];
      const from = x.from_warehouse as Json;
      const to = x.to_warehouse as Json;
      if (method === "GET" && !action) return ok(this.transferView(x, true));
      if (method === "POST" && action === "lines") {
        if (x.status !== "draft") return fail(409, "WAREHOUSE_TRANSFER_STATE");
        for (const raw of (body.barcodes as string[]) ?? []) {
          const barcode = raw.replace(/^OFW:UNIT:/, "");
          const loc = this.placed[barcode];
          if (!loc || loc.warehouse_uuid !== from.uuid) {
            return fail(409, "WAREHOUSE_UNIT_UNAVAILABLE");
          }
          if (lines.some((l) => l.barcode === barcode)) {
            return fail(409, "WAREHOUSE_UNIT_BUSY");
          }
          lines.push({
            uuid: this.id("fa"),
            unit_uuid: this.printed[barcode] ?? this.id("fb"),
            barcode,
            unit_status: "placed",
            product: PRODUCT,
            source_location: {
              uuid: loc.uuid,
              code: loc.code,
              full_code: loc.full_code,
            },
            target_location: null,
            out_movement_uuid: null,
            in_movement_uuid: null,
            placement_movement_uuid: null,
            restore_movement_uuid: null,
          });
        }
        return ok(this.transferView(x, true));
      }
      if (method === "DELETE" && action === "lines" && trPath[3]) {
        x.lines = lines.filter((l) => l.uuid !== trPath[3]);
        return ok(this.transferView(x, true));
      }
      if (method === "POST" && action === "place") {
        if (x.status !== "draft" && x.status !== "in_transit") {
          return fail(409, "WAREHOUSE_TRANSFER_STATE");
        }
        const code = String(body.location_code ?? "").replace(/^OFW:LOC:/, "");
        const target = body.location_uuid
          ? this.locations.find((l) => l.uuid === body.location_uuid)
          : this.locations.find((l) => l.full_code === code);
        if (!target || target.warehouse_uuid !== to.uuid) {
          return fail(400, "VALIDATION_ERROR");
        }
        const only = (body.line_uuids as string[]) ?? [];
        for (const l of lines) {
          if (only.length === 0 || only.includes(String(l.uuid))) {
            l.target_location = {
              uuid: target.uuid,
              code: target.code,
              full_code: target.full_code,
            };
          }
        }
        return ok(this.transferView(x, true));
      }
      if (method === "POST" && action === "ship") {
        if (x.status !== "draft") return fail(409, "WAREHOUSE_TRANSFER_STATE");
        if (lines.length === 0) return fail(409, "WAREHOUSE_TRANSFER_EMPTY");
        x.status = "in_transit";
        x.shipped_at = NOW;
        for (const l of lines) {
          l.unit_status = "in_transit";
          l.out_movement_uuid = this.id("fc");
        }
        return ok(this.transferView(x, true));
      }
      if (method === "POST" && action === "complete") {
        if (x.status !== "in_transit") {
          return fail(409, "WAREHOUSE_TRANSFER_STATE");
        }
        if (lines.some((l) => !l.target_location)) {
          return fail(409, "WAREHOUSE_TRANSFER_UNPLACED");
        }
        x.status = "completed";
        x.completed_at = NOW;
        for (const l of lines) {
          l.unit_status = "placed";
          const loc = this.locations.find(
            (y) => y.uuid === (l.target_location as Json).uuid,
          );
          if (loc) this.placed[String(l.barcode)] = loc;
        }
        return ok(this.transferView(x, true));
      }
      if (method === "POST" && action === "cancel") {
        x.status = "cancelled";
        x.cancelled_at = NOW;
        return ok(this.transferView(x, true));
      }
    }

    // --- Universal scan (TEC-203) ---
    if (method === "POST" && path === "/v1/warehouse/scan") {
      const code = String(body.code ?? "");
      if (code.startsWith("OFW:LOC:")) {
        const l = this.locations.find((x) => x.full_code === code.slice(8));
        if (!l) return fail(404, "SCAN_LOCATION_NOT_FOUND");
        return ok({
          type: "location",
          matched_by: "location_qr",
          code,
          location: { ...l, path: [] },
          unit: null,
          product: null,
        });
      }
      return fail(404, "SCAN_NO_MATCH");
    }

    this.unknown.push(`${method} ${path}`);
    return route.fulfill({
      status: 404,
      json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
    });
  }
}

/** Routes every BFF call except Auth.js itself to the warehouse mock. */
export async function mockWarehouse(page: Page): Promise<WarehouseMock> {
  const api = new WarehouseMock();
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => api.handle(route),
  );
  return api;
}
