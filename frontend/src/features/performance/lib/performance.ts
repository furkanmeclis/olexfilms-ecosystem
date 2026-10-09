import type {
  PerformanceDealerPoint,
  PerformanceMetricValue,
  PerformanceRankingRow,
  PerformanceTarget,
} from "@/features/performance/services/performance.service";

/** Monthly metric keys of the performance module (backend model.Metrics). */
export const PERFORMANCE_METRICS = [
  "services_count",
  "warranty_start_rate",
  "measurement_rate",
  "review_avg",
  "stock_turnover",
  "contract_days_left",
  "cari_overdue_amount",
  "cari_overdue_days",
  "certificate_coverage",
  "lead_conversion_rate",
  "waste_ratio",
  "order_volume",
] as const;

export type PerformanceMetric = (typeof PERFORMANCE_METRICS)[number];

export type MetricKind = "count" | "ratio" | "money" | "days" | "score";

const METRIC_KIND: Record<PerformanceMetric, MetricKind> = {
  services_count: "count",
  warranty_start_rate: "ratio",
  measurement_rate: "ratio",
  review_avg: "score",
  stock_turnover: "score",
  contract_days_left: "days",
  cari_overdue_amount: "money",
  cari_overdue_days: "days",
  certificate_coverage: "ratio",
  lead_conversion_rate: "ratio",
  waste_ratio: "ratio",
  order_volume: "money",
};

/**
 * Metrics the worker only computes while their module is on (backend
 * metricFlags): with the module off there is no value at all, so the card
 * and the column say "module off" instead of an empty value.
 */
export const METRIC_MODULE: Partial<Record<PerformanceMetric, string>> = {
  measurement_rate: "measurements",
  review_avg: "reviews",
  certificate_coverage: "certificates",
  lead_conversion_rate: "leads",
  waste_ratio: "efficiency",
};

/** Lower is better for these (a rising value is a bad trend). */
const LOWER_IS_BETTER = new Set<PerformanceMetric>([
  "cari_overdue_amount",
  "cari_overdue_days",
  "waste_ratio",
]);

/** Metrics a network target may have (performance_targets.metric). */
export const TARGET_METRICS = ["services_count", "order_volume"] as const;
export type TargetMetric = (typeof TARGET_METRICS)[number];
export const TARGET_PERIOD_KINDS = ["monthly", "quarterly", "yearly"] as const;

export const MAP_LEVELS = ["country", "province", "district"] as const;
export type MapLevel = (typeof MAP_LEVELS)[number];

/** Country of the province facet and the map (TR seed, QUESTIONS S21). */
export const PERFORMANCE_DEFAULT_COUNTRY = "TR";

export function metricKind(metric: string): MetricKind {
  return METRIC_KIND[metric as PerformanceMetric] ?? "score";
}

export function isPerformanceMetric(key: string): key is PerformanceMetric {
  return (PERFORMANCE_METRICS as readonly string[]).includes(key);
}

/**
 * Whether the metric's module is off for the organization. `enabled` null
 * (features still loading) never reports a module as off.
 */
export function metricModuleOff(
  metric: string,
  enabled: readonly string[] | null | undefined,
) {
  const key = METRIC_MODULE[metric as PerformanceMetric];
  if (!key || !enabled) return false;
  return !enabled.includes(key);
}

export function lowerIsBetter(metric: string) {
  return LOWER_IS_BETTER.has(metric as PerformanceMetric);
}

/** Decimal string → number; "" / null / garbage → null. */
export function numberValue(value: unknown): number | null {
  if (value == null || value === "") return null;
  const n = typeof value === "number" ? value : Number(value);
  return Number.isFinite(n) ? n : null;
}

export function metricNumber(value: PerformanceMetricValue | null | undefined) {
  return numberValue(value?.value);
}

export type MetricFormatters = {
  number: (value: number, options?: Intl.NumberFormatOptions) => string;
  percent: (value: number, maximumFractionDigits?: number) => string;
  currency?: (value: number, currency: string) => string;
};

/** Display text of a metric value; "—" when it has no value. */
export function formatMetric(
  metric: string,
  value: PerformanceMetricValue | number | string | null | undefined,
  format: MetricFormatters,
  currency?: string | null,
) {
  const raw =
    value != null && typeof value === "object" ? value.value : (value ?? null);
  const n = numberValue(raw);
  if (n == null) return "—";
  const cur =
    (value != null && typeof value === "object" ? value.currency : null) ??
    currency;
  switch (metricKind(metric)) {
    case "ratio":
      return format.percent(n, 1);
    case "money":
      return cur && format.currency
        ? format.currency(n, cur)
        : format.number(n, { maximumFractionDigits: 2 });
    case "count":
    case "days":
      return format.number(n, { maximumFractionDigits: 0 });
    default:
      return format.number(n, { maximumFractionDigits: 2 });
  }
}

export type DeltaTone = "up" | "down" | "flat";

/**
 * Tone of the previous-period change: "up" is good, "down" bad. Lower is
 * better for overdue and waste metrics.
 */
