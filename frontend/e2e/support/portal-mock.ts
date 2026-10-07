import type { BrowserContext, Route } from "@playwright/test";

import { E2E_PORTAL } from "./constants";

/**
 * Mocked portal BFF for the portal e2e (TEC-246). The browser only talks to
 * `/api/portal/v1/**`; this in-memory state answers it for every signed-in
 * browser context, so two portal users (owner and buyer, one context each)
 * see the same vehicles and transfers. Codes are never shown in the UI:
 * they go to `whatsapp`, the fake WhatsApp sender the spec reads from (no
 * wuzapi). The OTP sign-in codes are the ones the upstream mock verifies.
 */

type Json = Record<string, unknown>;
type PortalUser = (typeof E2E_PORTAL)[keyof typeof E2E_PORTAL];

export const PORTAL_VEHICLE = "0b9c4c1e-0000-4000-8000-000000002461";
export const PORTAL_SERVICES = {
  kadikoy: "0b9c4c1e-0000-4000-8000-000000002462",
  cankaya: "0b9c4c1e-0000-4000-8000-000000002463",
} as const;
const TRANSFER = "0b9c4c1e-0000-4000-8000-000000002464";

/** Two dealers of the brand; each did one service on the same vehicle. */
export const PORTAL_DEALERS = {
  kadikoy: {
    uuid: "0b9c4c1e-0000-4000-8000-000000002471",
    name: "Kadıköy Bayi",
    city: "İstanbul",
    district: "Kadıköy",
    address: "Bağdat Cd. 1",
    whatsapp: "+902161110000",
  },
  cankaya: {
    uuid: "0b9c4c1e-0000-4000-8000-000000002472",
    name: "Çankaya Bayi",
    city: "Ankara",
    district: "Çankaya",
    address: "Tunalı Hilmi Cd. 2",
    whatsapp: "+903121110000",
  },
} as const;

const envelope = (data: unknown) => ({ success: true, data, meta: {} });

/** A message of the fake WhatsApp sender. */
export type FakeWhatsApp = { to: string; purpose: string; code: string };

type Service = {
  uuid: string;
  service_no: string;
  dealer: (typeof PORTAL_DEALERS)[keyof typeof PORTAL_DEALERS];
  product: string;
  parts: string[];
  completed_at: string;
  warranty: { uuid: string; public_code: string };
};

const SERVICES: Service[] = [
  {
    uuid: PORTAL_SERVICES.kadikoy,
    service_no: "OLX-2026-0001",
    dealer: PORTAL_DEALERS.kadikoy,
    product: "Olex PPF Gloss",
    parts: ["body_kaput", "body_tavan"],
    completed_at: "2026-03-10T09:00:00Z",
    warranty: {
      uuid: "0b9c4c1e-0000-4000-8000-000000002481",
      public_code: "E2EPORTALWAR0001",
    },
  },
  {
    uuid: PORTAL_SERVICES.cankaya,
    service_no: "CNK-2026-0007",
    dealer: PORTAL_DEALERS.cankaya,
    product: "Olex Ceramic Pro",
    parts: ["body_on_tampon"],
    completed_at: "2026-08-20T09:00:00Z",
    warranty: {
      uuid: "0b9c4c1e-0000-4000-8000-000000002482",
      public_code: "E2EPORTALWAR0002",
    },
  },
];

const BRAND = { uuid: "0b9c4c1e-0000-4000-8000-000000002491", name: "BMW" };
const MODEL = { uuid: "0b9c4c1e-0000-4000-8000-000000002492", name: "M3" };
const PLATE = "34OLX246";
const CREATED = "2026-03-10T08:00:00Z";

function sixDigits(seed: number): string {
  return String(100000 + ((seed * 7919) % 900000));
}

