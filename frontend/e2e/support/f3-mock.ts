import { E2E_PORTAL } from "./constants";
import {
  ORG,
  SERVICE_UUID,
  USER,
  customer,
  type Json,
  type MockApi,
  type MockCall,
} from "./mock-api";

/**
 * F3 gate mocks (TEC-304) on top of the service MockApi: the intake
 * contract (TEC-287/291: customer OTP + canvas signature, staff signature,
 * executed PDF) and the before/after measurements of the service
 * (TEC-296/300: candidates, links, the part diff, "checked"). Only the
 * spec that calls `installF3` turns the measurements and intake_contracts
 * modules and contracts.intake_required on.
 */

export const VIN = "WBA8E9C50GK644197";
export const CONTRACT_UUID = "0b9c4c1e-0000-4000-8000-000000003041";
export const CONTRACT_NO = 3041;
export const MEASUREMENTS = {
  before: "0b9c4c1e-0000-4000-8000-000000003042",
  after: "0b9c4c1e-0000-4000-8000-000000003043",
  older: "0b9c4c1e-0000-4000-8000-000000003044",
} as const;
const CUSTOMER_SIGNER = "0b9c4c1e-0000-4000-8000-000000003045";
const STAFF_SIGNER = "0b9c4c1e-0000-4000-8000-000000003046";
const EXECUTED_AT = "2026-10-01T09:40:00Z";
const COMPLETED_AT = "2026-10-01T10:30:00Z";
/** Six digits the fake WhatsApp sender delivers for the contract OTP. */
export const CONTRACT_OTP = "304304";

export const F3_PERMISSIONS = [
  "contracts.read",
  "contracts.write",
  "measurements.read",
  "measurements.link",
];

type Phase = "before" | "after";
type Link = {
  phase: Phase;
  link_source: "auto" | "manual";
  confirmed: boolean;
  confirmed_at: string | null;
  confirmed_by: null | { uuid: string; name: string };
  linked_at: string;
  measurement: Json;
};

function brief(uuid: string, measuredAt: string, serial: string): Json {
  return {
    uuid,
    vin: VIN,
    status: "accepted",
    source: "mobile",
    device_serial: serial,
    measured_at: measuredAt,
    created_at: measuredAt,
  };
}

const stats = (avg: string | null) => ({
  average_um: avg,
  min_um: avg,
  max_um: avg,
  count: avg === null ? 0 : 4,
});

export class F3Mock {
  contract: Json | null = null;
  links: Link[] = [];
  checkedAt: string | null = null;
  /** Codes of the fake WhatsApp sender (no wuzapi). */
  whatsapp: { to: string; code: string }[] = [];
  /** Candidate measurements of the VIN; `before` is the suggestion. */
  readonly measurements = {
    before: brief(MEASUREMENTS.before, "2026-10-01T08:30:00Z", "NX-304-A"),
    older: brief(MEASUREMENTS.older, "2026-06-12T11:00:00Z", "NX-304-A"),
    after: brief(MEASUREMENTS.after, "2026-10-01T10:20:00Z", "NX-304-A"),
  };

  constructor(private api: MockApi) {}

  private signer(role: "customer" | "staff"): Json {
    const signers = (this.contract?.signers ?? []) as Json[];
    const s = signers.find((x) => x.role === role);
    if (!s) throw new Error(`no ${role} signer`);
    return s;
  }

  /** Mirrors the contract into the service summary. */
  private sync() {
    const c = this.contract;
    this.api.contract = c
      ? {
          uuid: c.uuid,
          status: c.status,
          contract_no: c.contract_no,
          pdf_ready: c.pdf_ready,
        }
      : null;
  }

  private confirm(link: Link) {
    link.confirmed = true;
    link.confirmed_at = new Date().toISOString();
    link.confirmed_by = { uuid: USER, name: "E2E Dealer" };
  }

  private serviceMeasurements(): Json {
    const linked = new Set(this.links.map((l) => l.measurement.uuid));
    const hasBefore = this.links.some((l) => l.phase === "before");
    const service = this.api.service ?? {};
    return {
      service_uuid: SERVICE_UUID,
      vin: service.vin ?? null,
      has_measurement: Boolean(service.has_measurement),
      status: service.status ?? "draft",
      links: this.links,
      suggestions: hasBefore
        ? []
        : [{ phase: "before", measurement: this.measurements.before }],
      candidates: [this.measurements.older].filter(
        (m) => !linked.has(m.uuid as string),
      ),
    };
  }

