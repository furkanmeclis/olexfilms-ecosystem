import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { activeMembership, useCan } from '@/core/spaces';
import { HomeHeaderLeft, HomeHeaderRight } from '@/features/shell/header-actions';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { KpiCard, KpiGrid } from '@/ui/kpi';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import { useWarehouseSummary } from './api';
import { useOrders, useWarehouseOrderCounts } from './orders-api';
import { useCurrentWarehouse } from './context';
import { go } from './format';
import { showsStockEntries } from './scope';
import { OrderCard } from './jobs-screen';
import { ErrorState, ListSkeleton } from './parts';
import { WarehousePickerSheet } from './warehouse-picker';
import type { IconName } from '@/ui/icons';
import { PullRefresh } from '@/ui/pull-refresh';

function greetingKey(hour: number) {
  if (hour < 12) return 'home:greeting_morning';
  if (hour < 18) return 'home:greeting_day';
  return 'home:greeting_evening';
}

/** Orders shown on the home before "see all". */
const HOME_ORDERS = 5;

/**
 * Warehouse home (TEC-1067, revize 7.2): where you work, the open and new
 * orders of that warehouse with their list, and the daily actions at the
 * bottom. Counts happen twice a year, so they are only a quick action.
 */
export function WarehouseHome() {
  const { t } = useTranslation('warehouse');
  const me = useSession((s) => s.me);
  const write = useCan('warehouse.write');
  const orders = useCan('orders.read');
  const read = useCan('warehouse.read');
  const stockRead = useCan('stock.read');
  const { warehouse, warehouses, isLoading, isError, refetch, select } = useCurrentWarehouse();
  // TEC-1049: the counters follow the picked warehouse (its uuid is in the query key).
  const scope = { uuid: warehouse?.uuid, ready: !isLoading };
  const summary = useWarehouseSummary(true, scope);
  // TEC-1067: the order counters and list are the picked warehouse's (seller_warehouse).
  const orderCounts = useWarehouseOrderCounts(orders, scope);
  const recent = useOrders('prepare', orders, scope);
  const recentOrders = (recent.data?.pages.flatMap((p) => p.items) ?? []).slice(0, HOME_ORDERS);
  const [picking, setPicking] = useState(false);
  // TEC-1087 (revize 7.4): stock entries are the center or distributor management's job; warehouse
  // staff see neither the "new entry" shortcut nor the open entry drafts.
  const entries =
    write &&
    showsStockEntries({
      permissions: me?.permissions ?? [],
      superAdmin: !!me?.user.is_super_admin,
      orgType: activeMembership(me)?.type,
    });

  // Without orders.read the home keeps the open job counters.
  const counters: { key: string; label: string; value: number | null | undefined; icon: IconName; tab: string; loading: boolean }[] =
    orders
      ? [
          { key: 'open', label: t('orders:warehouse_home.open'), value: orderCounts.data?.open, icon: 'package', tab: 'orders', loading: orderCounts.isLoading },
          { key: 'new', label: t('orders:warehouse_home.new'), value: orderCounts.data?.new, icon: 'cart', tab: 'orders', loading: orderCounts.isLoading },
        ]
      : [
          { key: 'transfers', label: t('m.open_transfers'), value: summary.data?.transfers, icon: 'transfer', tab: 'transfers', loading: summary.isLoading },
          ...(entries
            ? [{ key: 'entries', label: t('m.open_entries'), value: summary.data?.entries, icon: 'packageIn' as IconName, tab: 'entries', loading: summary.isLoading }]
            : []),
        ];

  // Daily work first; setup and counts last. Open jobs show their number on the tile.
  const actions: { key: string; label: string; icon: IconName; path: string; show: boolean; count?: number | null }[] = [
    { key: 'scan', label: t('nav_scan'), icon: 'scan', path: '/depo/scan', show: true },
    { key: 'orders', label: t('orders.nav'), icon: 'package', path: '/depo/jobs?tab=orders', show: orders },
    { key: 'transfer', label: t('transfers.new'), icon: 'transfer', path: '/depo/jobs/transfer/new', show: write, count: orders ? summary.data?.transfers : undefined },
    { key: 'entry', label: t('entries.new'), icon: 'packageIn', path: '/depo/jobs/entry/new', show: entries, count: orders ? summary.data?.entries : undefined },
    { key: 'move', label: t('move.title'), icon: 'location', path: '/depo/jobs/move', show: write },
    { key: 'stock', label: t('nav:stock'), icon: 'stock', path: '/depo/stock', show: true },
    { key: 'labels', label: t('labels.title'), icon: 'qr', path: '/depo/labels', show: read || stockRead },
    { key: 'reclass', label: t('reclass.title'), icon: 'barcode', path: '/depo/reclass', show: stockRead },
    { key: 'eod', label: t('eod.title'), icon: 'report', path: '/depo/eod', show: read },
    { key: 'count', label: t('counts.new'), icon: 'task', path: '/depo/jobs/count/new', show: write, count: summary.data?.counts },
    { key: 'setup', label: t('m.setup'), icon: 'settings', path: '/depo/setup', show: write },
  ];

  return (
    <>
      <Stack.Screen
        options={{
          title: t(greetingKey(new Date().getHours()), { name: me?.user.name ?? '' }),
          headerLeft: () => <HomeHeaderLeft />,
          headerRight: () => <HomeHeaderRight />,
        }}
      />
      <Screen
        refreshControl={
          <PullRefresh
            busy={summary.isRefetching}
            onRefresh={() => {
              refetch();
              void summary.refetch();
              if (orders) {
                void orderCounts.refetch();
                void recent.refetch();
              }
            }}
          />
        }>
        {isLoading ? (
          <Skeleton className="h-20 rounded-lg" />
        ) : isError && !warehouse ? (
          <ErrorState error={null} onRetry={refetch} />
        ) : !warehouse ? (
          <EmptyState
            icon="warehouse"
            title={t('locations.no_warehouses')}
            body={t('entries.no_active_warehouse')}
            action={write ? { title: t('m.setup'), onPress: () => go('/depo/setup') } : undefined}
          />
        ) : (
          <Pressable
            accessibilityRole="button"
            onPress={() => setPicking(true)}
            className="flex-row items-center gap-3 rounded-lg border border-border bg-card p-4 active:opacity-80">
            <View className="size-11 items-center justify-center rounded-md bg-secondary">
              <Icon name="warehouse" tone="primary" />
            </View>
            <View className="flex-1 gap-0.5">
              <Text variant="caption" tone="muted">
                {t('fields.warehouse')}
              </Text>
              <Text variant="headline" numberOfLines={1}>
                {warehouse.name}
              </Text>
              <Text variant="footnote" tone="muted" numberOfLines={1}>
                {[warehouse.code, warehouse.address].filter(Boolean).join(' · ')}
              </Text>
            </View>
            {warehouses.length > 1 ? <Icon name="chevronDown" tone="muted-foreground" size={18} /> : null}
          </Pressable>
        )}

        <KpiGrid>
          {counters.map((c) => (
            <KpiCard
              key={c.key}
              icon={c.icon}
              label={c.label}
              value={c.value == null ? '–' : String(c.value)}
              loading={c.loading}
              onPress={() => go(`/depo/jobs?tab=${c.tab}`)}
            />
          ))}
        </KpiGrid>
        {orders && orderCounts.error && !orderCounts.data ? (
          <ErrorState error={orderCounts.error} onRetry={() => void orderCounts.refetch()} />
        ) : null}
        {!orders && summary.error && !summary.data ? <ErrorState error={summary.error} onRetry={() => void summary.refetch()} /> : null}

        {orders ? (
          <>
            <View className="flex-row items-center justify-between">
              <SectionHeader title={t('orders:warehouse_home.list')} />
              <Pressable accessibilityRole="button" hitSlop={8} onPress={() => go('/depo/jobs?tab=orders')} className="pe-1 pt-4">
                <Text variant="footnote" tone="primary">
                  {t('orders:warehouse_home.see_all')}
                </Text>
              </Pressable>
            </View>
            {recent.isLoading ? (
              <ListSkeleton rows={3} />
            ) : recent.error && !recent.data ? (
              <ErrorState error={recent.error} onRetry={() => void recent.refetch()} />
            ) : recentOrders.length ? (
              <View className="gap-2">
                {recentOrders.map((o) => (
                  <OrderCard key={o.uuid} order={o} />
                ))}
              </View>
            ) : (
              <Text variant="callout" tone="muted" className="px-4">
                {t('orders:warehouse_home.empty')}
              </Text>
            )}
          </>
        ) : null}

        <SectionHeader title={t('home:quick_actions')} />
        <View className="flex-row flex-wrap gap-2.5">
          {actions
            .filter((a) => a.show)
            .map((a) => (
              <Pressable
                key={a.key}
                accessibilityRole="button"
                onPress={() => go(a.path)}
                className="min-h-24 grow basis-[46%] justify-between rounded-lg border border-border bg-card p-3.5 active:opacity-80">
                <View className="flex-row items-start justify-between">
                  <View className="size-9 items-center justify-center rounded-md bg-secondary">
                    <Icon name={a.icon} tone="primary" size={20} />
                  </View>
                  {a.count ? (
                    <Text variant="footnote" tone="muted" className="font-semibold tabular-nums">
                      {String(a.count)}
                    </Text>
                  ) : null}
                </View>
                <Text variant="headline">{a.label}</Text>
              </Pressable>
            ))}
        </View>
      </Screen>
      <WarehousePickerSheet withSetup visible={picking} onClose={() => setPicking(false)} value={warehouse?.uuid} onPick={(w) => select(w.uuid)} />
    </>
  );
}
