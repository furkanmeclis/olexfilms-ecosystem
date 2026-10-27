import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import { useModule, useModuleFilter } from '@/core/modules';
import { switchSpace } from '@/core/space-memory';
import { activeMembership, isCenterWorkplace, kindsOf, type SpaceKind } from '@/core/spaces';
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
  // TEC-1016: the center works in the dealer workplace; its admin tabs (approvals, network) become menu rows.
  { key: 'approvals', icon: 'warrantyClaim', spaces: [], built: [], orgTypes: ['center'] },
  { key: 'network', icon: 'network', permission: 'organizations.read', spaces: [], built: [], orgTypes: ['center'] },
  { key: 'customers', icon: 'customers', permission: 'customers.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  // TEC-866: no warranty list row for dealers (bayi); warranties open from the service detail.
  { key: 'appointments', icon: 'appointment', permission: 'appointments.read', spaces: ['bayi'], built: ['bayi'] },
  // TEC-958: the organization's own stock (the dealer's stock tab gives way to Measure).
  { key: 'stock', icon: 'stock', permission: 'stock.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
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
  // TEC-959: dealer fleet management, service subscriptions and staff cards (web: fleets, service-subscriptions, staff).
  { key: 'fleets', icon: 'truck', permission: 'fleets.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  {
    key: 'service_subscriptions',
    icon: 'subscription',
    permission: 'service_subscriptions.read',
    spaces: ['bayi', 'admin'],
    built: ['bayi', 'admin'],
  },
  { key: 'staff', icon: 'staff', permission: 'staff.manage', spaces: ['bayi'], built: ['bayi'] },
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
  // TEC-960: campaigns (approval queue inside), the panel AI assistant and the AI action approval queue.
  { key: 'campaigns', icon: 'campaign', permission: 'campaigns.read', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'assistant', icon: 'assistant', permission: 'ai.use', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
  { key: 'ai_actions', icon: 'check', permission: 'ai.actions.confirm', spaces: ['bayi', 'admin'], built: ['bayi', 'admin'] },
];

export function MenuScreen({ space }: { space: SpaceKind }) {
  const { t } = useTranslation();
  const me = useSession((s) => s.me);
  const can = (p?: string | string[]) =>
    !p || !!me?.user.is_super_admin || [p].flat().some((k) => !!me?.permissions.includes(k));
  const orgType = activeMembership(me)?.type;
  // The center's workplace also shows the admin rows (TEC-1016); their screens live under /bayi/menu.
  const workplace = space === 'bayi' && isCenterWorkplace(me);
  const shows = (i: Item) => i.spaces.includes(space) || (workplace && (i.spaces.includes('admin') || i.spaces.length === 0));
  const built = (i: Item) => !!i.built?.includes(space) || (workplace && (i.built?.includes('admin') || i.spaces.length === 0));
  // A switched-off module has no row (its key is also its route name).
  const moduleOn = useModuleFilter();
  const warehouseOn = useModule('warehouse');
  const items = ITEMS.filter(
    (i) =>
      shows(i) && can(i.permission) && moduleOn(i.key) && (!i.orgTypes || (!!orgType && i.orgTypes.includes(orgType))),
  );
  // Warehouse screens open over this space; swiping back returns here (no space switcher).
  const warehouse = space !== 'depo' && kindsOf(me).includes('depo') && warehouseOn;

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
                  built(i)
                    ? router.push(`/${space}/menu/${i.key}` as never)
                    : router.push({ pathname: '/soon/[key]', params: { key: i.key } })
                }
                last={idx === items.length - 1}
              />
            ))}
          </List>
        ) : null}
        {warehouse ? (
          <List>
            <ListRow icon="warehouse" title={t('menu:warehouse')} subtitle={t('menu:warehouse_hint')} onPress={() => switchSpace(me, 'depo')} last />
          </List>
        ) : null}
        <List>
          <ListRow icon="search" title={t('nav:search')} onPress={() => router.push('/search')} />
          {(me?.organizations?.length ?? 0) > 1 ? (
            <ListRow icon="spaces" title={t('spaces:title')} onPress={() => router.push('/spaces')} />
          ) : null}
          <ListRow icon="settings" title={t('nav:settings')} onPress={() => router.push('/settings')} last />
        </List>
      </Screen>
    </>
  );
}
