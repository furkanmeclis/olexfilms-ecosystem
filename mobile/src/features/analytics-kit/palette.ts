/**
 * Chart palette (TEC-1211, same values as the web kit). The categorical order
 * is fixed: series i always gets color i, and colors never cycle. Status
 * colors are only for states (always with an icon and a label), never for a
 * series. Pure, no `@/` imports, so node --test runs it.
 */

export type ChartMode = 'light' | 'dark';

export const CATEGORICAL: Record<ChartMode, readonly string[]> = {
  light: ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#008300', '#6250d6', '#e34948'],
  dark: ['#3987e5', '#d95926', '#199e70', '#c98500', '#d55181', '#008300', '#9085e9', '#e66767'],
};

/** Most categories a chart draws with their own color; the rest fold into "other". */
export const MAX_CATEGORIES = CATEGORICAL.light.length;

export const STATUS = {
  good: '#0ca30c',
  warning: '#fab219',
  serious: '#ec835a',
  critical: '#d03b3b',
} as const;

export type StatusTone = keyof typeof STATUS;

/** Text color of a good delta (a darker green on light backgrounds for contrast). */
export const DELTA_GOOD_TEXT: Record<ChartMode, string> = { light: '#006300', dark: '#0ca30c' };
/** Text color of a bad delta. */
export const DELTA_BAD_TEXT: Record<ChartMode, string> = { light: '#b42318', dark: '#e66767' };

/**
 * Color of the i-th series. Past the palette there is no color (null): the
 * caller folds those series into "other" instead of reusing a color.
 */
export function seriesColor(index: number, mode: ChartMode): string | null {
  const list = CATEGORICAL[mode];
  return index >= 0 && index < list.length ? list[index] : null;
}

/** Hex color with an alpha (0..1), for previous-period marks and fills. */
export function withAlpha(hex: string, alpha: number): string {
  const a = Math.round(Math.min(1, Math.max(0, alpha)) * 255)
    .toString(16)
    .padStart(2, '0');
  return /^#[0-9a-f]{6}$/i.test(hex) ? `${hex}${a}` : hex;
}

/** Report row status tone (`_status[key].tone`) → status color key. */
export function statusOfTone(tone: string | null | undefined): StatusTone | null {
  switch (tone) {
    case 'done':
      return 'good';
    case 'progress':
      return 'warning';
    case 'problem':
      return 'critical';
    default:
      return null;
  }
}
