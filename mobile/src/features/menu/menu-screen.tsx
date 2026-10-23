import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import { activeMembership, type SpaceKind } from '@/core/spaces';
import type { IconName } from '@/ui/icons';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';

type OrgType = 'center' | 'distributor' | 'dealer';
type Item = {
  key: string;
  icon: IconName;
  permission?: string | string[];
  spaces: SpaceKind[];
  built?: SpaceKind[];
  /** Organization types the backend serves the screen to (e.g. e-invoices: the brand center only). */
  orgTypes?: OrgType[];
};

/**
 * Menu rows appear only with the permission (and so the module) behind them:
 * a closed module is not drawn at all. Rows open their screens as they are built.
 */
const ITEMS: Item[] = [
  { key: 'customers', icon: 'customers', permission: 'customers.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  // TEC-866: no warranty list row for dealers (bayi); warranties open from the service detail.
  { key: 'appointments', icon: 'appointment', permission: 'appointments.read', spaces: ['bayi'], built: ['bayi'] },
  { key: 'stock', icon: 'stock', permission: 'stock.read', spaces: ['bayi'] },
  { key: 'orders', icon: 'order', permission: 'orders.read', spaces: ['bayi', 'depo', 'admin'], built: ['bayi', 'depo', 'admin'] },
  { key: 'catalog', icon: 'package', permission: 'catalog.read', spaces: ['bayi'], built: ['bayi'] },
  { key: 'transfers', icon: 'transfer', permission: 'transfers.request', spaces: ['bayi'], built: ['bayi'] },
  { key: 'warranty_claims', icon: 'warrantyClaim', permission: 'warranty_claims.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'leads', icon: 'lead', permission: 'leads.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'reports', icon: 'report', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  // TEC-957: cari accounts and disputes (both sides read with accounting.read); e-invoices in the center.
  { key: 'accounting', icon: 'accounting', permission: 'accounting.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'disputes', icon: 'alert', permission: 'accounting.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'einvoices', icon: 'report', permission: 'einvoice.read', spaces: ['admin'], built: ['admin'], orgTypes: ['center'] },
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
  const orgType = activeMembership(me)?.type;
  const items = ITEMS.filter(
    (i) => i.spaces.includes(space) && can(i.permission) && (!i.orgTypes || (!!orgType && i.orgTypes.includes(orgType))),
  );

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
