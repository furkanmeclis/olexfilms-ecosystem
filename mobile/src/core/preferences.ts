import { Uniwind } from 'uniwind';
import { create } from 'zustand';

import { type DigitStyle, setDigitStyle } from '@/core/digits';
import { readJSON, writeJSON } from '@/core/storage';

export type ThemePreference = 'system' | 'light' | 'dark';
/** Paint thickness display unit. The backend always stores microns. */
export type ThicknessUnit = 'um' | 'mil';

type Preferences = {
  theme: ThemePreference;
  unit: ThicknessUnit;
  /** Arabic digits (TEC-1009): Arabic-Indic by default, Latin by choice. */
  digits: DigitStyle;
  setTheme: (t: ThemePreference) => void;
  setUnit: (u: ThicknessUnit) => void;
  setDigits: (d: DigitStyle) => void;
};

const KEY = 'pref.app';
const saved = readJSON<Pick<Preferences, 'theme' | 'unit' | 'digits'>>(KEY);
setDigitStyle(saved?.digits === 'latn' ? 'latn' : 'arab');

const persist = (p: Pick<Preferences, 'theme' | 'unit' | 'digits'>) => writeJSON(KEY, p);

export const usePreferences = create<Preferences>((set, get) => ({
  theme: saved?.theme ?? 'system',
  unit: saved?.unit ?? 'um',
  digits: saved?.digits === 'latn' ? 'latn' : 'arab',
  setTheme: (theme) => {
    Uniwind.setTheme(theme);
    set({ theme });
    persist({ theme, unit: get().unit, digits: get().digits });
  },
  setUnit: (unit) => {
    set({ unit });
    persist({ theme: get().theme, unit, digits: get().digits });
  },
  setDigits: (digits) => {
    setDigitStyle(digits);
    set({ digits });
    persist({ theme: get().theme, unit: get().unit, digits });
  },
}));

/** Applies the saved theme before the first frame. */
export function applyThemePreference() {
  Uniwind.setTheme(usePreferences.getState().theme);
}
