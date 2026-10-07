import type {
  Appointment,
  AppointmentStatus,
} from "@/features/appointments/services/appointments.service";

import { isApiError } from "@/lib/api";

import { zonedParts } from "./time";

/** Card colors per status (border-s + tint), RTL-safe logical classes. */
export const STATUS_TONE: Record<AppointmentStatus, string> = {
  scheduled:
    "border-s-sky-500 bg-sky-50 text-sky-950 dark:bg-sky-950/40 dark:text-sky-100",
  confirmed:
    "border-s-indigo-500 bg-indigo-50 text-indigo-950 dark:bg-indigo-950/40 dark:text-indigo-100",
  arrived:
    "border-s-emerald-500 bg-emerald-50 text-emerald-950 dark:bg-emerald-950/40 dark:text-emerald-100",
  no_show:
    "border-s-amber-500 bg-amber-50 text-amber-950 dark:bg-amber-950/40 dark:text-amber-100",
  cancelled: "border-s-muted-foreground bg-muted text-muted-foreground",
};

/** Status changes the backend allows (usecase allowedTransition). */
export function nextStatuses(status: AppointmentStatus): AppointmentStatus[] {
  switch (status) {
    case "scheduled":
      return ["confirmed", "no_show", "cancelled"];
    case "confirmed":
      return ["arrived", "no_show", "cancelled"];
    default:
      return [];
  }
}

/** Only scheduled or confirmed appointments move (PATCH). */
export const canReschedule = (a: Appointment) =>
  a.status === "scheduled" || a.status === "confirmed";

/**
 * Vehicle intake starts once the customer has arrived, the booking has a
 * vehicle and no draft service is linked yet.
 */
export const canStartIntake = (a: Appointment) =>
  a.status === "arrived" && a.vehicle_id != null && !a.service_uuid;

export type PlacedAppointment = {
  appointment: Appointment;
  /** Minutes after local midnight. */
  start: number;
  end: number;
  /** Overlap lane and lane count of the overlap cluster. */
  lane: number;
  lanes: number;
};

const DAY = 24 * 60;

/**
 * Places appointments on calendar days in the given zone: each one lands on
 * the local day of its start, with overlap lanes so concurrent bookings sit
 * side by side. Appointments outside `days` are dropped.
 */
export function placeAppointments(
  items: Appointment[],
  days: string[],
  timeZone: string,
): Record<string, PlacedAppointment[]> {
  const out: Record<string, PlacedAppointment[]> = {};
  for (const d of days) out[d] = [];
  for (const a of items) {
    const { date, minutes } = zonedParts(a.starts_at, timeZone);
    const bucket = out[date];
    if (!bucket) continue;
    const length = Math.max(a.estimated_minutes, 15);
    bucket.push({
      appointment: a,
      start: minutes,
      end: Math.min(minutes + length, DAY),
      lane: 0,
      lanes: 1,
    });
  }
  for (const list of Object.values(out)) {
    list.sort(
      (x, y) =>
        x.start - y.start ||
        x.appointment.uuid.localeCompare(y.appointment.uuid),
    );
    let cluster: PlacedAppointment[] = [];
    let clusterEnd = -1;
    const flush = () => {
      const lanes = Math.max(1, ...cluster.map((p) => p.lane + 1));
      for (const p of cluster) p.lanes = lanes;
      cluster = [];
    };
    for (const p of list) {
      if (p.start >= clusterEnd && cluster.length) flush();
      const laneEnds: number[] = [];
      for (const q of cluster)
        laneEnds[q.lane] = Math.max(laneEnds[q.lane] ?? 0, q.end);
      let lane = 0;
      while ((laneEnds[lane] ?? -1) > p.start) lane++;
      p.lane = lane;
      cluster.push(p);
      clusterEnd = Math.max(clusterEnd, p.end);
    }
    if (cluster.length) flush();
  }
  return out;
}

/** Visible hour span: business hours widened to fit every booking. */
export function visibleSpan(
  placed: Record<string, PlacedAppointment[]>,
  minStart = 8 * 60,
  minEnd = 20 * 60,
): { start: number; end: number } {
  let start = minStart;
  let end = minEnd;
  for (const list of Object.values(placed)) {
    for (const p of list) {
      start = Math.min(start, Math.floor(p.start / 60) * 60);
      end = Math.max(end, Math.ceil(p.end / 60) * 60);
    }
  }
  return { start, end: Math.min(end, DAY) };
}

/** Droppable slot id: `slot|YYYY-MM-DD|minutes`. */
export const slotId = (date: string, minutes: number) =>
  `slot|${date}|${minutes}`;

export function parseSlotId(
  id: string | number,
): { date: string; minutes: number } | null {
  const parts = String(id).split("|");
  if (parts.length !== 3 || parts[0] !== "slot") return null;
  const minutes = Number(parts[2]);
  if (!parts[1] || !Number.isFinite(minutes)) return null;
  return { date: parts[1], minutes };
}

/** Toast i18n key of a failed booking write. */
export function bookingErrorKey(error: unknown): string {
  if (isApiError(error)) {
    switch (error.code) {
      case "APPOINTMENT_CAPACITY_FULL":
        return "appointments.errors.capacity_full";
      case "APPOINTMENT_DAY_CLOSED":
        return "appointments.errors.day_closed";
      case "APPOINTMENT_INVALID_TRANSITION":
        return "appointments.errors.invalid_transition";
    }
  }
  return "appointments.errors.save_failed";
}
