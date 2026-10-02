import type {
  ServiceListQuery,
  ServiceStatus,
} from "@/features/services/services/service-wizard.service";

export const SERVICE_STATUSES = [
  "draft",
  "pending",
  "processing",
  "ready",
  "completed",
  "cancelled",
] as const satisfies readonly ServiceStatus[];

export const ALL_STATUSES = "all";

/** Filter bar of the service list (TEC-183). Days are `YYYY-MM-DD`. */
export type ServiceListFilters = {
  status: ServiceStatus | typeof ALL_STATUSES;
  from: string;
  to: string;
  q: string;
};

export const EMPTY_SERVICE_FILTERS: ServiceListFilters = {
  status: ALL_STATUSES,
  from: "",
  to: "",
  q: "",
};

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

/**
 * Builds GET /v1/services params: empty filters are left out, the days
 * become local-day ISO bounds and a reversed range drops the dates.
 */
export function buildServiceListQuery(
  filters: ServiceListFilters,
  page: { limit: number; offset: number },
): ServiceListQuery {
  const query: ServiceListQuery = { limit: page.limit, offset: page.offset };
  const q = filters.q.trim();
  if (q) query.q = q.slice(0, 100);
  if (filters.status !== ALL_STATUSES) query.status = filters.status;
  if (!invalidDateRange(filters.from, filters.to)) {
    const from = dayStartIso(filters.from);
    const to = dayEndIso(filters.to);
    if (from) query.created_from = from;
    if (to) query.created_to = to;
  }
  return query;
}

/** Number of pages for a total (at least one). */
export function pageCount(total: number, limit: number): number {
  return Math.max(1, Math.ceil(total / Math.max(1, limit)));
}