export class PortalMock {
  /** Portal user that owns the vehicle (vehicles.user_id). */
  owner: string = E2E_PORTAL.owner.uuid;
  transfer: Json | null = null;
  /** Buyer of the open transfer (not part of the API answer). */
  private transferTo = "";
  private codes = { from: "", to: "" };
  whatsapp: FakeWhatsApp[] = [];
  calls: string[] = [];
  unknown: string[] = [];
  /** TEC-327: appointments booked through the portal. */
  appointments: Json[] = [];

  /** Latest code the fake WhatsApp sender delivered to `phone`. */
  lastCode(phone: string, purpose: string): string | undefined {
    return this.whatsapp.findLast(
      (m) => m.to === phone && m.purpose === purpose,
    )?.code;
  }

  private vehicle() {
    const last = SERVICES.at(-1)?.completed_at ?? null;
    return {
      uuid: PORTAL_VEHICLE,
      car_brand: BRAND,
      car_model: MODEL,
      model_year: 2024,
      plate: PLATE,
      plate_country: "TR",
      vin: "WBS00000000006752",
      service_count: SERVICES.length,
      active_warranty_count: SERVICES.length,
      last_service_at: last,
      created_at: CREATED,
    };
  }

  private vehicleDetail() {
    return {
      ...this.vehicle(),
      service_summary: {
        total: SERVICES.length,
        completed: SERVICES.length,
        organization_count: new Set(SERVICES.map((s) => s.dealer.uuid)).size,
        last_service_at: SERVICES.at(-1)?.completed_at ?? null,
      },
      // Newest first, one list across both dealers.
      services: [...SERVICES].reverse().map((s) => ({
        uuid: s.uuid,
        service_no: s.service_no,
        status: "completed",
        package: null,
        organization: { uuid: s.dealer.uuid, name: s.dealer.name },
        vehicle_uuid: PORTAL_VEHICLE,
        car_brand_name: BRAND.name,
        car_model_name: MODEL.name,
        model_year: 2024,
        plate: PLATE,
        plate_country: "TR",
        completed_at: s.completed_at,
        created_at: s.completed_at,
      })),
      active_warranties: SERVICES.map((s) => ({
        uuid: s.warranty.uuid,
        public_code: s.warranty.public_code,
        start_at: s.completed_at,
        end_at: "2036-03-10T20:59:59Z",
        days_left: 3400,
        percent_left: 95,
        product: {
          uuid: `${s.warranty.uuid.slice(0, -4)}9999`,
          sku: "PPF",
          name: s.product,
        },
        service: { uuid: s.uuid, service_no: s.service_no },
        organization: { uuid: s.dealer.uuid, name: s.dealer.name },
      })),
    };
  }

  private service(s: Service) {
    return {
      uuid: s.uuid,
      service_no: s.service_no,
      status: "completed",
      status_label: "Completed",
      vehicle_uuid: PORTAL_VEHICLE,
      car_brand: BRAND,
      car_model: MODEL,
      model_year: 2024,
      plate: PLATE,
      created_at: s.completed_at,
      completed_at: s.completed_at,
      applied_parts: s.parts,
      products: [
        {
          service_item_uuid: `${s.uuid.slice(0, -4)}8888`,
          name: s.product,
          category: "ppf",
          applied_parts: s.parts,
        },
      ],
      dealer: s.dealer,
      warranties: [
        {
          uuid: s.warranty.uuid,
          public_code: s.warranty.public_code,
          service_item_uuid: `${s.uuid.slice(0, -4)}8888`,
          product_name: s.product,
          item_kind: "product",
          status: "active",
          start_at: s.completed_at,
          end_at: "2036-03-10T20:59:59Z",
          expired_at: null,
          voided_at: null,
        },
      ],
    };
  }

