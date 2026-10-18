import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import type { SpaceKind } from '@/core/spaces';
import type { IconName } from '@/ui/icons';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';

type Item = { key: string; icon: IconName; permission?: string; spaces: SpaceKind[]; built?: SpaceKind[] };

/**
 * Menu rows appear only with the permission (and so the module) behind them:
 * a closed module is not drawn at all. Rows open their screens as they are built.
 */
const ITEMS: Item[] = [
  { key: 'customers', icon: 'customers', permission: 'customers.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'warranties', icon: 'warranty', permission: 'warranties.read', spaces: ['bayi'], built: ['bayi'] },
  { key: 'appointments', icon: 'appointment', permission: 'appointments.read', spaces: ['bayi'], built: ['bayi'] },
  { key: 'stock', icon: 'stock', permission: 'stock.read', spaces: ['bayi'] },
  { key: 'orders', icon: 'order', permission: 'orders.read', spaces: ['bayi', 'depo', 'admin'], built: ['bayi', 'admin'] },
  { key: 'warranty_claims', icon: 'warrantyClaim', permission: 'warranty_claims.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'leads', icon: 'lead', permission: 'leads.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'reports', icon: 'report', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'accounting', icon: 'accounting', permission: 'accounting.read', spaces: ['bayi', 'admin'] },
  { key: 'tasks', icon: 'task', permission: 'tasks.read', spaces: ['admin'], built: ['admin'] },
  { key: 'announcements', icon: 'announcement', permission: 'announcements.read', spaces: ['bayi', 'depo', 'admin'], built: ['bayi', 'depo', 'admin'] },
];

export function MenuScreen({ space }: { space: SpaceKind }) {
  const { t } = useTranslation();
  const me = useSession((s) => s.me);
  const can = (p?: string) => !p || !!me?.user.is_super_admin || !!me?.permissions.includes(p);
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
