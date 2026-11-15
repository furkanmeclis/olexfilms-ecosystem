/**
 * Period-over-period change: the percentage, its direction and whether the
 * direction is good for this metric (`good: up | down`, TEC-1210 `kpis[]`).
 * Pure, so node --test runs it.
 */

export type Good = 'up' | 'down';
export type Direction = 'up' | 'down' | 'flat';
export type DeltaTone = 'good' | 'bad' | 'neutral';

/** Change in percent; null when there is nothing to compare with (previous 0 or missing). */
export function deltaPct(value: number | null | undefined, previous: number | null | undefined): number | null {
  if (value === null || value === undefined || previous === null || previous === undefined) return null;
  if (!Number.isFinite(value) || !Number.isFinite(previous) || previous === 0) return null;
  return ((value - previous) / Math.abs(previous)) * 100;
}

/** Below half a percent counts as no change (the badge would read +0 %). */
export const FLAT_PCT = 0.5;

export function direction(pct: number | null | undefined): Direction | null {
  if (pct === null || pct === undefined || !Number.isFinite(pct)) return null;
  if (Math.abs(pct) < FLAT_PCT) return 'flat';
  return pct > 0 ? 'up' : 'down';
}

/** Good when the metric moved the way it should, bad the other way; flat and unknown are neutral. */
export function deltaTone(pct: number | null | undefined, good: Good | null | undefined = 'up'): DeltaTone {
  const dir = direction(pct);
  if (!dir || dir === 'flat') return 'neutral';
  return dir === (good ?? 'up') ? 'good' : 'bad';
}

/**
 * The delta as text with an explicit sign ("+12,5 %", "−3 %"), one decimal
 * under 10 %, none above. The minus is U+2212 so it never wraps from the number.
 */
export function formatDelta(pct: number, locale: string): string {
  const abs = Math.abs(pct);
  const digits = abs < 10 ? 1 : 0;
  const n = new Intl.NumberFormat(locale, { maximumFractionDigits: digits, minimumFractionDigits: 0 }).format(abs);
  const sign = direction(pct) === 'flat' ? '±' : pct > 0 ? '+' : '−';
  return `${sign}${n} %`;
}
