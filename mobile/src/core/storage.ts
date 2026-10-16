import { createMMKV } from 'react-native-mmkv';

/**
 * Synchronous key-value store for non-secret state (query cache, preferences,
 * active space). Reads are synchronous, so the first frame never waits on disk.
 * Tokens live in SecureStore, never here.
 */
export const storage = createMMKV({ id: 'olex' });

export function readJSON<T>(key: string): T | null {
  const raw = storage.getString(key);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as T;
  } catch {
    storage.remove(key);
    return null;
  }
}

export function writeJSON(key: string, value: unknown) {
  storage.set(key, JSON.stringify(value));
}

/** Adapter for TanStack Query's sync persister. */
export const queryStorage = {
  getItem: (key: string) => storage.getString(key) ?? null,
  setItem: (key: string, value: string) => storage.set(key, value),
  removeItem: (key: string) => {
    storage.remove(key);
  },
};
