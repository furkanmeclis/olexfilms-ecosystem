import type { StatusTone } from '@/ui/status-pill';

/** Decimal string from the API to a number; empty or garbage is null. */
export function num(value: string | number | null | undefined): number | null {
  if (value === null || value === undefined || value === '') return null;
  const n = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(n) ? n : null;
}

/** "+12.5%" / "-3%" / "–" for a deviation percentage. */
export function formatDeviation(value: string | number | null | undefined, locale: string): string {
  const n = num(value);
  if (n === null) return '–';
  const text = new Intl.NumberFormat(locale, { maximumFractionDigits: 1, signDisplay: 'exceptZero' }).format(n);
  return `${text}%`;
}

/**
 * A list price above the recommended one is the discipline problem the center
 * watches; one far below is a warning too. Past the threshold either way is
 * a problem, half of it is progress (watch), the rest is fine.
 */
export function deviationTone(value: string | number | null | undefined, threshold: number): StatusTone {
  const n = num(value);
  if (n === null) return 'neutral';
  const abs = Math.abs(n);
  if (threshold > 0 && abs >= threshold) return 'problem';
  if (threshold > 0 && abs >= threshold / 2) return 'progress';
  return 'done';
}

/** Default warning threshold of the web (pricing.deviation_warning_pct). */
export const DEFAULT_THRESHOLD = 15;
