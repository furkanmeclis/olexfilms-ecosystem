import type {
  FleetPlanAppointment,
  FleetPlanWarning,
} from "@/features/fleets/services/fleets.service";

const DAY_MS = 24 * 60 * 60 * 1000;

/** YYYY-MM-DD of an instant in the time zone. */
export function dayInZone(iso: string, timeZone: string): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date(iso));
  const get = (type: string) => parts.find((p) => p.type === type)?.value;
  return `${get("year")}-${get("month")}-${get("day")}`;
}

/**
 * Moves an appointment to another day keeping its wall-clock time: the
 * instant shifts by the whole days between the old and the new day.
 */
export function moveToDay(iso: string, day: string, timeZone: string) {
  const from = Date.parse(`${dayInZone(iso, timeZone)}T00:00:00Z`);
  const to = Date.parse(`${day}T00:00:00Z`);
  if (Number.isNaN(to)) return iso;
  const days = Math.round((to - from) / DAY_MS);
  return new Date(Date.parse(iso) + days * DAY_MS).toISOString();
}

/** Day → appointment count of the plan. */
export function countByDay(
  appointments: FleetPlanAppointment[],
  timeZone: string,
): Map<string, number> {
  const counts = new Map<string, number>();
  for (const a of appointments) {
    const day = dayInZone(a.starts_at, timeZone);
    counts.set(day, (counts.get(day) ?? 0) + 1);
  }
  return counts;
}

export type PlanRowWarning =
  | { kind: "capacity"; day: string; count: number; limit: number }
  | { kind: "server"; code: string; message: string };

/**
 * Warnings of one preview row: the server's (another active appointment of
 * the vehicle that day, …) and the daily limit exceeded by an edited day.
 */
export function rowWarnings(
  row: FleetPlanAppointment,
  appointments: FleetPlanAppointment[],
  serverWarnings: FleetPlanWarning[],
  dailyMax: number | undefined,
  timeZone: string,
): PlanRowWarning[] {
  const day = dayInZone(row.starts_at, timeZone);
  const out: PlanRowWarning[] = serverWarnings
    .filter((w) => w.vehicle_uuid === row.vehicle_uuid && w.date === day)
    .map((w) => ({ kind: "server", code: w.code, message: w.message }));
  if (dailyMax && dailyMax > 0) {
    const count = countByDay(appointments, timeZone).get(day) ?? 0;
    if (count > dailyMax) {
      out.push({ kind: "capacity", day, count, limit: dailyMax });
    }
  }
  return out;
}

/** Every capacity / server warning of the preview, for the summary. */
export function planWarningCount(
  appointments: FleetPlanAppointment[],
  serverWarnings: FleetPlanWarning[],
  dailyMax: number | undefined,
  timeZone: string,
): number {
  return appointments.reduce(
    (n, row) =>
      n +
      rowWarnings(row, appointments, serverWarnings, dailyMax, timeZone).length,
    0,
  );
}

/** Preview vehicles the server could not schedule (no free day). */
export function unscheduledVehicles(
  requested: string[],
  appointments: FleetPlanAppointment[],
): string[] {
  const planned = new Set(appointments.map((a) => a.vehicle_uuid));
  return requested.filter((uuid) => !planned.has(uuid));
}

/** "HH:MM" list from the free text field ("09:00, 14:30"). */
export function parsePreferredTimes(raw: string): string[] | null {
  const items = raw
    .split(/[,\s]+/)
    .map((s) => s.trim())
    .filter(Boolean);
  if (items.some((s) => !/^[0-2][0-9]:[0-5][0-9]$/.test(s))) return null;
  return items;
}

/** Latest closed month (YYYY-MM) and quarter (YYYY-Qn) before today. */
export function lastClosedPeriods(today: Date) {
  const y = today.getFullYear();
  const m = today.getMonth();
  const prevMonth = new Date(y, m - 1, 1);
  const month = `${prevMonth.getFullYear()}-${String(prevMonth.getMonth() + 1).padStart(2, "0")}`;
  const q = Math.floor(m / 3);
  const quarter = q === 0 ? `${y - 1}-Q4` : `${y}-Q${q}`;
  return { month, quarter };
}
