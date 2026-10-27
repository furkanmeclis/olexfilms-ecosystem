import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';

/**
 * Modules (feature flags) of the session. An admin can switch a module off;
 * the app then neither shows nor opens it (no tab, no menu row, no shortcut,
 * and a direct link lands on a "module closed" screen).
 *
 * Staff read `GET /v1/features` (the active organization's enabled keys, the
 * same source as the web menus). Until the answer is known everything shows,
 * so a slow or offline start never hides the app; the answer is cached on disk
 * with the other queries.
 */
export function useEnabledModules(): ReadonlySet<string> | null {
  const me = useSession((s) => s.me);
  const staff = !!me && me.session_kind !== 'customer';
  const q = useQuery({
    queryKey: ['features', me?.active_organization_uuid ?? ''],
    enabled: staff && !!me?.active_organization_uuid,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/features')),
    select: (d) => new Set(d.enabled),
  });
  return q.data ?? null;
}

/** False only when the module is known to be off; an empty key is always on. */
export function useModule(key: string | undefined): boolean {
  const enabled = useEnabledModules();
  return !key || !enabled || enabled.has(key);
}

/**
 * Module behind a screen, by the first segment of its route inside a tab stack
 * (`appointments/[uuid]`, `menu`'s `occupancy/index`, the `measure` tab ...).
 * Matches the web menu's `feature` keys (frontend/src/config/nav.ts).
 */
const ROUTE_MODULES: Record<string, string> = {
  appointments: 'appointments',
  occupancy: 'appointments',
  measure: 'measurements',
  measurements: 'measurements',
  warranty_claims: 'warranty_claims',
  claims: 'warranty_claims',
  transfers: 'dealer_transfers',
  fleets: 'fleet',
  fleet: 'fleet',
  service_subscriptions: 'service_catalog',
  catalog: 'catalog',
  orders: 'orders',
  stock: 'stock',
  accounting: 'accounting',
  disputes: 'accounting',
  einvoices: 'e_invoice',
  quick_sale: 'dealer_accounting',
  sale_prices: 'dealer_accounting',
  purchases: 'dealer_accounting',
  leads: 'leads',
  tasks: 'tasks',
  certificates: 'certificates',
  performance: 'performance',
  announcements: 'announcements',
  campaigns: 'campaigns',
  assistant: 'ai_assistant',
  ai_actions: 'ai_assistant',
};

export function moduleOfRoute(routeName: string): string | undefined {
  return ROUTE_MODULES[routeName.split('/')[0] ?? ''];
}

/** `(routeOrMenuKey) => on?` for lists of rows and shortcuts keyed like routes. */
export function useModuleFilter() {
  const enabled = useEnabledModules();
  return (key: string) => {
    const m = moduleOfRoute(key);
    return !m || !enabled || enabled.has(m);
  };
}
