import * as Updates from 'expo-updates';
import { AppState } from 'react-native';
import { create } from 'zustand';

import { shouldCheckForUpdate } from './updates-policy';

type UpdateStore = {
  /** An update was downloaded; applying it needs a reload. */
  ready: boolean;
  /** Hidden for this session after the user dismisses the banner. */
  dismissed: boolean;
  dismiss: () => void;
};

export const useUpdateStore = create<UpdateStore>((set) => ({
  ready: false,
  dismissed: false,
  dismiss: () => set({ dismissed: true }),
}));

/** OTA only runs in release builds with expo-updates enabled (never dev, Expo Go or web). */
export const otaAvailable = !__DEV__ && Updates.isEnabled;

/** Where this binary pulls updates from; shown in Settings > About. */
export function getUpdateInfo() {
  return {
    enabled: otaAvailable,
    channel: Updates.channel,
    runtimeVersion: Updates.runtimeVersion,
    updateId: Updates.updateId,
    embedded: Updates.isEmbeddedLaunch,
  };
}

let lastCheckedAt: number | null = null;
let busy = false;

async function checkAndFetch() {
  if (busy || !shouldCheckForUpdate(lastCheckedAt, Date.now())) return;
  busy = true;
  lastCheckedAt = Date.now();
  try {
    const check = await Updates.checkForUpdateAsync();
    if (!check.isAvailable) return;
    const fetched = await Updates.fetchUpdateAsync();
    // A fetched update also loads on the next cold start; the banner only speeds that up.
    if (fetched.isNew) useUpdateStore.setState({ ready: true, dismissed: false });
  } catch {
    // Offline or server trouble: try again on a later foreground, never surface an error.
  } finally {
    busy = false;
  }
}

/** Applies the downloaded update now (restarts the JS bundle). */
export async function applyUpdate() {
  try {
    await Updates.reloadAsync();
  } catch {
    // Reload failed; the update still applies on the next cold start.
  }
}

/**
 * Starts the foreground update check. Returns a cleanup function. A no-op when OTA is unavailable.
 * Independent of the 426 "update required" flow: that gates on the binary version, this swaps JS only.
 */
export function startUpdateChecks(): () => void {
  if (!otaAvailable) return () => {};
  void checkAndFetch();
  const sub = AppState.addEventListener('change', (state) => {
    if (state === 'active') void checkAndFetch();
  });
  return () => sub.remove();
}
