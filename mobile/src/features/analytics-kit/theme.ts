import { useTranslation } from 'react-i18next';
import { useCSSVariable, useUniwind } from 'uniwind';

import { intlLocale } from '@/core/digits';

import { CATEGORICAL, type ChartMode, DELTA_BAD_TEXT, DELTA_GOOD_TEXT } from './palette';

const str = (v: unknown, fallback: string) => (typeof v === 'string' ? v : fallback);

/**
 * Colors of the chart kit for the app's current theme (light / dark from the
 * theme system, Settings → Theme): the fixed categorical palette, and grid,
 * axis and text from the theme tokens so marks stay in front and text in the
 * text color.
 */
export function useChartTheme() {
  const { theme } = useUniwind();
  const mode: ChartMode = theme === 'dark' ? 'dark' : 'light';
  const [fg, muted, border, card] = useCSSVariable(['--color-foreground', '--color-muted-foreground', '--color-border', '--color-card']);
  const { i18n } = useTranslation();
  return {
    mode,
    palette: CATEGORICAL[mode],
    text: str(fg, mode === 'dark' ? '#e2eae4' : '#0f2d1e'),
    muted: str(muted, mode === 'dark' ? '#8d9d92' : '#55695d'),
    grid: str(border, mode === 'dark' ? '#283e32' : '#d7e1da'),
    card: str(card, mode === 'dark' ? '#12251b' : '#fcfefc'),
    good: DELTA_GOOD_TEXT[mode],
    bad: DELTA_BAD_TEXT[mode],
    /** Locale tag for Intl (Arabic digits setting included). */
    locale: intlLocale(i18n.language),
  };
}

export type ChartTheme = ReturnType<typeof useChartTheme>;
