import { useQuery } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { useCan } from '@/core/spaces';
import { Text } from '@/ui/text';
import { useWarehouses } from '@/features/warehouse/api';
import { useCurrentWarehouse, useWarehouseChoice } from '@/features/warehouse/context';
import { formatNumber, go } from '@/features/warehouse/format';
import { Chips, ListStatus, Segmented } from '@/features/warehouse/parts';
import { unitStatusTone } from '@/features/warehouse/scan-result-card';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';

import { type StockProduct, type StockWarehouseUnit, useOrgProducts, useWarehouseUnits } from './api';
import {
  filterTotals,
  totalsTruncated,
  WAREHOUSE_TOTALS_MAX_PAGES,
  WAREHOUSE_TOTALS_PAGE,
  type WarehouseProductTotal,
  warehouseProductTotals,
} from './warehouse-totals';

type Filter = '' | 'in_stock' | 'out_of_stock';
/** A picked warehouse: its product totals, or (depo space) its units one by one. */
type View_ = 'units' | 'products';
/** Warehouse chip of every warehouse of the organization (TEC-1069). */
const ALL = 'all';

/** Quantity of a stock row in its unit: pieces, or meters for rolls. */
export function stockAmount(p: StockProduct) {
  if (p.product.unit_type === 'roll_meter') return `${formatNumber(p.meters, 2)} m`;
  return formatNumber(p.quantity);
}

type Row =
  | { kind: 'org'; p: StockProduct }
  | { kind: 'zero'; p: StockProduct }
  | { kind: 'wh'; w: WarehouseProductTotal; category?: string };

/**
 * Stock of the active organization by product, with search. Shared by the
 * warehouse, the dealer space and the admin menu (center stock, TEC-958);
 * `base` is the route prefix of the stack.
 *
 * TEC-1069: warehouse chips above the stock filter, "All warehouses" first
 * (the organization totals, the default) and one chip per active warehouse.
 * In the depo space the chip is also the device's warehouse choice
 * (TEC-1049), so the screen has a single warehouse selector.
 */
