import type { components } from '@/core/api/schema';
import type { SpaceKind } from '@/core/spaces';

export type AppNotification = components['schemas']['Notification'];

type Kind = 'services' | 'orders' | 'customers' | 'warranties' | 'appointments';

const UUID = '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}';
const PATH_RE = new RegExp(`/(services|orders|customers|warranties|appointments)/(${UUID})(?:[/?#]|$)`, 'i');
const PAYLOAD_KEYS: [string, Kind][] = [
  ['service_uuid', 'services'],
  ['order_uuid', 'orders'],
  ['customer_uuid', 'customers'],
  ['warranty_uuid', 'warranties'],
  ['appointment_uuid', 'appointments'],
];

/** Record a notification points at: from its action link, else from payload ids. */
export function notificationTarget(n: Pick<AppNotification, 'action_url' | 'payload'>): { kind: Kind; uuid: string } | null {
  const m = n.action_url ? PATH_RE.exec(n.action_url) : null;
  if (m) return { kind: m[1]!.toLowerCase() as Kind, uuid: m[2]! };
  for (const [key, kind] of PAYLOAD_KEYS) {
    const v = n.payload?.[key];
    if (typeof v === 'string' && new RegExp(`^${UUID}$`, 'i').test(v)) return { kind, uuid: v };
  }
  return null;
}

/** Screens that exist per space; a record kind without one stays a plain message. */
const ROUTES: Record<SpaceKind, Partial<Record<Kind, (uuid: string) => string>>> = {
  bayi: {
    services: (u) => `/bayi/services/${u}`,
    orders: (u) => `/bayi/menu/orders/${u}`,
    customers: (u) => `/bayi/menu/customers/${u}`,
    warranties: (u) => `/bayi/menu/warranties/${u}`,
    appointments: (u) => `/bayi/menu/appointments/${u}`,
  },
  depo: {
    orders: (u) => `/depo/jobs/order/${u}`,
  },
  admin: {
    services: (u) => `/admin/menu/customers/services/${u}`,
    orders: (u) => `/admin/menu/orders/${u}`,
    customers: (u) => `/admin/menu/customers/${u}`,
    warranties: (u) => `/admin/menu/customers/warranties/${u}`,
  },
  // TEC-1019: the customer space opens its own detail screens from the home stack.
  musteri: {
    services: (u) => `/musteri/service/${u}`,
    warranties: (u) => `/musteri/warranty/${u}`,
    appointments: () => '/musteri/appointments',
  },
};

/** Icon of a notification row, from the record it points at. */
export function notificationIcon(n: Pick<AppNotification, 'action_url' | 'payload'>) {
  switch (notificationTarget(n)?.kind) {
    case 'warranties':
      return 'warranty' as const;
    case 'services':
      return 'service' as const;
    case 'appointments':
      return 'appointment' as const;
    case 'orders':
      return 'order' as const;
    case 'customers':
      return 'customer' as const;
    default:
      return 'bell' as const;
  }
}

/**
 * TEC-1034: customer rows (appointments, review request, welcome, warranty
 * claim result) carry web portal paths; the customer space opens its own
 * screens for them. The first match wins.
 */
const PORTAL_ROUTES: [RegExp, (m: RegExpExecArray, n: Pick<AppNotification, 'payload'>) => string][] = [
  [new RegExp(`^/portal/services/(${UUID})#review$`, 'i'), (m) => `/musteri/service/review/${m[1]}`],
  [new RegExp(`^/portal/services/(${UUID})(?:[/?#]|$)`, 'i'), (m) => `/musteri/service/${m[1]}`],
  [/^\/portal\/appointments(?:[/?#]|$)/i, () => '/musteri/appointments'],
  [/^\/portal\/warranties(?:[/?#]|$)/i, (_m, n) => (typeof n.payload?.claim_uuid === 'string' ? '/musteri/profile/claims' : '/musteri/warranties')],
  [/^\/portal\/?(?:[?#]|$)/i, () => '/musteri'],
];

function portalRoute(n: Pick<AppNotification, 'action_url' | 'payload'>): string | null {
  const path = n.action_url?.replace(/^https?:\/\/[^/]+/i, '');
  if (!path) return null;
  for (const [re, to] of PORTAL_ROUTES) {
    const m = re.exec(path);
    if (m) return to(m, n);
  }
  return null;
}

/** App route for a notification in the given space, or null when none exists. */
export function notificationRoute(n: Pick<AppNotification, 'action_url' | 'payload'>, space: SpaceKind | null): string | null {
  if (space === 'musteri') {
    const portal = portalRoute(n);
    if (portal) return portal;
  }
  const target = notificationTarget(n);
  if (!target || !space) return null;
  return ROUTES[space][target.kind]?.(target.uuid) ?? null;
}

/**
 * Web link a customer row opens in the browser when the app has no screen for
 * it (TEC-1034: the public quote link); staff rows never leave the app.
 */
export function notificationExternalUrl(n: Pick<AppNotification, 'action_url' | 'payload'>, space: SpaceKind | null): string | null {
  if (space !== 'musteri' || !n.action_url || !/^https?:\/\//i.test(n.action_url)) return null;
  return notificationRoute(n, space) ? null : n.action_url;
}
