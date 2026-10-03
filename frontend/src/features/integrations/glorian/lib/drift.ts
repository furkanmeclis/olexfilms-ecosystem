/**
 * Reconcile drift report of a sync run (glorian.ReconcileCounts, TEC-271):
 * the five category counts sit flat in `counts`, `details` holds the first
 * rows of every category.
 */

export const DRIFT_CATEGORIES = [
  "only_remote",
  "only_local",
  "status_drift",
  "product_drift",
  "owner_drift",
] as const;

export type DriftCategory = (typeof DRIFT_CATEGORIES)[number];

export type DriftRow = {
  barcode: string;
  unit_id?: string;
  remote_id?: string;
  local_external_id?: string;
  local_external_status?: string;
  remote_status?: string;
  local_product_id?: string;
  remote_product_id?: string;
  local_owner_type?: string;
  local_owner_id?: number;
  expected_location?: string;
  remote_location?: string;
  remote_dealer_id?: string;
};

export type DriftReport = {
  counts: Record<DriftCategory, number>;
  details: Record<DriftCategory, DriftRow[]>;
  total: number;
  remote: number;
  local: number;
  skipped: number;
};

function num(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Reads the report out of a reconcile run's counts (tolerates gaps). */
export function parseDriftReport(counts: unknown): DriftReport {
  const c = isRecord(counts) ? counts : {};
  const rawDetails = isRecord(c.details) ? c.details : {};
  const out = {
    counts: {} as Record<DriftCategory, number>,
    details: {} as Record<DriftCategory, DriftRow[]>,
    total: 0,
    remote: num(c.remote),
    local: num(c.local),
    skipped: num(c.skipped),
  };
  for (const cat of DRIFT_CATEGORIES) {
    out.counts[cat] = num(c[cat]);
    out.total += out.counts[cat];
    const rows = rawDetails[cat];
    out.details[cat] = Array.isArray(rows)
      ? rows.filter((r): r is DriftRow => isRecord(r))
      : [];
  }
  return out;
}

/** Drift row fields in display order (only the set ones are shown). */
export const DRIFT_ROW_FIELDS = [
  "unit_id",
  "remote_id",
  "local_external_id",
  "local_external_status",
  "remote_status",
  "local_product_id",
  "remote_product_id",
  "local_owner_type",
  "local_owner_id",
  "expected_location",
  "remote_location",
  "remote_dealer_id",
] as const satisfies readonly (keyof DriftRow)[];

export type DriftRowField = (typeof DRIFT_ROW_FIELDS)[number];

/** The set fields of a row besides its barcode. */
export function driftRowEntries(row: DriftRow): [DriftRowField, string][] {
  const out: [DriftRowField, string][] = [];
  for (const f of DRIFT_ROW_FIELDS) {
    const v = row[f];
    if (v !== undefined && v !== null && v !== "" && v !== 0) {
      out.push([f, String(v)]);
    }
  }
  return out;
}

/** Plain run totals of a non-reconcile run, e.g. "created: 3". */
export function countEntries(counts: unknown): [string, number][] {
  if (!isRecord(counts)) return [];
  return Object.entries(counts).filter(
    (e): e is [string, number] => typeof e[1] === "number",
  );
}
