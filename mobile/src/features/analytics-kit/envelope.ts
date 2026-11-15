/**
 * Report envelope v2 (TEC-1210) as the kit reads it. Every v2 field is
 * optional here, so the kit also renders an older envelope (no comparison,
 * KPI cards in the `cards` series). Pure, so node --test runs it.
 */

import type { Good } from './delta.ts';
import type { Column, Row } from './table.ts';

export type LinkKind = 'service' | 'order' | 'customer' | 'organization' | 'warranty' | 'measurement' | 'product';
/** A record (kind + uuid) or, for a KPI, a detail report (kind `report` + its key). */
export type ReportLink = { kind: LinkKind | 'report' | string; uuid?: string; report?: string };

export type Point = { key: string; label: string; value: number; previous?: number | null };

export type Series = {
  key: string;
  label: string;
  unit: string;
  points: Point[];
  total?: number | null;
  previous_total?: number | null;
  delta_pct?: number | null;
};

export type Kpi = {
  key: string;
  label: string;
  unit?: string | null;
  value: number | null;
  previous?: number | null;
  delta_pct?: number | null;
  good?: Good | null;
  spark?: number[] | null;
  link?: ReportLink | null;
};

export type PeriodInfo = {
  from?: string | null;
  to?: string | null;
  granularity?: string | null;
  compare?: string | null;
  previous_from?: string | null;
  previous_to?: string | null;
};

export type Envelope = {
  report: string;
  kind: string;
  title: string;
  period?: unknown;
  period_info?: PeriodInfo | null;
  range_from?: string | null;
  range_to?: string | null;
  granularity?: string | null;
  series: Series[];
  columns: Column[];
  rows: Row[];
  kpis?: Kpi[] | null;
};

const DATE_KEY = /^\d{4}-\d{2}(-\d{2})?/;

/** A series over time: more than one point and every key is a date (bucket start). */
export function isTimeSeries(s: Pick<Series, 'points'>): boolean {
  return s.points.length > 1 && s.points.every((p) => DATE_KEY.test(p.key));
}

/** Whether any point carries a previous-period value (compare was asked and answered). */
export function hasPrevious(series: readonly Series[]): boolean {
  return series.some((s) => s.points.some((p) => p.previous !== null && p.previous !== undefined));
}

/** Sum of a series' points; the API's `total` wins when present. */
export function seriesTotal(s: Series): number {
  if (typeof s.total === 'number') return s.total;
  return s.points.reduce((a, p) => a + (Number.isFinite(p.value) ? p.value : 0), 0);
}

/**
 * KPI tiles of a report: `kpis[]` (v2), else the points of the legacy
 * `cards` series (value only, no comparison).
 */
export function kpisOf(env: Pick<Envelope, 'kpis' | 'series'>): Kpi[] {
  if (env.kpis && env.kpis.length) return env.kpis;
  const cards = env.series.find((s) => s.key === 'cards');
  return (cards?.points ?? []).map((p) => ({ key: p.key, label: p.label, value: p.value, previous: p.previous ?? null }));
}

/** Series to chart: everything but the legacy `cards` series and empty ones. */
export function chartSeries(env: Pick<Envelope, 'series'>): Series[] {
  return env.series.filter((s) => s.key !== 'cards' && s.points.length > 0);
}

/**
 * Time series that share the same buckets (same keys in the same order) and
 * the same unit, so they can share one chart and one y axis. Others stand alone.
 */
export function groupTimeSeries(series: readonly Series[]): Series[][] {
  const groups: Series[][] = [];
  for (const s of series) {
    if (!isTimeSeries(s)) continue;
    const sig = s.points.map((p) => p.key).join(',');
    const g = groups.find((x) => x[0].unit === s.unit && x[0].points.map((p) => p.key).join(',') === sig);
    if (g) g.push(s);
    else groups.push([s]);
  }
  return groups;
}

/** The comparison range of the answer, wherever the envelope carries it. */
export function previousRange(env: { period?: unknown; period_info?: unknown } & Record<string, unknown>): { from: string; to: string } | null {
  const info = env.period_info ?? env.period;
  const p = info && typeof info === 'object' ? (info as PeriodInfo) : (env as PeriodInfo);
  return p.previous_from && p.previous_to ? { from: p.previous_from, to: p.previous_to } : null;
}

/** Detail link of a row (`_link`), when the report gives one. */
export function rowLink(row: Row): ReportLink | null {
  const l = row._link as ReportLink | undefined | null;
  return l && typeof l === 'object' && typeof l.kind === 'string' && (typeof l.uuid === 'string' || typeof l.report === 'string') ? l : null;
}

/** Status label and tone of a status cell (`_status[columnKey]`, else the raw value as label). */
export function rowStatus(row: Row, key: string): { label: string; tone: string } | null {
  const map = row._status as Record<string, { label?: string; tone?: string }> | undefined;
  const s = map && typeof map === 'object' ? map[key] : undefined;
  if (s && typeof s.label === 'string') return { label: s.label, tone: s.tone ?? 'neutral' };
  const raw = row[key];
  return raw === null || raw === undefined || raw === '' ? null : { label: String(raw), tone: 'neutral' };
}
