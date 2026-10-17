import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { useCan } from '@/core/spaces';
import type { LucideIcon } from '@/ui/icons';
import { ArrowLeftRight, Boxes, ChevronDown, ClipboardList, MapPin, Package, PackagePlus, ScanLine, Settings, Warehouse as WarehouseIcon } from '@/ui/icons';
import { HomeHeaderLeft, SearchButton } from '@/features/shell/header-actions';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import { useWarehouseSummary } from './api';
import { useCurrentWarehouse } from './context';
import { go } from './format';
import { ErrorState } from './parts';
import { WarehousePickerSheet } from './warehouse-picker';

function greetingKey(hour: number) {
  if (hour < 12) return 'home:greeting_morning';
  if (hour < 18) return 'home:greeting_day';
  return 'home:greeting_evening';
}

/** Warehouse home: where you work, what is open, and the daily actions. */
export function WarehouseHome() {
  const { t } = useTranslation('warehouse');
  const me = useSession((s) => s.me);
  const write = useCan('warehouse.write');
  const orders = useCan('orders.read');
  const { warehouse, warehouses, isLoading, isError, refetch, select } = useCurrentWarehouse();
  const summary = useWarehouseSummary(true);
  const [picking, setPicking] = useState(false);

  const counters: { key: string; label: string; value: number | null | undefined; icon: LucideIcon; tab: string }[] = [
    { key: 'transfers', label: t('m.open_transfers'), value: summary.data?.transfers, icon: ArrowLeftRight, tab: 'transfers' },
    { key: 'counts', label: t('m.open_counts'), value: summary.data?.counts, icon: ClipboardList, tab: 'counts' },
    { key: 'entries', label: t('m.open_entries'), value: summary.data?.entries, icon: PackagePlus, tab: 'entries' },
  ];

  const actions: { key: string; label: string; icon: LucideIcon; path: string; show: boolean }[] = [
    { key: 'scan', label: t('nav_scan'), icon: ScanLine, path: '/depo/scan', show: true },
    { key: 'entry', label: t('entries.new'), icon: PackagePlus, path: '/depo/jobs/entry/new', show: write },
    { key: 'transfer', label: t('transfers.new'), icon: ArrowLeftRight, path: '/depo/jobs/transfer/new', show: write },
    { key: 'count', label: t('counts.new'), icon: ClipboardList, path: '/depo/jobs/count/new', show: write },
    { key: 'move', label: t('move.title'), icon: MapPin, path: '/depo/jobs/move', show: write },
    { key: 'orders', label: t('orders.nav'), icon: Package, path: '/depo/jobs?tab=orders', show: orders },
    { key: 'setup', label: t('m.setup'), icon: Settings, path: '/depo/setup', show: write },
    { key: 'stock', label: t('nav:stock'), icon: Boxes, path: '/depo/stock', show: true },
  ];

  return (
    <>
      <Stack.Screen
        options={{
          title: t(greetingKey(new Date().getHours()), { name: me?.user.name ?? '' }),
          headerLeft: () => <HomeHeaderLeft />,
          headerRight: () => <SearchButton />,
        }}
      />
      <Screen
        refreshControl={
          <RefreshControl
            refreshing={summary.isRefetching}
            onRefresh={() => {
              refetch();
              void summary.refetch();
            }}
          />
        }>
        {isLoading ? (
          <Skeleton className="h-20 rounded-lg" />
        ) : isError && !warehouse ? (
          <ErrorState error={null} onRetry={refetch} />
        ) : !warehouse ? (
          <EmptyState
            icon={WarehouseIcon}
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
              <Icon as={WarehouseIcon} tone="primary" />
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
            {warehouses.length > 1 ? <Icon as={ChevronDown} tone="muted-foreground" size={18} /> : null}
          </Pressable>
        )}

        <View className="flex-row gap-2.5">
          {counters.map((c) => (
            <Pressable
              key={c.key}
              accessibilityRole="button"
              onPress={() => go(`/depo/jobs?tab=${c.tab}`)}
              className="flex-1 gap-1 rounded-lg border border-border bg-card p-3 active:opacity-80">
              <Icon as={c.icon} tone="primary" size={18} />
              {summary.isLoading ? (
                <Skeleton className="h-7 w-10" />
              ) : (
                <Text variant="title" className="tabular-nums">
                  {c.value ?? '–'}
                </Text>
              )}
              <Text variant="footnote" tone="muted" numberOfLines={2}>
                {c.label}
              </Text>
            </Pressable>
          ))}
        </View>
        {summary.error && !summary.data ? <ErrorState error={summary.error} onRetry={() => void summary.refetch()} /> : null}

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
                <View className="size-9 items-center justify-center rounded-md bg-secondary">
                  <Icon as={a.icon} tone="primary" size={20} />
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
