import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { useCan } from '@/core/spaces';

/**
 * The signed-in user's in-app inbox. Staff read the panel inbox
 * (`notifications.read`); customer and fleet sessions read their own rows on
 * the portal mirror (TEC-1021), which needs no permission.
 */
export function useInbox() {
  const portal = useSession((s) => s.me?.session_kind === 'customer');
  const canPanel = useCan('notifications.read');
  return {
    portal,
    enabled: portal || canPanel,
    list: (q: { unread?: boolean; limit: number; offset: number }) =>
      portal
        ? unwrap(api.GET('/v1/portal/notifications', { params: { query: q } }))
        : unwrap(api.GET('/v1/notifications', { params: { query: q } })),
    unreadCount: () =>
      portal ? unwrap(api.GET('/v1/portal/notifications/unread-count')) : unwrap(api.GET('/v1/notifications/unread-count')),
    read: (uuid: string) =>
      portal
        ? unwrap(api.POST('/v1/portal/notifications/{uuid}/read', { params: { path: { uuid } } }))
        : unwrap(api.POST('/v1/notifications/{uuid}/read', { params: { path: { uuid } } })),
    readAll: () => (portal ? unwrap(api.POST('/v1/portal/notifications/read-all')) : unwrap(api.POST('/v1/notifications/read-all'))),
  };
}
