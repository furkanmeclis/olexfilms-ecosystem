import { create } from 'zustand';

import { readJSON, writeJSON } from '@/core/storage';

import { callerIdNative } from './native';
import type { CallerIdSystem } from './system';

/** What survives a restart. */
type Saved = {
  /** User who switched the feature on (uuid); another user on this device starts with it off. */
  enabledBy: string | null;
  /** `user:organization` the list on the device belongs to; null: no list. */
  owner: string | null;
  /** Server version of that list. */
  version: string;
  count: number;
  truncated: boolean;
  syncedAt: number | null;
};

export type CallerIdError = 'network' | 'forbidden' | 'failed';

type State = Saved & {
  /** System permissions as last read; null: not read yet. */
  system: CallerIdSystem | null;
  busy: boolean;
  error: CallerIdError | null;
  patch: (p: Partial<Saved>) => void;
  refreshSystem: () => Promise<CallerIdSystem>;
};

const KEY = 'callerid.state';
export const EMPTY_LIST = { owner: null, version: '', count: 0, truncated: false, syncedAt: null } as const;

const saved = readJSON<Saved>(KEY);

export const useCallerId = create<State>((set, get) => ({
  enabledBy: saved?.enabledBy ?? null,
  owner: saved?.owner ?? null,
  version: saved?.version ?? '',
  count: saved?.count ?? 0,
  truncated: saved?.truncated ?? false,
  syncedAt: saved?.syncedAt ?? null,
  system: null,
  busy: false,
  error: null,
  patch: (p) => {
    set(p);
    const { enabledBy, owner, version, count, truncated, syncedAt } = get();
    writeJSON(KEY, { enabledBy, owner, version, count, truncated, syncedAt } satisfies Saved);
  },
  refreshSystem: async () => {
    const system = await callerIdNative.getSystem();
    set({ system });
    return system;
  },
}));

/**
 * Every switch-off, sign-out and owner change takes a new ticket; a sync
 * that finishes with an old ticket must not write its list.
 */
let ticket = 0;
export const currentTicket = () => ticket;
export const newTicket = () => ++ticket;