export function StockScreen({ base, space }: { base: string; space: 'depo' | 'bayi' | 'admin' }) {
  const { t } = useTranslation('warehouse');
  const org = useSession((s) => s.me?.active_organization_uuid ?? '');
  const [input, setInput] = useState('');
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState<Filter>('in_stock');
  const depo = space === 'depo';
  const [view, setView] = useState<View_>('products');
  const [picked, setPicked] = useState<string>(ALL);

  const canWarehouses = useCan('warehouse.read');
  const warehouses = useWarehouses(canWarehouses).data ?? [];
  const selectWarehouse = useWarehouseChoice((s) => s.select);
  const warehouse = picked === ALL ? undefined : warehouses.find((w) => w.uuid === picked);
  const pick = (uuid: string) => {
    setPicked(uuid);
    if (uuid === ALL) setView('products');
    // The depo space works in one warehouse: the chip moves the device's choice too.
    else if (depo) selectWarehouse(org, uuid);
  };

  useEffect(() => {
    const id = setTimeout(() => setQ(input.trim()), 300);
    return () => clearTimeout(id);
  }, [input]);

  // All warehouses: the organization totals with the API's stock filter.
  // One warehouse, "in stock": its totals summed from its units. One
  // warehouse, "out of stock" / "all": the product list with that
  // warehouse's amounts, so products missing there are listed too.
  const fromTotals = !!warehouse && filter === 'in_stock';
  const totals = useWarehouseTotals(warehouse?.uuid ?? '');
  const query = useOrgProducts(view === 'products' && !fromTotals ? org : '', q, warehouse ? '' : filter);
  const totalsData = totals.data;
  const pages = query.data?.pages;

  const rows = useMemo<Row[]>(() => {
    const orgItems = pages?.flatMap((p) => p.items) ?? [];
    if (!warehouse) return orgItems.map((p) => ({ kind: 'org', p }));
    if (filter === 'in_stock') return filterTotals(totalsData?.rows ?? [], q).map((w) => ({ kind: 'wh', w }));
    const byProduct = new Map((totalsData?.rows ?? []).map((r) => [r.product.uuid, r]));
    const all = orgItems.map<Row>((p) => {
      const w = byProduct.get(p.product.uuid);
      return w ? { kind: 'wh', w, category: p.product.category?.name } : { kind: 'zero', p };
    });
    return filter === 'out_of_stock' ? all.filter((r) => r.kind === 'zero') : all;
  }, [warehouse, filter, pages, totalsData, q]);

  const count = fromTotals ? rows.length : warehouse ? undefined : query.data?.pages[0]?.total;

  const header = (
    <View className="gap-2">
      {warehouses.length > 0 ? (
        <Chips
          value={warehouse ? warehouse.uuid : ALL}
          onChange={pick}
          options={[{ value: ALL, label: t('m.warehouse_all') }, ...warehouses.map((w) => ({ value: w.uuid, label: w.name }))]}
        />
      ) : null}
      {depo && warehouse ? <Segmented value={view} onChange={setView} options={viewOptions(t)} /> : null}
    </View>
  );

  return (
    <>
      <Stack.Screen
        options={{
          title: t('nav:stock'),
          headerSearchBarOptions: {
            placeholder: t('stock.filters.search_placeholder'),
            hideWhenScrolling: false,
            onChangeText: (e) => setInput(e.nativeEvent.text),
            onCancelButtonPress: () => setInput(''),
          },
        }}
      />
      {depo && warehouse && view === 'units' ? (
        <WarehouseUnitsList base={base} q={q} warehouseUuid={warehouse.uuid} header={header} />
      ) : (
        <FlatList
          data={rows}
          keyExtractor={(r) => (r.kind === 'wh' ? r.w.product.uuid : r.p.product.uuid)}
          contentInsetAdjustmentBehavior="automatic"
          keyboardDismissMode="on-drag"
          className="flex-1 bg-background"
          contentContainerClassName="gap-2 px-4 pb-10 pt-2"
          refreshControl={
            <RefreshControl
              refreshing={(query.isRefetching && !query.isFetchingNextPage) || totals.isRefetching}
              onRefresh={() => {
                if (!fromTotals) void query.refetch();
                if (warehouse) void totals.refetch();
              }}
            />
          }
          onEndReachedThreshold={0.5}
          onEndReached={() => !fromTotals && query.hasNextPage && !query.isFetchingNextPage && void query.fetchNextPage()}
          ListHeaderComponent={
            <View className="gap-2 pb-2">
              {header}
              <Chips
                value={filter}
                onChange={setFilter}
                options={[
                  { value: 'in_stock', label: t('stock.products.status.in_stock') },
                  { value: 'out_of_stock', label: t('stock.products.status.out_of_stock') },
                  { value: '', label: t('m.all') },
                ]}
              />
              <Text variant="footnote" tone="muted">
                {[warehouse?.name ?? t('m.all_warehouses'), count !== undefined ? t('m.products_count', { count }) : null]
                  .filter(Boolean)
                  .join(' · ')}
              </Text>
              {warehouse && totalsData?.truncated ? (
                <Text variant="footnote" tone="danger">
                  {t('m.warehouse_totals_partial', { count: WAREHOUSE_TOTALS_PAGE * WAREHOUSE_TOTALS_MAX_PAGES })}
                </Text>
              ) : null}
            </View>
          }
          ListEmptyComponent={
            <ListStatus
              isLoading={fromTotals ? totals.isLoading : query.isLoading || (!!warehouse && totals.isLoading)}
              error={fromTotals ? totals.error : (query.error ?? totals.error)}
              empty
              onRetry={() => void (fromTotals ? totals.refetch() : query.refetch())}
              emptyIcon="stock"
              emptyTitle={q ? t('stock.list.empty_filtered') : t('stock.products.empty_title')}
              emptyBody={q ? undefined : t('stock.products.empty_description')}
            />
          }
          renderItem={({ item }) =>
            item.kind === 'wh' ? (
              <ProductRow
                name={item.w.product.name}
                sku={item.w.product.sku}
                category={item.category}
                amount={item.w.meters !== null ? `${formatNumber(item.w.meters, 2)} m` : formatNumber(item.w.units)}
                sub={item.w.meters !== null ? t('m.rolls', { count: item.w.units }) : undefined}
                muted={false}
                onPress={() => go(`${base}/${item.w.product.uuid}?name=${encodeURIComponent(item.w.product.name)}`)}
              />
            ) : (
              <ProductRow
                name={item.p.product.name}
                sku={item.p.product.sku}
                category={item.p.product.category?.name}
                amount={item.kind === 'zero' ? formatNumber(0) : stockAmount(item.p)}
                sub={
                  item.kind === 'org' && item.p.product.unit_type === 'roll_meter'
                    ? t('m.rolls', { count: item.p.quantity })
                    : undefined
                }
                muted={item.kind === 'zero' || item.p.quantity <= 0}
                onPress={() => go(`${base}/${item.p.product.uuid}?name=${encodeURIComponent(item.p.product.name)}`)}
              />
            )
          }
        />
      )}
    </>
  );
}

