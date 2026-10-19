import { useEffect } from 'react';
import { create } from 'zustand';

import { readJSON, writeJSON } from '@/core/storage';

import { getPlateRecognizer, type RecognizerStatus } from './recognizer';

/**
 * "Plaka tanıma" capability: an opt-in in Settings on top of the engine's
 * own availability. The core flows never depend on it; the scan button only
 * shows when the user turned it on and the engine is ready.
 */
type Capability = {
  optIn: boolean;
  status: RecognizerStatus | 'unknown';
  /** Download/removal in progress. */
  busy: boolean;
  /** 0–1 while the model downloads, when the engine reports it. */
  progress: number | null;
  error: boolean;
  refresh: () => Promise<void>;
  enable: () => Promise<boolean>;
  disable: () => Promise<void>;
};

const KEY = 'pref.plate_scan';
const saved = readJSON<{ optIn: boolean }>(KEY);

export const usePlateCapability = create<Capability>((set, get) => ({
  optIn: saved?.optIn ?? false,
  status: getPlateRecognizer().supported ? 'unknown' : 'unsupported',
  busy: false,
  progress: null,
  error: false,
  refresh: async () => {
    set({ status: await getPlateRecognizer().getStatus() });
  },
  enable: async () => {
    const recognizer = getPlateRecognizer();
    if (!recognizer.supported || get().busy) return false;
    set({ busy: true, progress: null, error: false });
    try {
      let status = await recognizer.getStatus();
      if (status === 'not_installed') {
        await recognizer.prepare((progress) => set({ progress }));
        status = await recognizer.getStatus();
      }
      const ok = status === 'ready';
      set({ status, optIn: ok, busy: false, progress: null, error: !ok });
      writeJSON(KEY, { optIn: ok });
      return ok;
    } catch {
      set({ busy: false, progress: null, error: true });
      return false;
    }
  },
  disable: async () => {
    set({ optIn: false, busy: true, error: false });
    writeJSON(KEY, { optIn: false });
    try {
      await getPlateRecognizer().remove();
    } catch {
      // Removal is best effort; the opt-out already hides the feature.
    }
    set({ busy: false, status: await getPlateRecognizer().getStatus() });
  },
}));

/** True when the scan button may show: opted in and the engine is ready. */
export function usePlateScanAvailable(): boolean {
  const optIn = usePlateCapability((s) => s.optIn);
  const status = usePlateCapability((s) => s.status);
  useEffect(() => {
    if (optIn && status === 'unknown') void usePlateCapability.getState().refresh();
  }, [optIn, status]);
  return optIn && status === 'ready';
}
