import type { ServiceStatus } from "@/features/services/services/service-wizard.service";

export const SERVICE_STATUSES = [
  "draft",
  "pending",
  "processing",
  "ready",
  "completed",
  "cancelled",
] as const satisfies readonly ServiceStatus[];

const DAY = /^(\d{4})-(\d{2})-(\d{2})$/;

function localMidnight(day: string, addDays = 0): Date | null {
  const m = DAY.exec(day.trim());
  if (!m) return null;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]) + addDays);
  return Number.isNaN(d.getTime()) ? null : d;
}

function iso(d: Date): string {
  return d.toISOString().replace(/\.\d{3}Z$/, "Z");
}

/** Start of the local day as an inclusive ISO bound. */
export function dayStartIso(day: string): string | undefined {
  const d = localMidnight(day);
  return d ? iso(d) : undefined;
}

/** The next local midnight: the exclusive upper bound covering `day`. */
export function dayEndIso(day: string): string | undefined {
  const d = localMidnight(day, 1);
  return d ? iso(d) : undefined;
}

/** True when both days are set and `to` is before `from`. */
export function invalidDateRange(from: string, to: string): boolean {
  const a = localMidnight(from);
  const b = localMidnight(to);
  return Boolean(a && b && b.getTime() < a.getTime());
}

/** Number of pages for a total (at least one). */
export function pageCount(total: number, limit: number): number {
  return Math.max(1, Math.ceil(total / Math.max(1, limit)));
}
