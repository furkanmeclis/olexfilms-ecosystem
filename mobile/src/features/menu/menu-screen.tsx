import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import type { SpaceKind } from '@/core/spaces';
import type { IconName } from '@/ui/icons';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';

type Item = { key: string; icon: IconName; permission?: string | string[]; spaces: SpaceKind[]; built?: SpaceKind[] };

/**
 * Menu rows appear only with the permission (and so the module) behind them:
 * a closed module is not drawn at all. Rows open their screens as they are built.
 */
const ITEMS: Item[] = [
  { key: 'customers', icon: 'customers', permission: 'customers.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  // TEC-866: no warranty list row for dealers (bayi); warranties open from the service detail.
  { key: 'appointments', icon: 'appointment', permission: 'appointments.read', spaces: ['bayi'], built: ['bayi'] },
  // TEC-958: the dealer has its stock tab; the admin space gets the center's own stock (read only).
  { key: 'stock', icon: 'stock', permission: 'stock.read', spaces: ['bayi', 'admin'], built: ['admin'] },
  // TEC-958: dealer sales (web Dealer sales; accounting + dealer_accounting modules behind each permission).
  { key: 'quick_sale', icon: 'cart', permission: 'product_sales.write', spaces: ['bayi'], built: ['bayi'] },
  { key: 'sale_prices', icon: 'accounting', permission: 'dealer_pricing.write', spaces: ['bayi'], built: ['bayi'] },
  { key: 'purchases', icon: 'truck', permission: 'purchases.write', spaces: ['bayi'], built: ['bayi'] },
  { key: 'orders', icon: 'order', permission: 'orders.read', spaces: ['bayi', 'depo', 'admin'], built: ['bayi', 'depo', 'admin'] },
  { key: 'catalog', icon: 'package', permission: 'catalog.read', spaces: ['bayi'], built: ['bayi'] },
  { key: 'transfers', icon: 'transfer', permission: 'transfers.request', spaces: ['bayi'], built: ['bayi'] },
  { key: 'warranty_claims', icon: 'warrantyClaim', permission: 'warranty_claims.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'leads', icon: 'lead', permission: 'leads.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'reports', icon: 'report', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'accounting', icon: 'accounting', permission: 'accounting.read', spaces: ['bayi', 'admin'] },
  { key: 'tasks', icon: 'task', permission: 'tasks.read', spaces: ['admin'], built: ['admin'] },
  { key: 'certificates', icon: 'shield', permission: 'certificates.read', spaces: ['admin'], built: ['admin'] },
  { key: 'pricing', icon: 'accounting', permission: ['pricing.recommended.read', 'pricing.discipline.read'], spaces: ['admin'], built: ['admin'] },
  { key: 'occupancy', icon: 'calendar', permission: 'appointments.read', spaces: ['admin'], built: ['admin'] },
  { key: 'performance', icon: 'chart', permission: 'performance.read', spaces: ['admin'], built: ['admin'] },
  { key: 'measurements', icon: 'measure', permission: 'measurements.read', spaces: ['admin'], built: ['admin'] },
  { key: 'announcements', icon: 'announcement', permission: 'announcements.read', spaces: ['bayi', 'depo', 'admin'], built: ['bayi', 'depo', 'admin'] },
];

export function MenuScreen({ space }: { space: SpaceKind }) {
  const { t } = useTranslation();
  const me = useSession((s) => s.me);
  const can = (p?: string | string[]) =>
    !p || !!me?.user.is_super_admin || [p].flat().some((k) => !!me?.permissions.includes(k));
  const items = ITEMS.filter((i) => i.spaces.includes(space) && can(i.permission));

  return (
    <>
      <Stack.Screen options={{ title: t('nav:menu') }} />
      <Screen>
        {items.length ? (
          <List>
            {items.map((i, idx) => (
              <ListRow
                key={i.key}
                title={t(`menu:${i.key}`)}
                icon={i.icon}
                onPress={() =>
                  i.built?.includes(space)
                    ? router.push(`/${space}/menu/${i.key}` as never)
                    : router.push({ pathname: '/soon/[key]', params: { key: i.key } })
                }
                last={idx === items.length - 1}
              />
            ))}
          </List>
        ) : null}
        <List>
          <ListRow icon="search" title={t('nav:search')} onPress={() => router.push('/search')} />
          <ListRow icon="spaces" title={t('spaces:title')} onPress={() => router.push('/spaces')} />
          <ListRow icon="settings" title={t('nav:settings')} onPress={() => router.push('/settings')} last />
        </List>
      </Screen>
    </>
  );
}
