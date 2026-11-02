import { requireOptionalNativeModule } from 'expo';
import { NativeModules, TurboModuleRegistry } from 'react-native';

/**
 * Lazy access to the native modules shake-to-report needs. Builds made before
 * this feature lack expo-sensors, react-native-view-shot and expo-sharing, so
 * none of them is imported statically: each is looked up first (a registry
 * lookup, no JS package load) and only then required, inside try/catch. With
 * the feature off nothing here runs.
 */
type SensorsModule = typeof import('expo-sensors');
type ViewShotModule = typeof import('react-native-view-shot');
type SharingModule = typeof import('expo-sharing');
type NetworkModule = typeof import('expo-network');

/** True when this binary ships every native module the feature uses. Loads nothing. */
export function feedbackSupported(): boolean {
  try {
    return (
      !!requireOptionalNativeModule('ExponentAccelerometer') &&
      !!requireOptionalNativeModule('ExpoSharing') &&
      !!(TurboModuleRegistry.get('RNViewShot') ?? NativeModules.RNViewShot)
    );
  } catch {
    return false;
  }
}

function load<T>(present: () => boolean, get: () => T): T | null {
  try {
    return present() ? get() : null;
  } catch {
    return null;
  }
}

export function loadSensors(): SensorsModule | null {
  return load(
    () => !!requireOptionalNativeModule('ExponentAccelerometer'),
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    () => require('expo-sensors') as SensorsModule,
  );
}

export function loadViewShot(): ViewShotModule | null {
  return load(
    () => !!(TurboModuleRegistry.get('RNViewShot') ?? NativeModules.RNViewShot),
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    () => require('react-native-view-shot') as ViewShotModule,
  );
}

export function loadSharing(): SharingModule | null {
  return load(
    () => !!requireOptionalNativeModule('ExpoSharing'),
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    () => require('expo-sharing') as SharingModule,
  );
}

/** expo-network is in older builds too; still optional, metadata only. */
export function loadNetwork(): NetworkModule | null {
  return load(
    () => !!requireOptionalNativeModule('ExpoNetwork'),
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    () => require('expo-network') as NetworkModule,
  );
}
