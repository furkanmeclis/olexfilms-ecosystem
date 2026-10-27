import type { MobileMe } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';

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
 *   screens sit in its menu. Warehouse-only staff open `depo`.
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
  if (warehouse) kinds.push('depo');
  return kinds.length ? kinds : [org.type === 'center' ? 'bayi' : 'admin'];
}

/** True when the center works in the dealer workplace with the admin screens in its menu. */
export function isCenterWorkplace(me: MobileMe | null): boolean {
  return activeMembership(me)?.type === 'center' && kindsOf(me)[0] === 'bayi';
}

/** Organizations the user can switch to; only listed when there is more than one. */
export function listOrganizations(me: MobileMe | null): Membership[] {
  return me?.organizations ?? [];
}

/** The space to open: always the primary one, there is no switcher. */
export function useResolvedSpace(): SpaceKind | null {
  const me = useSession((s) => s.me);
  return kindsOf(me)[0] ?? null;
}

export function useCan(permission: string) {
  return useSession((s) => !!s.me && (s.me.user.is_super_admin || s.me.permissions.includes(permission)));
}
