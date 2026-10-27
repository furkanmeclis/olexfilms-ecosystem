import { useSegments } from 'expo-router';

import type { MobileMe } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';
import { readJSON } from '@/core/storage';

/**
 * A space is what one membership opens in the app: its own tabs and home.
 * Feature screens are shared; a space only arranges them.
 */
export type SpaceKind = 'bayi' | 'depo' | 'admin' | 'musteri';


type Membership = NonNullable<MobileMe['organizations']>[number];

export function activeMembership(me: MobileMe | null): Membership | undefined {
  return me?.organizations?.find((o) => o.uuid === me.active_organization_uuid);
}

/**
 * Route groups the session may open, primary first. One account opens one
 * space (no switcher, TEC-1016); the extra entries are screens reached from
 * that space's menu.
 *
 * - Dealer: `bayi`.
 * - Center (TEC-1003: it also runs the AFM Florya service point): the `bayi`
 *   workplace when it creates services or manages the network; the admin
 *   screens sit in its menu. Warehouse staff (no service intake) open `depo`
 *   first.
 * - Distributor (abroad): `admin`.
 * - `depo` is added when the role reads the warehouse.
 */
export function kindsOf(me: MobileMe | null): SpaceKind[] {
  // Customer and fleet accounts have their own session kind and no organization.
  if (me?.session_kind === 'customer') return ['musteri'];
  const org = activeMembership(me);
  if (!me || !org?.type) return [];
  if (org.type === 'dealer') return ['bayi'];
  const p = new Set(me.permissions);
  const all = me.user.is_super_admin;
  const warehouse = all || p.has('warehouse.read');
  const kinds: SpaceKind[] = [];
  if (org.type === 'center') {
    if (all || p.has('services.write') || p.has('organizations.read')) kinds.push('bayi');
  } else if (all || p.has('organizations.read')) {
    kinds.push('admin');
  }
  // Decision 2026-10-16: the warehouse runs fully on mobile, no web needed.
  // Warehouse staff of the center (center_warehouse also reads organizations)
  // open the warehouse first; the workplace stays one switch away.
  if (warehouse) {
    if (org.type === 'center' && !all && !p.has('services.write')) kinds.unshift('depo');
    else kinds.push('depo');
  }
  return kinds.length ? kinds : [org.type === 'center' ? 'bayi' : 'admin'];
}

/** True when the center works in the dealer workplace with the admin screens in its menu. */
export function isCenterWorkplace(me: MobileMe | null): boolean {
  return activeMembership(me)?.type === 'center' && kindsOf(me).includes('bayi');
}

/** Organizations the user can switch to; only listed when there is more than one. */
export function listOrganizations(me: MobileMe | null): Membership[] {
  return me?.organizations ?? [];
}

/**
 * The space the user is in: the open one (workplace or warehouse, TEC-1027)
 * when it is allowed, else the primary one. Screens outside a space
 * (notifications, search) route records into it.
 */
export function useResolvedSpace(): SpaceKind | null {
  const me = useSession((s) => s.me);
  const open = (useSegments() as string[])[1] as SpaceKind | undefined;
  const kinds = kindsOf(me);
  if (open && kinds.includes(open)) return open;
  return lastSpace(me) ?? kinds[0] ?? null;
}

/** The last space of the user (TEC-1027 memory) while still allowed. */
function lastSpace(me: MobileMe | null): SpaceKind | null {
  const kinds = kindsOf(me);
  const saved = me?.user.uuid ? readJSON<{ kind: SpaceKind }>(`space.last.v1.${me.user.uuid}`) : null;
  return saved && kinds.includes(saved.kind) ? saved.kind : null;
}

export function useCan(permission: string) {
  return useSession((s) => !!s.me && (s.me.user.is_super_admin || s.me.permissions.includes(permission)));
}
