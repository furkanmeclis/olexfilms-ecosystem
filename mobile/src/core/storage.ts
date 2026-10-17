import Storage from 'expo-sqlite/kv-store';

/**
 * Synchronous key-value store for non-secret state (query cache, preferences,
 * active space). Reads are synchronous, so the first frame never waits on disk.
 * Tokens live in SecureStore, never here.
 *
 * Backed by expo-sqlite's kv-store, which ships in Expo Go (MMKV does not).
 */
export const storage = {
  getString: (key: string) => Storage.getItemSync(key) ?? undefined,
  set: (key: string, value: string) => Storage.setItemSync(key, value),
  remove: (key: string) => {
    Storage.removeItemSync(key);
  },
};

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

/** Adapter for TanStack Query's persister. */
export const queryStorage = {
  getItem: (key: string) => storage.getString(key) ?? null,
  setItem: (key: string, value: string) => storage.set(key, value),
  removeItem: (key: string) => storage.remove(key),
};
