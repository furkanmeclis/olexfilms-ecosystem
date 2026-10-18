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
  musteri: {},
};

/** App route for a notification in the given space, or null when none exists. */
export function notificationRoute(n: Pick<AppNotification, 'action_url' | 'payload'>, space: SpaceKind | null): string | null {
  const target = notificationTarget(n);
  if (!target || !space) return null;
  const route = ROUTES[space][target.kind]?.(target.uuid) ?? null;
  if (!route || target.kind !== 'appointments') return route;
  // Appointments have no single-get: the start time lets the screen find the booking fast.
  const at = n.payload?.starts_at ?? n.payload?.scheduled_at;
  return typeof at === 'string' ? `${route}?at=${encodeURIComponent(at)}` : route;
}
