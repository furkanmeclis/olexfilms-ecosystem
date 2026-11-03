// Tasarım CSS değişkenlerinin widget karşılığı. Saf dosya.
import type { Palette, WidgetTheme } from './types.ts';

// İki temada da aynı olan jetonlar.
const SHARED = {
  onGreen: '#f8fbf9',
  onGreenMuted: '#c9dacf',
  amber: '#f2ac4a',
  plate: '#ffffff',
  plateFg: '#000000',
  plateStrip: '#1f4fb4',
} as const;

export const LIGHT_PALETTE: Palette = {
  card: '#fcfefc',
  fg: '#0f2d1e',
  pri: '#003d25',
  prifg: '#f8fbf9',
  sec: '#e2f0e7',
  muted: '#eaf3ed',
  track: '#d7e1da',
  mfg: '#55695d',
  bd: '#d7e1da',
  warn: '#e17100',
  warnbg: '#fbead6',
  ok: '#009966',
  green: '#003d25',
  ...SHARED,
};

export const DARK_PALETTE: Palette = {
  card: '#12251b',
  fg: '#e2eae4',
  pri: '#61c28e',
  prifg: '#002112',
  sec: '#1e3328',
  muted: '#182a20',
  track: '#283e32',
  mfg: '#8d9d92',
  bd: '#283e32',
  warn: '#ffb900',
  warnbg: '#33301a',
  ok: '#00d492',
  green: '#0b3a25',
  ...SHARED,
};

export const WIDGET_THEME: WidgetTheme = { light: LIGHT_PALETTE, dark: DARK_PALETTE };
