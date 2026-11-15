/**
 * Period bar state and its report query (GET /v1/reports/{report}): a preset
 * (today, 7d, 30d, 90d, 12m) or a custom inclusive range (at most 731 days),
 * the bucket size (day / week / month) and the comparison (none, previous
 * period, same period last year). Plus the month grid of the range picker.
 * Pure, so node --test runs it.
 */

export const PRESETS = ['today', '7d', '30d', '90d', '12m'] as const;
export type Preset = (typeof PRESETS)[number];
export type Granularity = 'day' | 'week' | 'month';
export type Compare = 'none' | 'previous' | 'year';

export const GRANULARITIES: readonly Granularity[] = ['day', 'week', 'month'];
export const COMPARES: readonly Compare[] = ['none', 'previous', 'year'];

/** Longest custom range the API accepts (inclusive days). */
export const MAX_RANGE_DAYS = 731;

export type PeriodState = {
  preset: Preset | 'custom';
  /** Inclusive YYYY-MM-DD, only for `custom`. */
  from?: string;
  to?: string;
  /** null: the API picks by range. */
  granularity: Granularity | null;
  compare: Compare;
};

export const DEFAULT_PERIOD: PeriodState = { preset: '30d', granularity: null, compare: 'previous' };

const DAY = 86_400_000;
const ISO = /^\d{4}-\d{2}-\d{2}$/;

/** Local calendar date as YYYY-MM-DD. */
export function isoDay(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${d.getFullYear()}-${m}-${day}`;
}

function parse(iso: string): number {
  return ISO.test(iso) ? Date.UTC(Number(iso.slice(0, 4)), Number(iso.slice(5, 7)) - 1, Number(iso.slice(8, 10))) : NaN;
}

/** Inclusive days between two YYYY-MM-DD dates (NaN when either is invalid). */
export function rangeDays(from: string, to: string): number {
  return Math.round((parse(to) - parse(from)) / DAY) + 1;
}

export type RangeError = 'invalid' | 'reversed' | 'too_long';

/** Why a custom range would be rejected by the API, or null when it is fine. */
export function rangeError(from: string | undefined, to: string | undefined): RangeError | null {
  if (!from || !to || Number.isNaN(parse(from)) || Number.isNaN(parse(to))) return 'invalid';
  const days = rangeDays(from, to);
  if (days < 1) return 'reversed';
  if (days > MAX_RANGE_DAYS) return 'too_long';
  return null;
}

/** Days a preset covers (12 months ≈ 365). */
export function presetDays(p: Preset): number {
  return p === 'today' ? 1 : p === '7d' ? 7 : p === '30d' ? 30 : p === '90d' ? 90 : 365;
}

export function stateDays(s: PeriodState): number {
  if (s.preset === 'custom') return s.from && s.to ? Math.max(1, rangeDays(s.from, s.to)) : 30;
  return presetDays(s.preset);
}

/**
 * Bucket sizes that make a readable chart for a range: days up to a quarter,
 * weeks from two weeks to two years, months from two months.
 */
export function allowedGranularities(days: number): Granularity[] {
  const out: Granularity[] = [];
  if (days <= 92) out.push('day');
  if (days >= 14) out.push('week');
  if (days >= 60) out.push('month');
  return out.length ? out : ['day'];
}

/** The granularity sent: the chosen one when it still fits the range, else the API default (null). */
export function effectiveGranularity(s: PeriodState): Granularity | null {
  if (!s.granularity) return null;
  return allowedGranularities(stateDays(s)).includes(s.granularity) ? s.granularity : null;
}

/** What a report takes (catalog entry: periodic, granularity, compare). */
export type PeriodSupport = { periodic: boolean; granularity?: boolean; compare?: boolean };

export type PeriodQuery = {
  period?: string;
  range_from?: string;
  range_to?: string;
  granularity?: Granularity;
  compare?: Compare;
};

/**
 * Query parameters for a report. Parameters a report does not take are a 400,
 * so only what `support` allows is sent; an invalid custom range falls back to 30d.
 */
export function toQuery(s: PeriodState, support: PeriodSupport): PeriodQuery {
  if (!support.periodic) return {};
  const q: PeriodQuery = {};
  if (s.preset === 'custom' && !rangeError(s.from, s.to)) {
    q.range_from = s.from;
    q.range_to = s.to;
  } else {
    q.period = s.preset === 'custom' ? '30d' : s.preset;
  }
  const g = support.granularity ? effectiveGranularity(s) : null;
  if (g) q.granularity = g;
  if (support.compare && s.compare !== 'none') q.compare = s.compare;
  return q;
}

/** Stable key of a state for query caches. */
export function stateKey(s: PeriodState): string {
  return [s.preset, s.from ?? '', s.to ?? '', s.granularity ?? '', s.compare].join('|');
}

// ------------------------------------------------------------- range picker

export type DayCell = { iso: string; day: number; inMonth: boolean };

/**
 * Weeks of a month for a calendar grid, starting on `weekStart`
 * (0 Sunday … 6 Saturday); leading and trailing days of the neighbour months
 * fill the first and last week.
 */
export function monthGrid(year: number, month: number, weekStart = 1): DayCell[][] {
  const first = new Date(Date.UTC(year, month, 1));
  const lead = (first.getUTCDay() - weekStart + 7) % 7;
  const start = Date.UTC(year, month, 1 - lead);
  const daysIn = new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
  const cells = Math.ceil((lead + daysIn) / 7) * 7;
  const weeks: DayCell[][] = [];
  for (let i = 0; i < cells; i++) {
    const d = new Date(start + i * DAY);
    const iso = d.toISOString().slice(0, 10);
    if (i % 7 === 0) weeks.push([]);
    weeks[weeks.length - 1].push({ iso, day: d.getUTCDate(), inMonth: d.getUTCMonth() === month });
  }
  return weeks;
}

/**
 * Next range after tapping `iso`: the first tap starts a range, the second
 * closes it (swapped when earlier), a third starts over.
 */
export function tapRange(range: { from?: string; to?: string }, iso: string): { from: string; to?: string } {
  if (!range.from || range.to) return { from: iso };
  return iso < range.from ? { from: iso, to: range.from } : { from: range.from, to: iso };
}
