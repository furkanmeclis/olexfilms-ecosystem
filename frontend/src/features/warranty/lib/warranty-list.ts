import type { components } from "@/generated/api";

/** Warranty list / detail row (TEC-191, GET /v1/warranties). */
export type Warranty = components["schemas"]["Warranty"];
export type WarrantyStatus = components["schemas"]["WarrantyStatus"];

export const WARRANTY_STATUSES = [
  "active",
  "expired",
  "void",
] as const satisfies readonly WarrantyStatus[];

export const ALL = "all";

/** "Ends within N days" presets of the filter bar. */
export const DAYS_LEFT_PRESETS = [7, 30, 90] as const;
export type DaysLeftPreset = (typeof DAYS_LEFT_PRESETS)[number];

const DAY_MS = 24 * 60 * 60 * 1000;

export type WarrantyProgress = {
  /** Elapsed share of the period, 0–100 (whole numbers). */
  percent: number;
  /** Whole days until the end (0 once ended; partial days round up). */
  daysLeft: number;
  /** Length of the period in whole days. */
  totalDays: number;
  /** The end is in the past. */
  ended: boolean;
};

/**
 * Elapsed share of the start → end period at `now`, clamped to 0–100, and
 * the days left. A broken period (end ≤ start) counts as fully elapsed.
 */
export function warrantyProgress(
  startAt: string | Date,
  endAt: string | Date,
  now: Date = new Date(),
): WarrantyProgress {
  const start = new Date(startAt).getTime();
  const end = new Date(endAt).getTime();
  const at = now.getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) {
    return { percent: 100, daysLeft: 0, totalDays: 0, ended: true };
  }
  const ratio = (at - start) / (end - start);
  const percent = Math.round(Math.min(1, Math.max(0, ratio)) * 100);
  const left = end - at;
  return {
    percent,
    daysLeft: left > 0 ? Math.ceil(left / DAY_MS) : 0,
    totalDays: Math.round((end - start) / DAY_MS),
    ended: left <= 0,
  };
}

/** Progress bar tone: void / ended grey, last 30 days amber, else green. */
export function progressTone(
  status: WarrantyStatus,
  p: WarrantyProgress,
): "muted" | "warning" | "success" {
  if (status !== "active" || p.ended) return "muted";
  if (p.daysLeft <= 30) return "warning";
  return "success";
}

export function warrantyStatusTone(
  status: WarrantyStatus,
): "success" | "danger" | "default" {
  if (status === "active") return "success";
  if (status === "void") return "danger";
  return "default";
}

/** Filter bar of the warranty list. */
export type WarrantyListFilters = {
  status: WarrantyStatus | typeof ALL;
  /** "Ends within N days" (only active warranties can still end). */
  endsWithin: DaysLeftPreset | typeof ALL;
  q: string;
  productUuid: string;
};

export const EMPTY_WARRANTY_FILTERS: WarrantyListFilters = {
  status: ALL,
  endsWithin: ALL,
  q: "",
  productUuid: "",
};

export type WarrantyListQuery = {
  status?: WarrantyStatus;
  q?: string;
  product_uuid?: string;
  days_left_max?: number;
  limit: number;
  offset: number;
};

/** True when any filter differs from the empty bar. */
export function hasWarrantyFilters(f: WarrantyListFilters): boolean {
  return (
    f.status !== ALL ||
    f.endsWithin !== ALL ||
    f.q.trim() !== "" ||
    f.productUuid !== ""
  );
}

/**
 * Builds GET /v1/warranties (or /v1/portal/warranties) params; empty
 * filters are left out. "Ends within N days" also asks for active
 * warranties: a voided one is no longer "ending".
 */
export function buildWarrantyListQuery(
  filters: WarrantyListFilters,
  page: { limit: number; offset: number },
): WarrantyListQuery {
  const query: WarrantyListQuery = { limit: page.limit, offset: page.offset };
  const q = filters.q.trim();
  if (q) query.q = q.slice(0, 100);
  if (filters.status !== ALL) query.status = filters.status;
  if (filters.endsWithin !== ALL) {
    query.days_left_max = filters.endsWithin;
    query.status ??= "active";
  }
  if (filters.productUuid) query.product_uuid = filters.productUuid;
  return query;
}

/** Vehicle line: brand model (year). */
export function warrantyVehicleTitle(w: Pick<Warranty, "vehicle">): string {
  const v = w.vehicle;
  const name = [v.brand_name, v.model_name].filter(Boolean).join(" ");
  return v.model_year ? `${name} (${v.model_year})` : name;
}

/** Holder name for the panel (empty in the portal). */
export function warrantyHolderName(w: Pick<Warranty, "holder">): string {
  if (!w.holder) return "";
  return [w.holder.name, w.holder.surname].filter(Boolean).join(" ");
}

/** Number of pages for a total (at least one). */
export function pageCount(total: number, limit: number): number {
  return Math.max(1, Math.ceil(total / Math.max(1, limit)));
}