  private diff(): Json {
    const after = this.links.some((l) => l.phase === "after");
    const row = (
      place: string,
      part: string,
      key: string,
      before: string,
      afterUm: string,
      diffUm: string,
      deviation: boolean,
    ) => ({
      place_id: place,
      part_type: part,
      service_part_key: key,
      before: stats(before),
      after: stats(after ? afterUm : null),
      diff_um: after ? diffUm : null,
      expected_um: "190.00",
      deviation: after && deviation,
      expected_status: "available",
    });
    const parts = [
      row("front", "HOOD", "body_kaput", "110.00", "296.00", "186.00", false),
      row("top", "ROOF", "body_tavan", "105.00", "120.00", "15.00", true),
    ];
    return {
      service_uuid: SERVICE_UUID,
      check_required: parts.some((p) => p.deviation),
      checked_at: this.checkedAt,
      tolerance_um: "20.00",
      parts,
    };
  }

  /** The executed contract as "Sözleşmelerim" lists it (PortalContract). */
  portalContract(): Json {
    const c = this.contract;
    const s = this.api.service;
    if (!c || !s) throw new Error("no contract");
    return {
      contract_uuid: c.uuid,
      contract_no: c.contract_no,
      service: { uuid: s.uuid, service_no: s.service_no },
      status: "completed",
      organization: { uuid: ORG, name: "Acme Bayi" },
      vehicle_uuid: s.vehicle_uuid,
      car_brand_name: "BMW",
      car_model_name: "320i",
      model_year: s.model_year,
      plate: s.plate,
      plate_country: s.plate_country,
      executed_at: c.executed_at,
      pdf_ready: c.pdf_ready,
      created_at: c.created_at,
    };
  }

