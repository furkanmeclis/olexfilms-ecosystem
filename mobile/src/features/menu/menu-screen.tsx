import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import type { SpaceKind } from '@/core/spaces';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';

type Item = { key: string; permission?: string; spaces: SpaceKind[]; built?: SpaceKind[] };

/**
 * Menu rows appear only with the permission (and so the module) behind them:
 * a closed module is not drawn at all. Rows open their screens as they are built.
 */
const ITEMS: Item[] = [
  { key: 'customers', permission: 'customers.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'warranties', permission: 'warranties.read', spaces: ['bayi'], built: ['bayi'] },
  { key: 'appointments', permission: 'appointments.read', spaces: ['bayi'] },
  { key: 'stock', permission: 'stock.read', spaces: ['bayi'] },
  { key: 'orders', permission: 'orders.read', spaces: ['bayi', 'depo', 'admin'], built: ['bayi', 'admin'] },
  { key: 'warranty_claims', permission: 'warranty_claims.read', spaces: ['bayi', 'admin'] },
  { key: 'leads', permission: 'leads.read', spaces: ['bayi', 'admin'] },
  { key: 'reports', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'accounting', permission: 'accounting.read', spaces: ['bayi', 'admin'] },
  { key: 'tasks', permission: 'tasks.read', spaces: ['admin'] },
  { key: 'announcements', permission: 'announcements.read', spaces: ['bayi', 'depo', 'admin'] },
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
          <ListRow title={t('nav:search')} onPress={() => router.push('/search')} />
          <ListRow title={t('spaces:title')} onPress={() => router.push('/spaces')} />
          <ListRow title={t('nav:settings')} onPress={() => router.push('/settings')} last />
        </List>
      </Screen>
    </>
  );
}
