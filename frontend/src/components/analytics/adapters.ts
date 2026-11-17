/**
 * Adapters from the report envelope v2 (TEC-1210) to the kit's props.
 * The contract types are declared here on purpose: the generated API types
 * may land later; once they do these structurally match them.
 */
import { entityHref } from "@/config/routes";

import type { KpiTileProps } from "./kpi-tile";
import {
  type Granularity,
  type ValueFormat,
  type ValueType,
  unitFormat,
} from "./format";
import {
  type CategoryDatum,
  type ChartDatum,
  type ChartSeriesDef,
  type GridColumn,
  type GridRow,
  type ReportLink,
  type StatusCell,
  previousKey,
} from "./types";

export type ReportCompare = "none" | "previous" | "year";

export type ReportPeriodV2 = {
  from: string;
  to: string;
  granularity?: Granularity | null;
  compare?: ReportCompare | null;
  previous_from?: string | null;
  previous_to?: string | null;
};

export type ReportPointV2 = {
  key: string;
  label: string;
  value: number;
  previous?: number | null;
};

export type ReportSeriesV2 = {
  key: string;
  label: string;
  unit?: string | null;
  points: ReportPointV2[];
  total?: number | null;
  previous_total?: number | null;
  delta_pct?: number | null;
};

export type ReportKpiV2 = {
  key: string;
  label: string;
  unit?: string | null;
  value: number | null;
  previous?: number | null;
  delta_pct?: number | null;
  good?: "up" | "down";
  spark?: number[] | null;
  link?: ReportLink | null;
};

export type ReportColumnV2 = {
  key: string;
  label: string;
  type?: string;
  align?: "left" | "right" | null;
  aggregate?: "sum" | "avg" | "none" | null;
  currency?: string | null;
};

export type ReportRowV2 = Record<string, unknown> & {
  _link?: ReportLink | null;
  _status?: Record<string, StatusCell> | null;
};

export type ReportEnvelopeV2 = {
  report: string;
  kind?: string;
  title?: string;
  /** v1 sent the period preset string here; v2 the resolved period. */
  period?: ReportPeriodV2 | string | null;
  granularity?: string | null;
  series?: ReportSeriesV2[] | null;
  columns?: ReportColumnV2[] | null;
  rows?: ReportRowV2[] | null;
  kpis?: ReportKpiV2[] | null;
};

const VALUE_TYPES: readonly ValueType[] = [
  "text",
  "number",
  "money",
  "percent",
  "date",
  "datetime",
  "duration",
  "status",
  "org",
  "link",
];

/** Unknown column types fall back to text (old envelopes: "string"). */
export function normalizeColumnType(
  type: string | null | undefined,
): ValueType {
  const t = (type ?? "").toLowerCase();
  if ((VALUE_TYPES as readonly string[]).includes(t)) return t as ValueType;
  if (t === "int" || t === "integer" || t === "float" || t === "count")
    return "number";
  return "text";
}

/** Resolved granularity of an envelope (v2 period object or v1 field). */
export function envelopeGranularity(
  env: Pick<ReportEnvelopeV2, "period" | "granularity">,
): Granularity | undefined {
  const raw =
    (typeof env.period === "object" && env.period
      ? env.period.granularity
      : null) ?? env.granularity;
  return raw === "day" || raw === "week" || raw === "month" || raw === "hour"
    ? raw
    : undefined;
}

/**
 * Detail page of a row / KPI link (`null` when the kind has no page).
 * TEC-1217: resolves through the shared `entityHref` registry.
 */
export function reportLinkHref(
  link: ReportLink | null | undefined,
  slug: string,
): string | null {
  return entityHref(link?.kind, link?.uuid, slug);
}

/** KPI tiles from `kpis[]`. */
export function kpisToTiles(
  kpis: readonly ReportKpiV2[] | null | undefined,
  slug = "",
): KpiTileProps[] {
  return (kpis ?? []).map((k) => ({
    id: k.key,
    label: k.label,
    value: k.value,
    previous: k.previous ?? null,
    deltaPct: k.delta_pct ?? null,
    good: k.good ?? "up",
    spark: k.spark ?? undefined,
    format: unitFormat(k.unit),
    href: reportLinkHref(k.link, slug) ?? undefined,
  }));
}

export type TimeSeriesAdapted = {
  data: ChartDatum[];
  series: ChartSeriesDef[];
  /** At least one point carries a previous-period value. */
  hasPrevious: boolean;
  format: ValueFormat;
};

/**
 * Time series (`series[]` sharing bucket keys) → chart rows. Previous
 * values land under `previousKey(series.key)`.
 */
export function timeSeriesFromReport(
  series: readonly ReportSeriesV2[] | null | undefined,
): TimeSeriesAdapted {
  const rows = new Map<string, ChartDatum>();
  let hasPrevious = false;
  const list = series ?? [];
  for (const s of list) {
    for (const p of s.points) {
      const row = rows.get(p.key) ?? { key: p.key, label: p.label };
      row[s.key] = p.value;
      if (p.previous !== undefined) {
        row[previousKey(s.key)] = p.previous;
        if (p.previous !== null) hasPrevious = true;
      }
      rows.set(p.key, row);
    }
  }
  const data = [...rows.values()].sort((a, b) =>
    a.key < b.key ? -1 : a.key > b.key ? 1 : 0,
  );
  return {
    data,
    series: list.map((s) => ({
      key: s.key,
      label: s.label,
      format: unitFormat(s.unit),
    })),
    hasPrevious,
    format: unitFormat(list[0]?.unit),
  };
}

/** One series' points → donut / ranking data (order kept). */
export function categoriesFromSeries(
  series: ReportSeriesV2 | null | undefined,
): CategoryDatum[] {
  return (series?.points ?? []).map((p) => ({
    key: p.key,
    label: p.label,
    value: p.value,
  }));
}

/** Report columns → DataGrid columns. */
export function gridColumnsFromReport(
  columns: readonly ReportColumnV2[] | null | undefined,
): GridColumn[] {
  return (columns ?? []).map((c) => {
    const type = normalizeColumnType(c.type);
    return {
      key: c.key,
      label: c.label,
      type,
      align: c.align ?? undefined,
      aggregate: c.aggregate ?? undefined,
      currency: c.currency ?? undefined,
    };
  });
}

export function gridRowsFromReport(
  rows: readonly ReportRowV2[] | null | undefined,
): GridRow[] {
  return (rows ?? []) as GridRow[];
}

export type ReportQuery = Record<string, string | number | null | undefined>;

/** BFF path of the backend export: `/api/v1/reports/{report}/export`. */
export function reportExportPath(
  report: string,
  query: ReportQuery,
  format: "csv" | "xlsx",
): string {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === null || v === undefined || v === "") continue;
    params.set(k, String(v));
  }
  params.set("format", format);
  return `/api/v1/reports/${encodeURIComponent(report)}/export?${params.toString()}`;
}