  handle = async ({
    method,
    path,
    body,
    ok,
    route,
  }: MockCall): Promise<boolean> => {
    const svc = `/v1/services/${SERVICE_UUID}`;
    const contract = `/v1/contracts/${CONTRACT_UUID}`;
    const fail = async (status: number, code: string, details: Json[] = []) => {
      await route.fulfill({
        status,
        json: { success: false, error: { code, message: code, details } },
      });
      return true;
    };

    // Intake contract (TEC-287).
    if (method === "POST" && path === `${svc}/contract`) {
      const now = new Date().toISOString();
      this.contract = {
        uuid: CONTRACT_UUID,
        contract_no: CONTRACT_NO,
        subject_type: "service",
        subject_id: 1,
        kind: "vehicle_intake",
        locale: "en",
        template_version: 1,
        otp_required: true,
        signature_required: true,
        status: "pending",
        rendered_html: `<h1>Vehicle intake</h1><p>${customer.name} ${customer.surname}, 34ABC123, ${VIN}</p>`,
        content_sha256: "e2e",
        executed_at: null,
        voided_at: null,
        void_reason: null,
        pdf_ready: false,
        signers: [
          {
            uuid: CUSTOMER_SIGNER,
            role: "customer",
            user_id: null,
            name: `${customer.name} ${customer.surname}`,
            phone_e164: E2E_PORTAL.owner.phone,
            otp_verified_at: null,
            signed_at: null,
            signature: null,
          },
          {
            uuid: STAFF_SIGNER,
            role: "staff",
            user_id: 2,
            name: "E2E Dealer",
            phone_e164: null,
            otp_verified_at: null,
            signed_at: null,
            signature: null,
          },
        ],
        media: [],
        created_at: now,
        updated_at: now,
      };
      this.sync();
      await ok(this.contract, 201);
      return true;
    }
    if (method === "GET" && path === contract && this.contract) {
      await ok(this.contract);
      return true;
    }
    if (method === "POST" && path === `${contract}/signers/customer/otp`) {
      this.whatsapp.push({ to: E2E_PORTAL.owner.phone, code: CONTRACT_OTP });
      await ok({
        channel: "whatsapp",
        expires_at: new Date(Date.now() + 1_800_000).toISOString(),
        resend_at: new Date(Date.now() + 60_000).toISOString(),
      });
      return true;
    }
    const sign = path.match(
      /^\/v1\/contracts\/[^/]+\/signers\/(customer|staff)\/sign$/,
    );
    if (method === "POST" && sign && this.contract) {
      const role = sign[1] as "customer" | "staff";
      if (role === "customer" && body?.code !== CONTRACT_OTP) {
        return fail(422, "VALIDATION_ERROR", [
          { field: "code", message: "invalid" },
        ]);
      }
      if (!body?.signature_png) {
        return fail(422, "VALIDATION_ERROR", [
          { field: "signature_png", message: "required" },
        ]);
      }
      const at = new Date().toISOString();
      Object.assign(this.signer(role), {
        signed_at: at,
        otp_verified_at: role === "customer" ? at : null,
      });
      const both = ["customer", "staff"].every((r) =>
        Boolean(this.signer(r as "customer" | "staff").signed_at),
      );
      if (both) {
        // The PDF render is a job; the mock has it ready at once.
        Object.assign(this.contract, {
          status: "executed",
          executed_at: EXECUTED_AT,
          pdf_ready: true,
        });
      }
      this.contract.updated_at = at;
      this.sync();
      await ok(this.contract);
      return true;
    }
    if (method === "GET" && path === `${contract}/pdf`) {
      await route.fulfill({
        status: 200,
        contentType: "application/pdf",
        headers: {
          "content-disposition": `attachment; filename="contract-${CONTRACT_NO}.pdf"`,
        },
        body: "%PDF-1.4\n%e2e\n",
      });
      return true;
    }

    // Service measurements (TEC-296/297).
    if (method === "GET" && path === `${svc}/measurements`) {
      await ok(this.serviceMeasurements());
      return true;
    }
    if (method === "POST" && path === `${svc}/measurements`) {
      if (!this.api.service?.has_measurement) {
        return fail(409, "MEASUREMENT_NOT_EXPECTED");
      }
      const phase = body?.phase as Phase;
      const uuid = body?.measurement_uuid as string;
      const existing = this.links.find((l) => l.phase === phase);
      if (existing && existing.measurement.uuid === uuid) {
        this.confirm(existing);
      } else {
        const m = Object.values(this.measurements).find((x) => x.uuid === uuid);
        if (!m) return fail(404, "NOT_FOUND");
        this.links = this.links.filter((l) => l.phase !== phase);
        const link: Link = {
          phase,
          link_source: "manual",
          confirmed: false,
          confirmed_at: null,
          confirmed_by: null,
          linked_at: new Date().toISOString(),
          measurement: m,
        };
        // A manual pick is confirmed by the dealer who picks it.
        this.confirm(link);
        this.links.push(link);
      }
      await ok(this.serviceMeasurements());
      return true;
    }
    if (method === "GET" && path === `${svc}/measurements/diff`) {
      await ok(this.diff());
      return true;
    }
    if (method === "POST" && path === `${svc}/measurements/checked`) {
      this.checkedAt = new Date().toISOString();
      await route.fulfill({ status: 204 });
      return true;
    }
    return false;
  };

  /**
   * Completion auto-links the "after" measurement of the VIN (TEC-296);
   * the auto match waits for the dealer's confirmation.
   */
  onTransition = (to: string) => {
    if (to !== "completed") return;
    this.links.push({
      phase: "after",
      link_source: "auto",
      confirmed: false,
      confirmed_at: null,
      confirmed_by: null,
      linked_at: COMPLETED_AT,
      measurement: this.measurements.after,
    });
  };
}

/**
 * Turns the F3 modules on for this MockApi only: measurements and
 * intake_contracts, contracts.intake_required, a vehicle with a VIN and
 * the contract / measurement permissions.
 */
export function installF3(api: MockApi): F3Mock {
  const f3 = new F3Mock(api);
  api.features = [...api.features, "measurements", "intake_contracts"];
  api.permissions = [...api.permissions, ...F3_PERMISSIONS];
  api.vehicleVin = VIN;
  api.contractRequired = true;
  api.extra.push(f3.handle);
  api.onTransition = f3.onTransition;
  return f3;
}
