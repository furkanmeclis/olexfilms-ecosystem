import { create } from 'zustand';

import { readJSON, writeJSON } from '@/core/storage';

type Onboarding = {
  /** The customer onboarding was finished or skipped on this device. */
  seen: boolean;
  /**
   * Call from "Skip", a last-page action, or a successful sign-in — never when
   * the screen opens. Idempotent: writes storage only on the first call.
   */
  markSeen: () => void;
};

/**
 * Device-level, not per account: sign-out keeps this key (it only clears the
 * cached profile), so the onboarding never comes back after the first run.
 * Any signed-in session also counts as seen, so installs that were already
 * signed in before the onboarding shipped (and staff) never get it later.
 */
const KEY = 'onboarding.v1';
const saved = readJSON<Pick<Onboarding, 'seen'>>(KEY);

export const useOnboarding = create<Onboarding>((set, get) => ({
  seen: saved?.seen ?? false,
  markSeen: () => {
    if (get().seen) return;
    set({ seen: true });
    writeJSON(KEY, { seen: true });
  },
}));
