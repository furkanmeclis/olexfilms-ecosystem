import * as SecureStore from 'expo-secure-store';

import type { components } from '@/core/api/schema';

export type Tokens = {
  access: string;
  refresh: string;
  /** Epoch ms when the access token expires. */
  accessExpiresAt: number;
};

const KEY = 'olex.auth.tokens';
let current: Tokens | null = null;
const listeners = new Set<(t: Tokens | null) => void>();

export function fromApi(t: components['schemas']['Tokens']): Tokens {
  return {
    access: t.access_token,
    refresh: t.refresh_token,
    accessExpiresAt: Date.now() + t.expires_in * 1000,
  };
}

export function getTokens() {
  return current;
}

/** Reads the stored pair once at launch (before the splash hides). */
export async function loadTokens(): Promise<Tokens | null> {
  try {
    const raw = await SecureStore.getItemAsync(KEY);
    current = raw ? (JSON.parse(raw) as Tokens) : null;
  } catch {
    current = null;
  }
  return current;
}

export async function setTokens(t: Tokens) {
  current = t;
  listeners.forEach((fn) => fn(t));
  await SecureStore.setItemAsync(KEY, JSON.stringify(t));
}

export async function clearTokens() {
  current = null;
  listeners.forEach((fn) => fn(null));
  await SecureStore.deleteItemAsync(KEY);
}

export function onTokensChange(fn: (t: Tokens | null) => void) {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}
