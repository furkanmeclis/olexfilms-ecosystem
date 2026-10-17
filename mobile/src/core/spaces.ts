import { create } from 'zustand';

import type { MobileMe } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';
import { readJSON, writeJSON } from '@/core/storage';

/**
 * A space is what one membership opens in the app: its own tabs and home.
 * Feature screens are shared; a space only arranges them.
 */
export type SpaceKind = 'bayi' | 'depo' | 'admin' | 'musteri';

export type Space = {
  key: string;
  kind: SpaceKind;
  orgSlug: string;
  orgName: string;
  orgType: 'center' | 'distributor' | 'dealer';
  /** True for the organization the current token is scoped to. */
  active: boolean;
};

type Membership = NonNullable<MobileMe['organizations']>[number];

export function activeMembership(me: MobileMe | null): Membership | undefined {
  return me?.organizations?.find((o) => o.uuid === me.active_organization_uuid);
}

/** Spaces of the active organization, from its permissions. */
export function kindsOf(me: MobileMe | null): SpaceKind[] {
  // Customer and fleet accounts have their own session kind and no organization.
  if (me?.session_kind === 'customer') return ['musteri'];
  const org = activeMembership(me);
  if (!me || !org?.type) return [];
  if (org.type === 'dealer') return ['bayi'];
  const p = new Set(me.permissions);
  const kinds: SpaceKind[] = [];
  if (me.user.is_super_admin || p.has('organizations.read')) kinds.push('admin');
  // Decision 2026-10-16: the warehouse runs fully on mobile, no web needed.
  if (p.has('warehouse.read')) kinds.push('depo');
  return kinds.length ? kinds : ['admin'];
}

/**
 * Every space the user can open. Other organizations show one entry each; their
 * exact spaces are known after switching (permissions are per active org).
 */
export function listSpaces(me: MobileMe | null): Space[] {
  if (!me) return [];
  const out: Space[] = [];
  for (const org of me.organizations ?? []) {
    const type = org.type ?? 'dealer';
    const isActive = org.uuid === me.active_organization_uuid;
    const kinds: SpaceKind[] = isActive ? kindsOf(me) : [type === 'dealer' ? 'bayi' : 'admin'];
    for (const kind of kinds) {
      out.push({ key: `${org.slug}:${kind}`, kind, orgSlug: org.slug, orgName: org.name, orgType: type, active: isActive });
    }
  }
  return out;
}

type ActiveSpace = {
  kind: SpaceKind | null;
  setKind: (k: SpaceKind) => void;
};

const KEY = 'space.active';

export const useActiveSpace = create<ActiveSpace>((set) => ({
  kind: readJSON<SpaceKind>(KEY),
  setKind: (kind) => {
    writeJSON(KEY, kind);
    set({ kind });
  },
}));

/** The space to open: the last one if still allowed, else the first available. */
export function useResolvedSpace(): SpaceKind | null {
  const me = useSession((s) => s.me);
  const saved = useActiveSpace((s) => s.kind);
  const kinds = kindsOf(me);
  if (saved && kinds.includes(saved)) return saved;
  return kinds[0] ?? null;
}

export function useCan(permission: string) {
  return useSession((s) => !!s.me && (s.me.user.is_super_admin || s.me.permissions.includes(permission)));
}
