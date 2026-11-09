import { Stack } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { Text } from '@/ui/text';
import { useCurrentWarehouse } from '@/features/warehouse/context';
import { formatNumber, go } from '@/features/warehouse/format';
import { Chips, ListStatus, Segmented } from '@/features/warehouse/parts';
import { unitStatusTone } from '@/features/warehouse/scan-result-card';
import { WarehouseChip } from '@/features/warehouse/warehouse-picker';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';

import { type StockProduct, type StockWarehouseUnit, useOrgProducts, useWarehouseUnits } from './api';

type Filter = '' | 'in_stock' | 'out_of_stock';
/** Depo space: the units in the picked warehouse, or the organization's product totals. */
type View_ = 'units' | 'products';

/** Quantity of a stock row in its unit: pieces, or meters for rolls. */
export function stockAmount(p: StockProduct) {
  if (p.product.unit_type === 'roll_meter') return `${formatNumber(p.meters, 2)} m`;
  return formatNumber(p.quantity);
}

/**
 * Stock of the active organization by product, with search. Shared by the
 * warehouse, the dealer space and the admin menu (center stock, TEC-958);
 * `base` is the route prefix of the stack.
 */
export function StockScreen({ base, space }: { base: string; space: 'depo' | 'bayi' | 'admin' }) {
  const { t } = useTranslation('warehouse');
  const org = useSession((s) => s.me?.active_organization_uuid ?? '');
  const [input, setInput] = useState('');
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState<Filter>('in_stock');
  // TEC-1049: the depo space opens on the picked warehouse's units; the
  // product totals of the organization (all warehouses) stay one tap away.
  const depo = space === 'depo';
  const [view, setView] = useState<View_>(depo ? 'units' : 'products');

  useEffect(() => {
    const id = setTimeout(() => setQ(input.trim()), 300);
    return () => clearTimeout(id);
  }, [input]);

  const query = useOrgProducts(view === 'products' ? org : '', q, filter);
  const items = query.data?.pages.flatMap((p) => p.items) ?? [];
  const total = query.data?.pages[0]?.total;

  return (
    <>
      <Stack.Screen
        options={{
          title: t('nav:stock'),
          headerRight: depo ? () => <WarehouseChip /> : undefined,
          headerSearchBarOptions: {
            placeholder: t('stock.filters.search_placeholder'),
            hideWhenScrolling: false,
            onChangeText: (e) => setInput(e.nativeEvent.text),
            onCancelButtonPress: () => setInput(''),
          },
        }}
      />
      {depo && view === 'units' ? (
        <WarehouseUnitsList base={base} q={q} header={<Segmented value={view} onChange={setView} options={viewOptions(t)} />} />
      ) : (
        <FlatList
          data={items}
          keyExtractor={(i) => i.product.uuid}
          contentInsetAdjustmentBehavior="automatic"
          keyboardDismissMode="on-drag"
          className="flex-1 bg-background"
          contentContainerClassName="gap-2 px-4 pb-10 pt-2"
          refreshControl={<RefreshControl refreshing={query.isRefetching && !query.isFetchingNextPage} onRefresh={() => void query.refetch()} />}
          onEndReachedThreshold={0.5}
          onEndReached={() => query.hasNextPage && !query.isFetchingNextPage && void query.fetchNextPage()}
          ListHeaderComponent={
            <View className="gap-2 pb-2">
              {depo ? <Segmented value={view} onChange={setView} options={viewOptions(t)} /> : null}
              <Chips
                value={filter}
                onChange={setFilter}
                options={[
                  { value: 'in_stock', label: t('stock.products.status.in_stock') },
                  { value: 'out_of_stock', label: t('stock.products.status.out_of_stock') },
                  { value: '', label: t('m.all') },
                ]}
              />
              {total !== undefined ? (
                <Text variant="footnote" tone="muted">
                  {/* The product totals have no warehouse filter: say so. */}
                  {depo ? `${t('m.all_warehouses')} · ` : ''}
                  {t('m.products_count', { count: total })}
                </Text>
              ) : null}
            </View>
          }
          ListEmptyComponent={
            <ListStatus
              isLoading={query.isLoading}
              error={query.error}
              empty
              onRetry={() => void query.refetch()}
              emptyIcon="stock"
              emptyTitle={q ? t('stock.list.empty_filtered') : t('stock.products.empty_title')}
              emptyBody={q ? undefined : t('stock.products.empty_description')}
            />
          }
          renderItem={({ item }) => (
            <Pressable
              accessibilityRole="button"
              onPress={() => go(`${base}/${item.product.uuid}?name=${encodeURIComponent(item.product.name)}`)}
              className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
              <View className="flex-1 gap-0.5">
                <Text variant="headline" numberOfLines={2}>
                  {item.product.name}
                </Text>
                <Text variant="footnote" tone="muted" numberOfLines={1}>
                  <Text variant="footnote" tone="muted" className="font-mono">
                    {item.product.sku}
                  </Text>
                  {item.product.category?.name ? ` · ${item.product.category.name}` : ''}
                </Text>
              </View>
              <View className="items-end">
                <Text variant="title" className={item.quantity > 0 ? '' : 'text-muted-foreground'}>
                  {stockAmount(item)}
                </Text>
                {item.product.unit_type === 'roll_meter' ? (
                  <Text variant="footnote" tone="muted">
                    {t('m.rolls', { count: item.quantity })}
                  </Text>
                ) : null}
              </View>
            </Pressable>
          )}
        />
      )}
    </>
  );
}

