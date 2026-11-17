import { create } from 'zustand';

import { readJSON, writeJSON } from '@/core/storage';

/**
 * TEC-1230: how codes are read. `camera` is the phone camera; `wedge` is a
 * hardware reader (Bluetooth / USB scanner, Zebra or Honeywell terminal)
 * that types the code as keystrokes followed by Enter ("keyboard wedge").
 * Zebra DataWedge / Honeywell intents need a native module and are not
 * used: set the terminal's profile to keystroke output instead.
 */
export type ScannerInputMode = 'camera' | 'wedge';

export const SCANNER_INPUT_MODES: ScannerInputMode[] = ['camera', 'wedge'];

const KEY = 'pref.scanner';

type State = {
  mode: ScannerInputMode;
  setMode: (m: ScannerInputMode) => void;
};

const saved = readJSON<{ mode?: ScannerInputMode }>(KEY);

export const useScannerInput = create<State>((set) => ({
  mode: saved?.mode === 'wedge' ? 'wedge' : 'camera',
  setMode: (mode) => {
    set({ mode });
    writeJSON(KEY, { mode });
  },
}));
