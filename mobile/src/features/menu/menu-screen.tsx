import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { useModule, useModuleFilter } from '@/core/modules';
import { switchSpace } from '@/core/space-memory';
import { activeMembership, isCenterWorkplace, kindsOf, type SpaceKind } from '@/core/spaces';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { itemBuilt, menuGroups, type MenuRow, type MenuViewer } from './groups';

export function MenuScreen({ space }: { space: SpaceKind }) {
  const { t } = useTranslation();
  const me = useSession((s) => s.me);
  const moduleOn = useModuleFilter();
  const warehouseOn = useModule('warehouse');
  const viewer: MenuViewer = {
    space,
    // The center's workplace also shows the admin rows (TEC-1016); their screens live under /bayi/menu.
    workplace: space === 'bayi' && isCenterWorkplace(me),
    orgType: activeMembership(me)?.type,
    isSuperAdmin: !!me?.user.is_super_admin,
    permissions: me?.permissions ?? [],
    // A switched-off module has no row (its key is also its route name).
    moduleOn,
    // Warehouse screens open over this space; swiping back returns here (no space switcher).
    warehouse: space !== 'depo' && kindsOf(me).includes('depo') && warehouseOn,
    multiOrg: (me?.organizations?.length ?? 0) > 1,
  };
  const groups = menuGroups(viewer);

  const open = (row: MenuRow) => {
    if (row.kind === 'item') {
      const i = row.item;
      if (itemBuilt(i, viewer)) router.push(`/${space}/menu/${i.key}` as never);
      else router.push({ pathname: '/soon/[key]', params: { key: i.key } });
      return;
    }
    if (row.row.key === 'warehouse') switchSpace(me, 'depo');
    else if (row.row.key === 'search') router.push('/search');
    else if (row.row.key === 'spaces') router.push('/spaces');
    else router.push('/settings');
  };

  const title = (row: MenuRow) => {
    if (row.kind === 'item') return t(`menu:${row.item.key}`);
    if (row.row.key === 'warehouse') return t('menu:warehouse');
    if (row.row.key === 'search') return t('nav:search');
    if (row.row.key === 'spaces') return t('spaces:title');
    return t('nav:settings');
  };

  return (
    <>
      <Stack.Screen options={{ title: t('nav:menu') }} />
      <Screen className="gap-5">
        {groups.map((g) => (
          <View key={g.key} className="gap-2">
            {g.titled ? (
              // Not the caption variant: its letter spacing breaks joined Arabic script.
              <Text variant="footnote" tone="muted" accessibilityRole="header" numberOfLines={1} className="px-1 font-semibold">
                {t(`menu:group.${g.key}`)}
              </Text>
            ) : null}
            <List>
              {g.rows.map((row, idx) => (
                <ListRow
                  key={row.kind === 'item' ? row.item.key : `account.${row.row.key}`}
                  title={title(row)}
                  subtitle={row.kind === 'account' && row.row.key === 'warehouse' ? t('menu:warehouse_hint') : undefined}
                  icon={row.kind === 'item' ? row.item.icon : row.row.icon}
                  onPress={() => open(row)}
                  last={idx === g.rows.length - 1}
                />
              ))}
            </List>
          </View>
        ))}
      </Screen>
    </>
  );
}