  /**
   * TEC-327 portal appointments: two weeks of availability from `from`
   * (the first day full, the others with a 09:00 and a 10:00 Istanbul
   * slot), booking, listing and cancelling.
   */
  private handleAppointments(
    method: string,
    path: string,
    url: URL,
    body: Json,
  ): { data: unknown; status?: number } | null {
    const availability = /^portal\/dealers\/([^/]+)\/availability$/.exec(path);
    if (method === "GET" && availability) {
      const from = url.searchParams.get("from") ?? "";
      const days = [];
      for (let i = 0; i < 14; i++) {
        const d = new Date(`${from}T00:00:00Z`);
        d.setUTCDate(d.getUTCDate() + i);
        const date = d.toISOString().slice(0, 10);
        const full = i === 0;
        days.push({
          date,
          capacity: 2,
          occupied: full ? 2 : 0,
          remaining_capacity: full ? 0 : 2,
          closed: false,
          slots: [6, 7].map((h) => ({
            start: `${date}T0${h}:00:00Z`,
            end: `${date}T0${h + 1}:00:00Z`,
          })),
        });
      }
      return { data: days };
    }
    if (method === "GET" && path === "portal/appointments") {
      const now = Date.now();
      const upcoming = url.searchParams.get("period") !== "past";
      const items = this.appointments.filter(
        (a) => Date.parse(String(a.starts_at)) >= now === upcoming,
      );
      return { data: { items, total: items.length, limit: 20, offset: 0 } };
    }
    if (method === "POST" && path === "portal/appointments") {
      const dealer = Object.values(PORTAL_DEALERS).find(
        (d) => d.uuid === body.dealer_uuid,
      );
      const start = String(body.starts_at);
      const a = {
        uuid: `0b9c4c1e-0000-4000-8000-00000000${String(
          2480 + this.appointments.length,
        ).padStart(4, "0")}`,
        organization_id: 1,
        customer_user_id: 1,
        vehicle_id: 1,
        starts_at: start,
        ends_at: new Date(Date.parse(start) + 3_600_000).toISOString(),
        estimated_minutes: 60,
        source: "portal",
        status: "scheduled",
        note: String(body.note ?? ""),
        dealer_uuid: body.dealer_uuid,
        dealer_name: dealer?.name ?? "",
        vehicle_uuid: body.vehicle_uuid,
        vehicle_plate: PLATE,
      };
      this.appointments.push(a);
      return { data: a, status: 201 };
    }
    const cancel = /^portal\/appointments\/([^/]+)\/cancel$/.exec(path);
    if (method === "POST" && cancel) {
      const a = this.appointments.find((x) => x.uuid === cancel[1]);
      if (!a) return null;
      a.status = "cancelled";
      return { data: a };
    }
    return null;
  }

