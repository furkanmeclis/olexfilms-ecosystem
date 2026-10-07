import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Appointment = Schemas["Appointment"];
export type AppointmentStatus = Schemas["AppointmentStatus"];
export type AppointmentInput = Schemas["AppointmentInput"];
export type AppointmentSettings = Schemas["AppointmentSettings"];
export type AppointmentSettingsInput = Schemas["AppointmentSettingsInput"];
export type AppointmentClosure = Schemas["AppointmentClosure"];
export type AppointmentAvailabilityDay = Schemas["AppointmentAvailabilityDay"];
export type AppointmentOccupancy = Schemas["AppointmentOccupancy"];

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

const enc = encodeURIComponent;

/** GET /v1/appointments allows 100 per page; a calendar range is paged. */
const PAGE = 100;
const MAX_PAGES = 10;

/**
 * Panel appointments API (TEC-322 backend, TEC-326 calendar). Every call
 * goes through the BFF with the active organization of the session.
 */
export const appointmentsService = {
  /**
   * Every appointment starting in [from, to); `organizationUuid` narrows
   * to one organization of the read scope (TEC-328 read-only calendar).
   */
  async listRange(
    from: string,
    to: string,
    organizationUuid?: string,
  ): Promise<Appointment[]> {
    const items: Appointment[] = [];
    for (let page = 0; page < MAX_PAGES; page++) {
      const res = await platformRequest<Page<Appointment>>(
        "GET",
        "/v1/appointments",
        {
          query: {
            from,
            to,
            limit: PAGE,
            offset: page * PAGE,
            ...(organizationUuid
              ? { organization_uuid: organizationUuid }
              : {}),
          },
        },
      );
      items.push(...res.items);
      if (items.length >= res.total || res.items.length < PAGE) break;
    }
    return items;
  },
  availability(from: string, to: string, organizationUuid?: string) {
    return platformRequest<AppointmentAvailabilityDay[]>(
      "GET",
      "/v1/appointments/availability",
      {
        query: organizationUuid
          ? { from, to, organization_uuid: organizationUuid }
          : { from, to },
      },
    );
  },
  /** Occupancy of every organization of the read scope on `date` (TEC-328). */
  occupancy(date: string) {
    return platformRequest<AppointmentOccupancy[]>(
      "GET",
      "/v1/appointments/occupancy",
      { query: { date } },
    );
  },
  create(body: AppointmentInput) {
    return platformRequest<Appointment>("POST", "/v1/appointments", { body });
  },
  /** Full replace of the booking fields (backend PatchInput = CreateInput). */
  patch(uuid: string, body: AppointmentInput) {
    return platformRequest<Appointment>(
      "PATCH",
      `/v1/appointments/${enc(uuid)}`,
      { body },
    );
  },
  setStatus(uuid: string, status: AppointmentStatus, cancelReason?: string) {
    return platformRequest<Appointment>(
      "POST",
      `/v1/appointments/${enc(uuid)}/status`,
      {
        body:
          status === "cancelled"
            ? { status, cancel_reason: cancelReason ?? "" }
            : { status },
      },
    );
  },
  startIntake(uuid: string) {
    return platformRequest<Appointment>(
      "POST",
      `/v1/appointments/${enc(uuid)}/start-intake`,
    );
  },
  getSettings() {
    return platformRequest<AppointmentSettings>(
      "GET",
      "/v1/appointment-settings",
    );
  },
  putSettings(body: AppointmentSettingsInput) {
    return platformRequest<AppointmentSettings>(
      "PUT",
      "/v1/appointment-settings",
      { body },
    );
  },
  listClosures(from: string, to: string) {
    return platformRequest<AppointmentClosure[]>(
      "GET",
      "/v1/appointment-closures",
      { query: { from, to } },
    );
  },
  createClosure(closedOn: string, reason: string) {
    return platformRequest<AppointmentClosure>(
      "POST",
      "/v1/appointment-closures",
      {
        body: reason
          ? { closed_on: closedOn, reason }
          : { closed_on: closedOn },
      },
    );
  },
  deleteClosure(uuid: string) {
    return platformRequest<void>(
      "DELETE",
      `/v1/appointment-closures/${enc(uuid)}`,
    );
  },
};

/**
 * The PATCH body that keeps every booking field of `a` and moves it to
 * `startsAt` (the backend replaces all fields, so nothing may be dropped).
 */
export function rescheduleBody(
  a: Appointment,
  startsAt: string,
): AppointmentInput {
  return {
    customer_user_id: a.customer_user_id,
    vehicle_id: a.vehicle_id ?? null,
    starts_at: startsAt,
    estimated_minutes: a.estimated_minutes,
    source: a.source,
    note: a.note,
  };
}

export const appointmentKeys = {
  all: ["appointments"] as const,
  range: (from: string, to: string, organizationUuid = "") =>
    ["appointments", "range", from, to, organizationUuid] as const,
  availability: (from: string, to: string, organizationUuid = "") =>
    ["appointments", "availability", from, to, organizationUuid] as const,
  occupancy: (date: string) => ["appointments", "occupancy", date] as const,
  settings: ["appointments", "settings"] as const,
  closures: (from: string, to: string) =>
    ["appointments", "closures", from, to] as const,
};