export function deltaTone(metric: string, deltaPct: unknown): DeltaTone {
  const n = numberValue(deltaPct);
  if (n == null || n === 0) return "flat";
  const better = lowerIsBetter(metric) ? n < 0 : n > 0;
  return better ? "up" : "down";
}

/** `YYYY-MM` of a date (local calendar). */
export function monthOf(date: Date) {
  const m = String(date.getMonth() + 1).padStart(2, "0");
  return `${date.getFullYear()}-${m}`;
}

/** The last `count` months, newest first (`YYYY-MM`). */
export function recentMonths(now: Date, count = 12) {
  const out: string[] = [];
  for (let i = 0; i < count; i++) {
    out.push(monthOf(new Date(now.getFullYear(), now.getMonth() - i, 1)));
  }
  return out;
}

/**
 * Average achievement (%) of the targets of a metric that cover the month;
 * null without such a target. A dealer sees its own targets, a center or
 * distributor the targets it set for its network.
 */
export function targetAchievement(
  targets: readonly PerformanceTarget[],
  metric: string,
  period: string,
): number | null {
  const start = `${period}-01`;
  const values = targets
    .filter(
      (t) =>
        t.metric === metric &&
        (t.period_start ?? "") <= `${period}-31` &&
        (t.period_end ?? "") >= start,
    )
    .map((t) => numberValue(t.achievement_pct))
    .filter((v): v is number => v != null);
  if (values.length === 0) return null;
  return values.reduce((a, b) => a + b, 0) / values.length;
}

/** First day of the period (`YYYY-MM` → `YYYY-MM-01`). */
export function periodStartDate(period: string) {
  return `${period}-01`;
}

/**
 * Circle radius (px) of a region marker: square-root scale of the dealer
 * count so the area follows the count, between 6 and 30 px.
 */
export function regionRadius(count: number, maxCount: number) {
  if (count <= 0 || maxCount <= 0) return 6;
  return Math.round(6 + 24 * Math.sqrt(count / maxCount));
}

const SCALE = ["#dc2626", "#f97316", "#eab308", "#84cc16", "#16a34a"];
const NO_VALUE_COLOR = "#94a3b8";

/**
 * Color of a metric value within [min, max]: red (worst) → green (best);
 * lower-is-better metrics are reversed. No value is grey.
 */
export function metricColor(
  metric: string,
  value: number | null | undefined,
  min: number,
  max: number,
) {
  if (value == null || !Number.isFinite(value)) return NO_VALUE_COLOR;
  let pos = max > min ? (value - min) / (max - min) : 1;
  pos = Math.min(1, Math.max(0, pos));
  if (lowerIsBetter(metric)) pos = 1 - pos;
  return SCALE[Math.min(SCALE.length - 1, Math.floor(pos * SCALE.length))];
}

/** Min / max of the finite values (0 / 0 when there are none). */
export function valueRange(values: readonly (number | null | undefined)[]) {
  const finite = values.filter(
    (v): v is number => v != null && Number.isFinite(v),
  );
  if (finite.length === 0) return { min: 0, max: 0 };
  return { min: Math.min(...finite), max: Math.max(...finite) };
}

export type DealerCluster = {
  id: string;
  lat: number;
  lng: number;
  dealers: PerformanceDealerPoint[];
};

/** Grid cell (degrees) of the dealer clusters per map level. */
export const CLUSTER_CELL: Record<MapLevel, number> = {
  country: 2,
  province: 0.5,
  district: 0.1,
};

/**
 * Groups dealer points into grid cells of `cell` degrees; a cluster sits on
 * the mean coordinate of its dealers. Single dealers stay their own point.
 */
export function clusterDealers(
  points: readonly PerformanceDealerPoint[],
  cell: number,
): DealerCluster[] {
  const cells = new Map<string, PerformanceDealerPoint[]>();
  for (const p of points) {
    const key = `${Math.floor(p.latitude / cell)}:${Math.floor(p.longitude / cell)}`;
    const list = cells.get(key);
    if (list) list.push(p);
    else cells.set(key, [p]);
  }
  return Array.from(cells.entries()).map(([key, dealers]) => ({
    id: dealers.length === 1 ? `dealer:${dealers[0].uuid}` : `cluster:${key}`,
    lat: dealers.reduce((a, d) => a + d.latitude, 0) / dealers.length,
    lng: dealers.reduce((a, d) => a + d.longitude, 0) / dealers.length,
    dealers,
  }));
}

function csvCell(value: string) {
  return /[",\n;]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
}

/**
 * CSV of the ranking (raw metric values, the ranking has no export job):
 * rank, organization, type, distributor, province, then every metric.
 */
export function rankingCsv(rows: readonly PerformanceRankingRow[]) {
  const header = [
    "rank",
    "organization",
    "type",
    "distributor",
    "province",
    ...PERFORMANCE_METRICS,
  ];
  const lines = rows.map((row) =>
    [
      String(row.rank ?? ""),
      row.name ?? "",
      row.type ?? "",
      row.distributor?.name ?? "",
      row.province_name ?? "",
      ...PERFORMANCE_METRICS.map((m) => row.metrics?.[m]?.value ?? ""),
    ]
      .map(csvCell)
      .join(","),
  );
  return [header.join(","), ...lines].join("\n") + "\n";
}
