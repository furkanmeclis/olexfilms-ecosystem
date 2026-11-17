/**
 * One link registry for every record kind (TEC-1219, cross-record links).
 * `kind` uses the backend's Ref vocabulary (TEC-1216, the same words as the
 * report `_link.kind`). Pure, no `@/` value imports, so node --test runs it;
 * the hook in `use-entity-href.ts` feeds in the router segments, the space,
 * the permissions and the module switches.
 *
 * Two rules:
 * - Services, customers and warranties stay inside the current tab's stack
 *   (the old `useLinks`): the services tab and the customer / warranty menu
 *   stacks carry detail routes for the other two kinds, so a pushed screen
 *   never jumps to another tab.
 * - Every other kind opens its menu route in the space (`/<space>/menu/<route>/<id>`),
 *   or its tab when the space has one (stock, network).
 * A kind the space has no screen for, a missing permission or a switched-off
 * module gives null: the caller shows the name as plain text.
 */

export const ENTITY_KINDS = [
  'service',
  'warranty',
  'warranty_claim',
  'customer',
  'vehicle',
  'order',
  'product',
  'stock_unit',
  'transfer',
  'organization',
  'user',
  'lead',
  'appointment',
  'task',
  'measurement',
  'contract',
  'invoice',
  'cari',
  'announcement',
  'warehouse',
  'subscription',
] as const;
export type EntityKind = (typeof ENTITY_KINDS)[number];

export type Space = 'bayi' | 'depo' | 'admin' | 'musteri';

export type HrefContext = {
  /** `useSegments()`: `['(app)', '<space>', '<tab>', ...]`, or `['(app)', 'search']` outside a space. */
  segments: readonly string[];
  /** Space records open in when the screen sits outside one (search, notifications). */
  fallbackSpace: Space | null;
  can: (permission: string) => boolean;
  /** Module switch by route name (`useModuleFilter`); true when unknown. */
  routeOn: (route: string) => boolean;
  /** Organization type of the active membership (network screens: center only in the dealer workplace). */
  orgType?: string;
};

const SPACES: readonly string[] = ['bayi', 'depo', 'admin', 'musteri'];

type StackKind = 'services' | 'customers' | 'warranties';

/**
 * Stack-local path of a service, customer or warranty (the old `useLinks`,
 * unchanged for the dealer workplace). The admin space has no services tab, so
 * its records open in the customers menu stack, like the search screen did.
 */
export function stackPath(segments: readonly string[], kind: StackKind, id: string): string {
  const space = segments[1] ?? 'bayi';
  const tab = segments[2];
  let base = space === 'admin' ? '/admin/menu/customers' : `/${space}/services`;
  let self: StackKind = space === 'admin' ? 'customers' : 'services';
  if (tab === 'menu' && (segments[3] === 'customers' || segments[3] === 'warranties')) {
    base = `/${space}/menu/${segments[3]}`;
    self = segments[3];
  }
  return kind === self ? `${base}/${id}` : `${base}/${kind}/${id}`;
}

/**
 * Menu route of a kind in a space without the permission / module check, for
 * screens that are already behind it (the old per-module link hooks).
 */
export function menuRoutePath(space: string, kind: 'order' | 'lead' | 'warranty_claim' | 'cari' | 'invoice', id: string): string {
  return `/${space}/menu/${MENU_ROUTES[kind]!.route}/${id}`;
}

/** Canonical path from outside a space (search): what the search screen opened before. */
function outsidePath(space: Space, kind: StackKind, id: string): string {
  if (space === 'admin') return stackPath(['(app)', 'admin'], kind, id);
  if (kind === 'services') return `/${space}/services/${id}`;
  return `/${space}/menu/${kind}/${id}`;
}

type MenuRoute = { route: string; spaces: Space[]; permission: string | string[] };

/** Kinds with a `/<space>/menu/<route>/[uuid]` screen. */
const MENU_ROUTES: Partial<Record<EntityKind, MenuRoute>> = {
  order: { route: 'orders', spaces: ['bayi', 'depo', 'admin'], permission: 'orders.read' },
  lead: { route: 'leads', spaces: ['bayi', 'admin'], permission: 'leads.read' },
  warranty_claim: { route: 'warranty_claims', spaces: ['bayi', 'admin'], permission: 'warranty_claims.read' },
  cari: { route: 'accounting', spaces: ['bayi', 'admin'], permission: 'accounting.read' },
  invoice: { route: 'einvoices', spaces: ['bayi', 'admin'], permission: 'einvoice.read' },
  task: { route: 'tasks', spaces: ['bayi', 'admin'], permission: 'tasks.read' },
  appointment: { route: 'appointments', spaces: ['bayi'], permission: 'appointments.read' },
  measurement: { route: 'measurements', spaces: ['bayi', 'admin'], permission: 'measurements.read' },
  announcement: { route: 'announcements', spaces: ['bayi', 'depo', 'admin'], permission: 'announcements.read' },
  subscription: { route: 'service_subscriptions', spaces: ['bayi', 'admin'], permission: 'service_subscriptions.read' },
  transfer: { route: 'transfers', spaces: ['bayi'], permission: 'transfers.request' },
};

