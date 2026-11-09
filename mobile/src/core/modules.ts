import { useQuery } from '@tanstack/react-query';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';

/**
 * Modules (feature flags) of the session. An admin can switch a module off;
 * the app then neither shows nor opens it (no tab, no menu row, no shortcut,
 * and a direct link lands on a "module closed" screen).
 *
 * Staff read `GET /v1/features` (the active organization's enabled keys, the
 * same source as the web menus). Customers read `GET /v1/portal/features`
 * (TEC-1023), which answers only the customer-facing modules in
 * `PORTAL_MODULES`; every other key stays on for them. Until the answer is
 * known everything shows, so a slow or offline start never hides the app; the
 * answer is cached on disk with the other queries.
 */
const PORTAL_MODULES: ReadonlySet<string> = new Set(['appointments', 'warranty_claims', 'reviews', 'fleet']);

function useModuleCheck(): (module: string | undefined) => boolean {
  const me = useSession((s) => s.me);
  const customer = me?.session_kind === 'customer';
  const staff = useQuery({
    queryKey: ['features', me?.active_organization_uuid ?? ''],
    enabled: !!me && !customer && !!me.active_organization_uuid,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/features')),
    select: (d) => new Set<string>(d.enabled),
  });
  const portal = useQuery({
    queryKey: ['features', 'portal'],
    enabled: !!me && customer,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/portal/features')),
    select: (d) => new Set<string>(d.enabled),
  });
  return (module) => {
    if (!module) return true;
    if (customer) return !PORTAL_MODULES.has(module) || !portal.data || portal.data.has(module);
    return !staff.data || staff.data.has(module);
  };
}

/** False only when the module is known to be off; an empty key is always on. */
export function useModule(key: string | undefined): boolean {
  return useModuleCheck()(key);
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
  accounting_queue: 'accounting',
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
  // The customer's review form sits under a service (`service/review/[uuid]`).
  if (routeName.includes('review/')) return 'reviews';
  return ROUTE_MODULES[routeName.split('/')[0] ?? ''];
}

/** `(routeOrMenuKey) => on?` for lists of rows and shortcuts keyed like routes. */
export function useModuleFilter() {
  const on = useModuleCheck();
  return (key: string) => on(moduleOfRoute(key));
}
