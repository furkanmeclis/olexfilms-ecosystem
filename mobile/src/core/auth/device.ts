import * as Device from 'expo-device';
import * as SecureStore from 'expo-secure-store';

import { APP_VERSION, PLATFORM } from '@/core/config';

const KEY = 'olex.device.id';
let cached: string | null = null;

function newId() {
  const rand = () => Math.random().toString(36).slice(2, 10);
  return `${PLATFORM}-${Date.now().toString(36)}-${rand()}${rand()}`;
}

/** Stable installation id: one refresh chain per device on the backend. */
export async function deviceId() {
  if (cached) return cached;
  cached = await SecureStore.getItemAsync(KEY);
  if (!cached) {
    cached = newId();
    await SecureStore.setItemAsync(KEY, cached);
  }
  return cached;
}

export async function deviceInfo() {
  return {
    id: await deviceId(),
    name: Device.deviceName ?? Device.modelName ?? undefined,
    platform: PLATFORM,
    app_version: APP_VERSION,
  } as const;
}
