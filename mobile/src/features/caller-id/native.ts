import { requireOptionalNativeModule } from 'expo';
import { Platform } from 'react-native';

import { type CallerIdStep, type CallerIdSystem, type NativeStatus, toSystem, UNSUPPORTED } from './system';

/**
 * The native side of the caller ID (`modules/caller-id`, TEC-1208). It only
 * exists in dev/store builds made after the module shipped: Expo Go, the web
 * and older builds have none and the feature stays hidden.
 *
 * iOS: the system shows the name itself from a list the Call Directory
 * extension loaded; the user switches the extension on in Settings.
 * Android: the app must be the "Caller ID & spam app" (to learn the number)
 * and may draw over other apps (to show the card).
 */
type CallerIdNative = {
  getStatus(): Promise<NativeStatus>;
  setEntries(tsv: string): Promise<{ supported: boolean; reloaded?: boolean; count?: number; errorCode?: number }>;
  clear(): Promise<unknown>;
  // iOS
  openSettings?(): Promise<boolean>;
  // Android
  requestRole?(): Promise<boolean>;
  openDefaultApps?(): Promise<boolean>;
  openOverlaySettings?(): Promise<boolean>;
  setEnabled?(enabled: boolean): Promise<boolean>;
};

const native = Platform.OS === 'web' ? null : requireOptionalNativeModule<CallerIdNative>('OlexCallerId');

export const callerIdNative = {
  /** Native module compiled into this build. */
  available: native != null,

  async getSystem(): Promise<CallerIdSystem> {
    if (!native) return UNSUPPORTED;
    try {
      return toSystem(Platform.OS, await native.getStatus());
    } catch {
      return UNSUPPORTED;
    }
  },

  /** Opens the system screen of a step. False when the system could not open it. */
  async openStep(step: CallerIdStep): Promise<boolean> {
    if (!native) return false;
    try {
      if (step === 'ios_extension') return (await native.openSettings?.()) ?? false;
      if (step === 'android_role') return (await native.requestRole?.()) ?? false;
      return (await native.openOverlaySettings?.()) ?? false;
    } catch {
      return false;
    }
  },

  /** Android: Settings > Apps > Default apps (when the role dialog no longer appears). */
  async openDefaultApps(): Promise<boolean> {
    try {
      return (await native?.openDefaultApps?.()) ?? false;
    } catch {
      return false;
    }
  },

  /** Android: the in-app switch the screening service obeys. iOS has none (the list is emptied). */
  async setEnabled(enabled: boolean): Promise<void> {
    await native?.setEnabled?.(enabled);
  },

  async setEntries(tsv: string): Promise<void> {
    await native?.setEntries(tsv);
  },

  async clear(): Promise<void> {
    await native?.clear();
  },
};