const STACK_PERMISSION: Record<StackKind, string> = {
  services: 'services.read',
  customers: 'customers.read',
  warranties: 'warranties.read',
};

const STACK_KIND: Partial<Record<EntityKind, StackKind>> = {
  service: 'services',
  customer: 'customers',
  warranty: 'warranties',
};

function allowed(ctx: HrefContext, p: string | string[]) {
  return [p].flat().some((k) => ctx.can(k));
}

/** The space of the current screen, or the fallback when it sits outside one. */
export function spaceOf(ctx: Pick<HrefContext, 'segments' | 'fallbackSpace'>): { space: Space | null; inside: boolean } {
  const s = ctx.segments[1];
  if (s && SPACES.includes(s)) return { space: s as Space, inside: true };
  return { space: ctx.fallbackSpace, inside: false };
}

/**
 * Detail path of a record for the viewer, or null when the app has no screen
 * for it there or the viewer may not open it. `id` is the record uuid, except
 * `stock_unit`, whose screen is keyed by barcode.
 */
export function entityPath(ctx: HrefContext, kind: EntityKind, id: string | null | undefined): string | null {
  if (!id) return null;
  const { space, inside } = spaceOf(ctx);
  // The customer space has its own links (customer-space/lib.ts).
  if (!space || space === 'musteri') return null;
  const seg = ctx.segments;
  const e = encodeURIComponent(id);

  const stack = STACK_KIND[kind];
  if (stack) {
    if (space === 'depo' || !ctx.can(STACK_PERMISSION[stack])) return null;
    return inside ? stackPath(seg, stack, e) : outsidePath(space, stack, e);
  }

  const menu = MENU_ROUTES[kind];
  if (menu) {
    if (!menu.spaces.includes(space) || !allowed(ctx, menu.permission) || !ctx.routeOn(menu.route)) return null;
    // Center sales staff have an orders tab in the dealer workplace: stay in it.
    if (kind === 'order' && inside && seg[2] === 'orders') return `/${space}/orders/${e}`;
    return `/${space}/menu/${menu.route}/${e}`;
  }

  switch (kind) {
    case 'organization': {
      if (!ctx.can('organizations.read')) return null;
      if (space === 'admin') return `/admin/network/${e}`;
      // The center works in the dealer workplace with the network in its menu (TEC-1016).
      if (space === 'bayi' && ctx.orgType === 'center') return `/bayi/menu/network/${e}`;
      return null;
    }
    case 'product': {
      // The dealer's catalog shows the product card; elsewhere the organization's units of it.
      if (space === 'bayi' && ctx.can('catalog.read') && ctx.routeOn('catalog')) return `/bayi/menu/catalog/products/${e}`;
      if (!ctx.can('stock.read') || !ctx.routeOn('stock')) return null;
      return `${stockBase(space, inside, seg)}/${e}`;
    }
    case 'stock_unit': {
      if (!ctx.can('stock.read') || !ctx.routeOn('stock')) return null;
      return `${stockBase(space, inside, seg)}/unit/${e}`;
    }
    default:
      // vehicle, user, contract, warehouse: no mobile screen yet.
      return null;
  }
}

/** Stock screens: the stock tab (dealer, warehouse) or the menu's stock stack (admin, or when already in it). */
function stockBase(space: Space, inside: boolean, seg: readonly string[]): string {
  if (inside && seg[2] === 'menu' && seg[3] === 'stock') return `/${space}/menu/stock`;
  return space === 'admin' ? '/admin/menu/stock' : `/${space}/stock`;
}

/** Concept icon of a kind (RefRow, related lists). Names match `@/ui/icons`. */
export const KIND_ICONS: Record<EntityKind, string> = {
  service: 'service',
  warranty: 'warranty',
  warranty_claim: 'warrantyClaim',
  customer: 'customer',
  vehicle: 'vehicle',
  order: 'order',
  product: 'package',
  stock_unit: 'barcode',
  transfer: 'transfer',
  organization: 'building',
  user: 'staff',
  lead: 'lead',
  appointment: 'appointment',
  task: 'task',
  measurement: 'measure',
  contract: 'report',
  invoice: 'report',
  cari: 'accounting',
  announcement: 'announcement',
  warehouse: 'warehouse',
  subscription: 'subscription',
};

/** Search hit spec (`SearchHit.spec`) → kind. Stock units are left out: their screen is keyed by barcode, the hit by uuid. */
export const SEARCH_SPEC_KINDS: Record<string, EntityKind> = {
  services: 'service',
  customers: 'customer',
  warranties: 'warranty',
  orders: 'order',
  organizations: 'organization',
  leads: 'lead',
  products: 'product',
  vehicles: 'vehicle',
};
