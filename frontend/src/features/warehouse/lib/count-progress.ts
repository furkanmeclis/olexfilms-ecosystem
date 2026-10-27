import type { StockCount } from "@/features/warehouse/services/warehouse.service";

/**
 * Guided progress of a count (TEC-1032, legacy "Rehberli ilerleme"):
 * counted serial units plus fixed quantity against what the ledger expects,
 * as a 0–100 percentage. Null for blind counts (no expected totals) or when
 * nothing is expected.
 */
export function countProgressPercent(count: StockCount): number | null {
  const exp = count.expected;
  const prog = count.progress;
  if (!exp || !prog) return null;
  const expected = exp.serial_units + exp.fixed_quantity;
  if (expected <= 0) return null;
  const counted = prog.serial_units + prog.fixed_quantity;
  return Math.min(100, Math.round((counted / expected) * 100));
}

/** Matched lines over every line of the report, as a percentage. */
export function countAccuracyPercent(count: StockCount): number | null {
  const s = count.summary;
  if (!s || s.lines <= 0) return null;
  const matched = s.by_result["matched"] ?? 0;
  return Math.round((matched / s.lines) * 100);
}

/** Minutes between the start and the completion of the count. */
export function countDurationMinutes(count: StockCount): number | null {
  if (!count.started_at || !count.completed_at) return null;
  const ms =
    new Date(count.completed_at).getTime() -
    new Date(count.started_at).getTime();
  if (!Number.isFinite(ms) || ms < 0) return null;
  return Math.round(ms / 60_000);
}

/** Chip tone of a count line result. */
export function countResultTone(
  result: string,
): "default" | "success" | "warning" | "danger" {
  switch (result) {
    case "matched":
      return "success";
    case "missing":
    case "unexpected":
      return "danger";
    case "wrong_location":
    case "unlocated":
    case "qty_variance":
    case "meter_variance":
      return "warning";
    default:
      return "default";
  }
}

/** Result counts in display order, zero rows dropped. */
export function countResultBadges(
  count: StockCount,
): { result: string; count: number }[] {
  const order = [
    "matched",
    "missing",
    "unexpected",
    "wrong_location",
    "unlocated",
    "qty_variance",
    "meter_variance",
  ];
  const by = count.summary?.by_result ?? {};
  const known = order
    .filter((r) => (by[r] ?? 0) > 0)
    .map((r) => ({ result: r, count: by[r] ?? 0 }));
  const extra = Object.keys(by)
    .filter((r) => !order.includes(r) && (by[r] ?? 0) > 0)
    .sort()
    .map((r) => ({ result: r, count: by[r] ?? 0 }));
  return [...known, ...extra];
}
