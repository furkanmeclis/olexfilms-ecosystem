import type { Page, Route } from "@playwright/test";

/**
 * Mocked BFF for the customer e2e (TEC-163): a center admin with every
 * customer / vehicle permission and an in-memory customer store that
 * follows the TEC-159..161 API (create, detail, vehicles, anonymize).
 */

export const CUSTOMER_SLUG = "acme";
const ORG = "0b9c4c1e-0000-4000-8000-000000000101";
const USER = "0b9c4c1e-0000-4000-8000-000000000102";
export const CUSTOMER_UUID = "0b9c4c1e-0000-4000-8000-0000000001c1";
const VEHICLE_UUID = "0b9c4c1e-0000-4000-8000-0000000001a1";
const NOW = "2026-10-01T09:00:00Z";

const PERMISSIONS = [
  "customers.read",
  "customers.write",
  "customers.anonymize",
  "vehicles.read",
  "vehicles.write",
  "organizations.read",
];

const PLATE_FORMATS = [
  {
    country_iso2: "TR",
    country_name_en: "Turkey",
    country_name_tr: "Türkiye",
    regex: "^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$",
    input_mask: "99 AAA 9999",
    example: "34 ABC 123",
    country_label: "TR",
    strip_color: "#003399",
    background_color: "#FFFFFF",
    text_color: "#000000",
    is_active: true,
    sort_order: 1,
  },
];

type Json = Record<string, unknown>;
const envelope = (data: unknown) => ({ success: true, data, meta: {} });

export class CustomerMock {
  customer: Json | null = null;
  vehicles: Json[] = [];
  calls: string[] = [];
  bodies: Record<string, unknown[]> = {};
  unknown: string[] = [];

