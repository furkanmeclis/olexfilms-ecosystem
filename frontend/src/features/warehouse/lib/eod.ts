import type { EodReport } from "@/features/warehouse/services/warehouse.service";

/** Movement groups of a report (TEC-207). */
export const EOD_GROUPS = [
  "entry",
  "placement",
  "transfer",
  "order",
  "consumption",
  "return",
  "adjustment",
  "disposal",
] as const;

/** Scope filter of the report list (`scope`, TEC-207). */
export const EOD_SCOPES = ["system", "warehouse"] as const;

/** Report sources: the nightly cron (auto) or a manual run. */
export const EOD_KINDS: EodReport["kind"][] = ["auto", "manual"];

/** Today in the given IANA time zone as YYYY-MM-DD. */
export function todayIn(timeZone: string, now: Date = new Date()): string {
  try {
    const parts = new Intl.DateTimeFormat("en-CA", {
      timeZone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).formatToParts(now);
    const get = (type: string) =>
      parts.find((p) => p.type === type)?.value ?? "";
    return `${get("year")}-${get("month")}-${get("day")}`;
  } catch {
    return now.toISOString().slice(0, 10);
  }
}

/** The backend refuses a future day (400); ISO dates compare as strings. */
export function isFutureDay(date: string, today: string): boolean {
  return date > today;
}

/** "CODE · Name" of a warehouse report, null for the system report. */
export function eodScopeLabel(report: EodReport): string | null {
  return report.warehouse
    ? `${report.warehouse.code} · ${report.warehouse.name}`
    : null;
}

/** Signed net quantity of a totals row. */
export function netQuantity(t: {
  quantity_in: number;
  quantity_out: number;
}): number {
  return t.quantity_in - t.quantity_out;
}
