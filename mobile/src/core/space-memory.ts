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

/**
 * Tab roots of each space. Screens of the home tab drop their group from the
 * path (`/musteri/service/<uuid>`), so a second segment is not always a tab.
 */
const TABS: Record<SpaceKind, readonly string[]> = {
  bayi: ['services', 'scan', 'measure', 'stock', 'menu'],
  depo: ['jobs', 'scan', 'stock', 'menu'],
  admin: ['approvals', 'scan', 'network', 'menu'],
  // A remembered `warranties` (the old tab) is not listed and falls back to `/musteri`.
  musteri: ['services', 'appointments', 'fleet', 'profile'],
};

/** The path of a remembered tab; anything else is the space's home. */
const tabPath = (space: SpaceKind, tab: string | undefined) =>
  tab && TABS[space].includes(tab) ? `/${space}/${tab}` : `/${space}`;

const keyOf = (me: MobileMe | null) => (me?.user.uuid ? `space.last.v1.${me.user.uuid}` : null);

/** Remembers the space and tab of a signed-in path; other paths are ignored. */
export function rememberPath(me: MobileMe | null, pathname: string) {
  const key = keyOf(me);
  const [space, tab] = pathname.split('/').filter(Boolean) as [SpaceKind | undefined, string | undefined];
  if (!key || !space || !SPACES.includes(space) || !kindsOf(me).includes(space)) return;
  const prev = readJSON<Memory>(key);
  const path = tabPath(space, tab);
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
  return tabPath(kind, saved?.tabs[kind]?.split('/')[2]);
}

/** Opens the other space at its last tab (header switch, menu row). */
export function switchSpace(me: MobileMe | null, target: SpaceKind) {
  const key = keyOf(me);
  const saved = key ? readJSON<Memory>(key) : null;
  router.replace(tabPath(target, saved?.tabs[target]?.split('/')[2]) as never);
}

// Sign-out (any path: button, rejected refresh) forgets the user's place;
// another account on the device keeps its own.
useSession.subscribe((s, prev) => {
  if (prev.me?.user.uuid && !s.me) {
    const key = keyOf(prev.me);
    if (key) storage.remove(key);
  }
});
