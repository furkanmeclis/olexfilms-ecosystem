import { router } from 'expo-router';

import type { MobileMe } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';
import { readJSON, storage, writeJSON } from '@/core/storage';

import { kindsOf, type SpaceKind } from './spaces';

/**
 * Where the user left the app (TEC-1027): the last space (workplace or
 * warehouse) and the last tab of each space, per user on this device. The app
 * reopens there; a space the user may no longer open falls back to the
 * primary one. Only the tab level is kept (`/depo/jobs`), never a record
 * screen whose record may be gone by the next launch.
 */
type Memory = { kind: SpaceKind; tabs: Partial<Record<SpaceKind, string>> };

const SPACES: readonly SpaceKind[] = ['bayi', 'depo', 'admin', 'musteri'];

const keyOf = (me: MobileMe | null) => (me?.user.uuid ? `space.last.v1.${me.user.uuid}` : null);

/** Remembers the space and tab of a signed-in path; other paths are ignored. */
export function rememberPath(me: MobileMe | null, pathname: string) {
  const key = keyOf(me);
  const [space, tab] = pathname.split('/').filter(Boolean) as [SpaceKind | undefined, string | undefined];
  if (!key || !space || !SPACES.includes(space) || !kindsOf(me).includes(space)) return;
  const prev = readJSON<Memory>(key);
  const path = tab ? `/${space}/${tab}` : `/${space}`;
  if (prev?.kind === space && prev.tabs[space] === path) return;
  writeJSON(key, { kind: space, tabs: { ...prev?.tabs, [space]: path } } satisfies Memory);
}

/** The path to open: the last space and tab while still allowed, else the primary space. */
export function startPath(me: MobileMe | null): string | null {
  const kinds = kindsOf(me);
  if (!kinds.length) return null;
  const key = keyOf(me);
  const saved = key ? readJSON<Memory>(key) : null;
  const kind = saved && kinds.includes(saved.kind) ? saved.kind : kinds[0]!;
  return saved?.tabs[kind] ?? `/${kind}`;
}

/** Opens the other space at its last tab (header switch, menu row). */
export function switchSpace(me: MobileMe | null, target: SpaceKind) {
  const key = keyOf(me);
  const saved = key ? readJSON<Memory>(key) : null;
  router.replace((saved?.tabs[target] ?? `/${target}`) as never);
}

// Sign-out (any path: button, rejected refresh) forgets the user's place;
// another account on the device keeps its own.
useSession.subscribe((s, prev) => {
  if (prev.me?.user.uuid && !s.me) {
    const key = keyOf(prev.me);
    if (key) storage.remove(key);
  }
});