function ProductRow(p: {
  name: string;
  sku: string;
  category?: string;
  amount: string;
  sub?: string;
  muted: boolean;
  onPress: () => void;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      onPress={p.onPress}
      className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
      <View className="flex-1 gap-0.5">
        <Text variant="headline" numberOfLines={2}>
          {p.name}
        </Text>
        <Text variant="footnote" tone="muted" numberOfLines={1}>
          <Text variant="footnote" tone="muted" className="font-mono">
            {p.sku}
          </Text>
          {p.category ? ` · ${p.category}` : ''}
        </Text>
      </View>
      <View className="items-end">
        <Text variant="title" className={p.muted ? 'text-muted-foreground' : ''}>
          {p.amount}
        </Text>
        {p.sub ? (
          <Text variant="footnote" tone="muted">
            {p.sub}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
}

/**
 * Product totals of one warehouse, summed from its units (TEC-1069: the API
 * has no per-warehouse product list). Reads at most
 * WAREHOUSE_TOTALS_MAX_PAGES pages of 100 units; `truncated` says when the
 * warehouse holds more than that.
 */
function useWarehouseTotals(warehouse: string) {
  return useQuery({
    queryKey: ['stock', 'warehouse-totals', warehouse],
    enabled: !!warehouse,
    queryFn: async () => {
      const units: StockWarehouseUnit[] = [];
      let total = 0;
      for (let page = 0; page < WAREHOUSE_TOTALS_MAX_PAGES; page++) {
        const r = await unwrap(
          api.GET('/v1/stock/warehouses/{uuid}/units', {
            params: {
              path: { uuid: warehouse },
              query: { state: 'in', sort: 'product', limit: WAREHOUSE_TOTALS_PAGE, offset: page * WAREHOUSE_TOTALS_PAGE },
            },
          }),
        );
        units.push(...r.items);
        total = r.total;
        if (units.length >= r.total || r.items.length === 0) break;
      }
      return { rows: warehouseProductTotals(units), truncated: totalsTruncated(units.length, total) };
    },
  });
}

function viewOptions(t: (k: string) => string): { value: View_; label: string }[] {
  return [
    { value: 'products', label: t('m.product_totals') },
    { value: 'units', label: t('m.this_warehouse_units') },
  ];
}

/** Units in the picked warehouse (TEC-1049): reloads when the warehouse changes. */
function WarehouseUnitsList({
  base,
  q,
  warehouseUuid,
  header,
}: {
  base: string;
  q: string;
  warehouseUuid: string;
  header: React.ReactElement;
}) {
  const { t } = useTranslation('warehouse');
  const { warehouses, isLoading } = useCurrentWarehouse();
  const warehouse = warehouses.find((w) => w.uuid === warehouseUuid);
  const query = useWarehouseUnits(warehouseUuid, q);
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