  private membership() {
    return {
      uuid: ORG,
      slug: CUSTOMER_SLUG,
      name: "Olex Merkez",
      role: "owner",
      logo_url: null,
      status: "active",
      access_ends_at: null,
      type: "center",
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
        surname: "Customers",
        status: "active",
        is_super_admin: false,
        email_verified: true,
        locale: "en",
        timezone: "Europe/Istanbul",
      },
      roles: [],
      permissions: PERMISSIONS,
      grants: Object.fromEntries(PERMISSIONS.map((p) => [p, "brand"])),
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
    const body = req.postData() ? (req.postDataJSON() as Json) : undefined;
    if (body !== undefined) {
      (this.bodies[`${method} ${path}`] ??= []).push(body);
    }
    const ok = (data: unknown, status = 200) =>
      route.fulfill({ status, json: envelope(data) });
    const cust = `/v1/customers/${CUSTOMER_UUID}`;

    if (
      method === "GET" &&
      path === `/v1/public/organizations/by-slug/${CUSTOMER_SLUG}`
    ) {
      return ok({
        uuid: ORG,
        slug: CUSTOMER_SLUG,
        name: "Olex Merkez",
        status: "active",
        logo_url: null,
        access_ok: true,
      });
    }
    if (method === "GET" && path === "/v1/auth/me") return ok(this.me());
    if (method === "GET" && path === "/v1/auth/step-up") {
      return ok({ valid: true, expires_at: null, methods: [] });
    }
    if (method === "GET" && path === "/v1/features") {
      return ok({
        organization_type: "center",
        items: [],
        enabled: ["customers"],
      });
    }
    if (method === "GET" && path === "/v1/me/organizations") {
      return ok({ items: [this.membership()] });
    }
    if (method === "GET" && path === "/v1/plate-formats") {
      return ok({ items: PLATE_FORMATS });
    }
    if (method === "GET" && path === "/v1/vehicle-catalog/brands") {
      return ok({ items: [], total: 0, limit: 20, offset: 0 });
    }
    if (method === "GET" && path === "/v1/customers") {
      const q = (url.searchParams.get("q") ?? "").toLowerCase();
      const c = this.customer;
      const hit =
        c &&
        (!q ||
          String(c.name).toLowerCase().includes(q) ||
          String(c.phone ?? "").includes(q));
      return ok({
        items: hit ? [c] : [],
        total: hit ? 1 : 0,
        limit: 20,
        offset: 0,
      });
    }
    if (method === "POST" && path === "/v1/customers") {
      this.customer = {
        uuid: CUSTOMER_UUID,
        name: body?.name,
        surname: body?.surname ?? "",
        email: body?.email ?? null,
        phone: "+905551234567",
        status: "active",
        anonymized: false,
        type: body?.type ?? "individual",
        company_name: body?.company_name ?? null,
        locale: null,
        created_at: NOW,
        linked_at: NOW,
        first_service_at: null,
        tax_office: null,
        national_id_last4: null,
        tax_no_last4: null,
        address: {},
        notification_prefs: {},
        editable: true,
        identity_editable: true,
        organizations: [
          {
            uuid: ORG,
            name: "Olex Merkez",
            type: "center",
            linked_at: NOW,
            first_service_at: null,
          },
        ],
      };
      return ok(
        { ...this.customer, existing_user: false, ignored_fields: [] },
        201,
      );
    }
    if (this.customer && method === "GET" && path === cust) {
      return ok(this.customer);
    }
    if (this.customer && method === "POST" && path === `${cust}/anonymize`) {
      Object.assign(this.customer, {
        name: "Anonymous customer",
        surname: "",
        email: null,
        phone: null,
        status: "anonymized",
        anonymized: true,
        editable: false,
      });
      return ok({
        uuid: CUSTOMER_UUID,
        status: "anonymized",
        changed: true,
        anonymized_at: NOW,
      });
    }
    if (method === "GET" && path === "/v1/vehicles") {
      return ok({
        items: this.vehicles,
        total: this.vehicles.length,
        limit: 100,
        offset: 0,
      });
    }
    if (method === "POST" && path === "/v1/vehicles") {
      const plate = String(body?.plate ?? "");
      const v = {
        uuid: VEHICLE_UUID,
        customer_uuid: CUSTOMER_UUID,
        organization_uuid: ORG,
        plate,
        plate_normalized: plate.replace(/\s/g, "").toUpperCase(),
        plate_country: body?.plate_country ?? "TR",
        vin: body?.vin ?? null,
        model_year: body?.model_year ?? null,
        car_brand: null,
        car_model: null,
        created_at: NOW,
        updated_at: NOW,
        warnings: [],
      };
      this.vehicles.push(v);
      return ok(v, 201);
    }

    // Customer list export (TEC-164/TEC-199): queued, then completed.
    const listExport = "/v1/customer-list-exports/list-export-1";
    if (method === "POST" && path === "/v1/customers/export") {
      return ok(
        {
          uuid: "list-export-1",
          resource: "customers",
          format: body?.format ?? "csv",
          status: "queued",
          row_count: 0,
          error: null,
          download_url: null,
          created_at: NOW,
        },
        202,
      );
    }
    if (method === "GET" && path === listExport) {
      return ok({
        uuid: "list-export-1",
        resource: "customers",
        format: "csv",
        status: "completed",
        row_count: this.customer ? 1 : 0,
        error: null,
        download_url: `${listExport}/download`,
        created_at: NOW,
      });
    }
    if (method === "GET" && path === `${listExport}/download`) {
      return route.fulfill({
        status: 200,
        contentType: "text/csv",
        headers: {
          "Content-Disposition": 'attachment; filename="customers.csv"',
        },
        body: "name,phone\nAyşe Yılmaz,+905551234567\n",
      });
    }

    this.unknown.push(`${method} ${path}`);
    return route.fulfill({
      status: 404,
      json: { error: { code: "NOT_FOUND", message: "Not mocked" } },
    });
  }
}

/** Routes every BFF call except Auth.js itself to the customer mock. */
export async function mockCustomers(page: Page): Promise<CustomerMock> {
  const api = new CustomerMock();
  await page.route(
    (url) =>
      url.pathname.startsWith("/api/") &&
      !url.pathname.startsWith("/api/auth/"),
    (route) => api.handle(route),
  );
  return api;
}
