import type {
  AppointmentAvailabilityDay,
  PortalAppointment,
} from "@/features/portal/lib/portal-client";

/**
 * TEC-327 portal appointments: the rules the booking flow and "my
 * appointments" share with the F3-04c API.
 */

/** Page size of /portal/appointments. */
export const PORTAL_APPOINTMENT_PAGE_SIZE = 20;
/** Days shown at once in the booking day picker. */
export const BOOKING_WINDOW_DAYS = 14;
/** How far ahead the booking day picker can go (API range limit is 62). */
export const BOOKING_MAX_AHEAD_DAYS = 56;
/** The portal can cancel until two hours before the start (422 after). */
export const PORTAL_CANCEL_WINDOW_MS = 2 * 60 * 60 * 1000;

/** Local calendar date as YYYY-MM-DD (the availability `from`/`to`). */
export function isoDate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

/** YYYY-MM-DD plus n calendar days. */
export function addDaysIso(date: string, days: number): string {
  const [y, m, d] = date.split("-").map(Number);
  return isoDate(new Date(y, m - 1, d + days));
}

/**
 * Whether a day can be booked: open, not full and with at least one free
 * start time left (the API drops past slots).
 */
export function isDayBookable(day: AppointmentAvailabilityDay): boolean {
  return !day.closed && day.remaining_capacity > 0 && day.slots.length > 0;
}

export type DayState = "bookable" | "closed" | "full" | "none";

export function dayState(day: AppointmentAvailabilityDay): DayState {
  if (day.closed) return "closed";
  if (day.remaining_capacity <= 0) return "full";
  return day.slots.length > 0 ? "bookable" : "none";
}

/** Statuses the customer may still cancel. */
const CANCELLABLE = new Set(["scheduled", "confirmed"]);

export type CancelState =
  { kind: "allowed" } | { kind: "window_closed" } | { kind: "not_cancellable" };

/**
 * Whether "cancel" is offered for an appointment: only scheduled or
 * confirmed ones, and only until two hours before the start; inside the
 * window the button stays visible but disabled with an explanation.
 */
export function portalCancelState(
  a: Pick<PortalAppointment, "status" | "starts_at">,
  now: Date,
): CancelState {
  if (!CANCELLABLE.has(a.status)) return { kind: "not_cancellable" };
  const left = new Date(a.starts_at).getTime() - now.getTime();
  if (!(left >= PORTAL_CANCEL_WINDOW_MS)) return { kind: "window_closed" };
  return { kind: "allowed" };
}

const ERROR_KEYS: Record<string, string> = {
  APPOINTMENT_CAPACITY_FULL: "portal.appointments.error.capacity_full",
  APPOINTMENT_DAY_CLOSED: "portal.appointments.error.day_closed",
  APPOINTMENT_CANCEL_WINDOW_CLOSED: "portal.appointments.error.cancel_window",
  APPOINTMENT_INVALID_TRANSITION: "portal.appointments.error.invalid_status",
  PORTAL_READ_ONLY: "portal.appointments.error.read_only",
};

/** Message key of a failed booking or cancellation. */
export function portalAppointmentErrorKey(err: {
  status?: number;
  code?: string | null;
}): string {
  if (err.code && ERROR_KEYS[err.code]) return ERROR_KEYS[err.code];
  if (err.status === 404) return "portal.appointments.error.not_found";
  if (err.status === 400) return "portal.appointments.error.invalid";
  return "portal.appointments.error.generic";
}

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function single(v: string | string[] | undefined): string | null {
  const s = Array.isArray(v) ? v[0] : v;
  return s && s.trim() ? s.trim() : null;
}

/**
 * Preselection of the booking page from its query (`dealer` + `dealer_name`
 * from a dealer card, `vehicle` from the vehicle page); malformed values
 * are ignored.
 */
export function bookingPreset(
  query: Record<string, string | string[] | undefined>,
): {
  dealer: { uuid: string; name: string } | null;
  vehicle: string | null;
} {
  const dealer = single(query.dealer);
  const name = single(query.dealer_name);
  const vehicle = single(query.vehicle);
  return {
    dealer:
      dealer && UUID_RE.test(dealer)
        ? { uuid: dealer, name: (name ?? "").slice(0, 200) }
        : null,
    vehicle: vehicle && UUID_RE.test(vehicle) ? vehicle : null,
  };
}
