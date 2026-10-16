import { Uniwind } from 'uniwind';
import { create } from 'zustand';

import { readJSON, writeJSON } from '@/core/storage';

export type ThemePreference = 'system' | 'light' | 'dark';
/** Paint thickness display unit. The backend always stores microns. */
export type ThicknessUnit = 'um' | 'mil';

type Preferences = {
  theme: ThemePreference;
  unit: ThicknessUnit;
  setTheme: (t: ThemePreference) => void;
  setUnit: (u: ThicknessUnit) => void;
};

const KEY = 'pref.app';
const saved = readJSON<Pick<Preferences, 'theme' | 'unit'>>(KEY);

export const usePreferences = create<Preferences>((set, get) => ({
  theme: saved?.theme ?? 'system',
  unit: saved?.unit ?? 'um',
  setTheme: (theme) => {
    Uniwind.setTheme(theme);
    set({ theme });
    writeJSON(KEY, { theme, unit: get().unit });
  },
  setUnit: (unit) => {
    set({ unit });
    writeJSON(KEY, { theme: get().theme, unit });
  },
}));

/** Applies the saved theme before the first frame. */
export function applyThemePreference() {
  Uniwind.setTheme(usePreferences.getState().theme);
}
