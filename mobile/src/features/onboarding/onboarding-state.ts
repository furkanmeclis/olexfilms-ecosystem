import { create } from 'zustand';

import { readJSON, writeJSON } from '@/core/storage';

type Onboarding = {
  /** The customer onboarding was finished or skipped on this device. */
  seen: boolean;
  /** Call only from "Skip" or a last-page action, never when the screen opens. */
  markSeen: () => void;
};

/**
 * Device-level, not per account: sign-out keeps this key (it only clears the
 * cached profile), so the onboarding never comes back after the first run.
 */
const KEY = 'onboarding.v1';
const saved = readJSON<Pick<Onboarding, 'seen'>>(KEY);

export const useOnboarding = create<Onboarding>((set) => ({
  seen: saved?.seen ?? false,
  markSeen: () => {
    set({ seen: true });
    writeJSON(KEY, { seen: true });
  },
}));