function viewOptions(t: (k: string) => string): { value: View_; label: string }[] {
  return [
    { value: 'units', label: t('m.this_warehouse_units') },
    { value: 'products', label: t('m.product_totals') },
  ];
}

/** Units in the picked warehouse (TEC-1049): reloads when the warehouse changes. */
function WarehouseUnitsList({ base, q, header }: { base: string; q: string; header: React.ReactElement }) {
  const { t } = useTranslation('warehouse');
  const { warehouse, isLoading } = useCurrentWarehouse();
  const query = useWarehouseUnits(warehouse?.uuid ?? '', q);
  const items = query.data?.pages.flatMap((p) => p.items) ?? [];
  const total = query.data?.pages[0]?.total;
  return (
    <FlatList
      data={items}
      keyExtractor={(i) => i.uuid}
      contentInsetAdjustmentBehavior="automatic"
      keyboardDismissMode="on-drag"
      className="flex-1 bg-background"
      contentContainerClassName="gap-2 px-4 pb-10 pt-2"
      refreshControl={<RefreshControl refreshing={query.isRefetching && !query.isFetchingNextPage} onRefresh={() => void query.refetch()} />}
      onEndReachedThreshold={0.5}
      onEndReached={() => query.hasNextPage && !query.isFetchingNextPage && void query.fetchNextPage()}
      ListHeaderComponent={
        <View className="gap-2 pb-2">
          {header}
          {warehouse && total !== undefined ? (
            <Text variant="footnote" tone="muted">
              {`${warehouse.name} · ${t('m.units_count', { count: total })}`}
            </Text>
          ) : null}
        </View>
      }
      ListEmptyComponent={
        <ListStatus
          isLoading={isLoading || query.isLoading}
          error={query.error}
          empty
          onRetry={() => void query.refetch()}
          emptyIcon="stock"
          emptyTitle={!warehouse ? t('locations.no_warehouses') : q ? t('stock.list.empty_filtered') : t('stock.list.empty_title')}
        />
      }
      renderItem={({ item }) => (
        <WarehouseUnitRow unit={item} onPress={() => go(`${base}/unit/${encodeURIComponent(item.barcode)}`)} />
      )}
    />
  );
}

function WarehouseUnitRow({ unit, onPress }: { unit: StockWarehouseUnit; onPress: () => void }) {
  const { t } = useTranslation('warehouse');
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="gap-1.5 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
      <View className="flex-row items-center gap-2">
        <Text variant="headline" className="flex-1" numberOfLines={1}>
          {unit.product.name}
        </Text>
        {unit.remaining_meters !== null ? <Text variant="headline">{`${formatNumber(unit.remaining_meters, 2)} m`}</Text> : null}
      </View>
      <Text variant="footnote" tone="muted" className="font-mono" numberOfLines={1}>
        {unit.barcode}
      </Text>
      <View className="flex-row items-center gap-2">
        <StatusPill tone={unitStatusTone(unit.status)} label={t(`stock.status.${unit.status}`)} />
        <Icon name="location" tone="muted-foreground" size={14} />
        <Text variant="footnote" tone="muted" numberOfLines={1} className="flex-1">
          {unit.location_code ?? t('scan.unplaced')}
        </Text>
      </View>
    </Pressable>
  );
}