  async handle(route: Route, user: PortalUser) {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api\/portal\/v1\//, "");
    const method = req.method();
    this.calls.push(`${user.phone} ${method} ${path}`);
    const body = (req.postData() ? req.postDataJSON() : {}) as Json;
    const ok = (data: unknown, status = 200) =>
      route.fulfill({ status, json: envelope(data) });
    const fail = (status: number, code: string, details: Json[] = []) =>
      route.fulfill({
        status,
        json: { success: false, error: { code, message: code, details } },
      });
    const owns = this.owner === user.uuid;

    if (method === "POST" && path === "auth/otp/request") {
      const target = Object.values(E2E_PORTAL).find(
        (u) => u.typed === body.phone,
      );
      if (body.purpose !== "customer_login" || !target) {
        return fail(400, "INVALID_PHONE");
      }
      this.whatsapp.push({
        to: target.phone,
        purpose: "customer_login",
        code: target.code,
      });
      return ok({
        status: "sent",
        channel: "whatsapp",
        expires_at: new Date(Date.now() + 300_000).toISOString(),
        resend_at: new Date(Date.now() + 60_000).toISOString(),
      });
    }
    if (method === "GET" && path === "auth/me") {
      return ok({
        user: { name: user.name, surname: user.surname, email: null },
        roles: ["customer"],
      });
    }
    if (method === "GET" && path === "portal/consents/pending") {
      return ok({ items: [] });
    }
    if (method === "GET" && path === "portal/vehicles") {
      const items = owns ? [this.vehicle()] : [];
      return ok({ items, total: items.length, limit: 12, offset: 0 });
    }
    if (path.startsWith(`portal/vehicles/${PORTAL_VEHICLE}`) && !owns) {
      return fail(404, "NOT_FOUND");
    }
    if (method === "GET" && path === `portal/vehicles/${PORTAL_VEHICLE}`) {
      return ok(this.vehicleDetail());
    }
    const service = SERVICES.find((s) => path === `portal/services/${s.uuid}`);
    if (method === "GET" && service) {
      return owns ? ok(this.service(service)) : fail(404, "NOT_FOUND");
    }
    // TEC-244: review state of the service (no review, dealer without a
    // Google link); the card is not under test here.
    const reviewed = SERVICES.find(
      (s) => path === `portal/services/${s.uuid}/review`,
    );
    if (method === "GET" && reviewed) {
      return owns
        ? ok({ review: null, can_review: true, google_business_url: null })
        : fail(404, "NOT_FOUND");
    }
    const transfers = `portal/vehicles/${PORTAL_VEHICLE}/transfers`;
    if (method === "GET" && path === transfers) {
      return ok({ items: this.transfer ? [this.transfer] : [] });
    }
    if (method === "POST" && path === transfers) {
      const to = Object.values(E2E_PORTAL).find((u) => u.phone === body.phone);
      if (!to) return fail(400, "VALIDATION_ERROR", [{ field: "phone" }]);
      if (to.uuid === user.uuid)
        return fail(409, "VEHICLE_TRANSFER_SAME_OWNER");
      this.codes = { from: sixDigits(1), to: sixDigits(2) };
      this.whatsapp.push(
        { to: user.phone, purpose: "vehicle_transfer", code: this.codes.from },
        { to: to.phone, purpose: "vehicle_transfer", code: this.codes.to },
      );
      this.transfer = {
        uuid: TRANSFER,
        vehicle_uuid: PORTAL_VEHICLE,
        status: "pending",
        to_phone_masked: `${to.phone.slice(0, 5)}*****${to.phone.slice(-2)}`,
        new_owner_known: true,
        from_verified: false,
        to_verified: false,
        attempts: 0,
        max_attempts: 5,
        expires_at: new Date(Date.now() + 600_000).toISOString(),
        created_at: new Date().toISOString(),
        completed_at: null,
        cancelled_at: null,
        warranties_moved: 0,
      };
      this.transferTo = to.uuid;
      return ok(this.transfer, 201);
    }
    if (
      method === "POST" &&
      path === `portal/vehicle-transfers/${TRANSFER}/verify` &&
      this.transfer?.status === "pending" &&
      owns
    ) {
      const t = this.transfer;
      const wrong: string[] = [];
      if (body.from_code !== undefined) {
        if (body.from_code === this.codes.from) t.from_verified = true;
        else wrong.push("from_code");
      }
      if (body.to_code !== undefined) {
        if (body.to_code === this.codes.to) t.to_verified = true;
        else wrong.push("to_code");
      }
      if (wrong.length > 0) {
        t.attempts = Number(t.attempts) + 1;
        const left = String(Number(t.max_attempts) - Number(t.attempts));
        return fail(
          422,
          "VEHICLE_TRANSFER_INVALID_CODE",
          wrong.map((field) => ({ field, message: "wrong code", code: left })),
        );
      }
      if (t.from_verified && t.to_verified) {
        t.status = "completed";
        t.completed_at = new Date().toISOString();
        t.warranties_moved = SERVICES.length;
        // Vehicle and its warranties move to the buyer's account.
        this.owner = this.transferTo;
      }
      return ok(t);
    }

    const appointment = this.handleAppointments(method, path, url, body);
    if (appointment) return ok(appointment.data, appointment.status);

    this.unknown.push(`${method} ${path}`);
    return fail(404, "NOT_FOUND");
  }
}

/** Routes the portal BFF of `context` (signed in as `user`) to `api`. */
export async function routePortal(
  context: BrowserContext,
  api: PortalMock,
  user: PortalUser,
): Promise<void> {
  await context.route("**/api/portal/v1/**", (route) =>
    api.handle(route, user),
  );
}
