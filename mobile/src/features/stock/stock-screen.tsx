import { Stack } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { Boxes } from '@/ui/icons';
import { Text } from '@/ui/text';
import { formatNumber, go } from '@/features/warehouse/format';
import { Chips, ListStatus } from '@/features/warehouse/parts';

import { type StockProduct, useOrgProducts } from './api';

type Filter = '' | 'in_stock' | 'out_of_stock';

/** Quantity of a stock row in its unit: pieces, or meters for rolls. */
export function stockAmount(p: StockProduct) {
  if (p.product.unit_type === 'roll_meter') return `${formatNumber(p.meters, 2)} m`;
  return formatNumber(p.quantity);
}

/**
 * Stock of the active organization by product, with search. Shared by the
 * warehouse and the dealer space (`base` is the tab's route prefix).
 */
export function StockScreen({ base, space }: { base: string; space: 'depo' | 'bayi' }) {
  const { t } = useTranslation('warehouse');
  const org = useSession((s) => s.me?.active_organization_uuid ?? '');
  const [input, setInput] = useState('');
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState<Filter>('in_stock');

  useEffect(() => {
    const id = setTimeout(() => setQ(input.trim()), 300);
    return () => clearTimeout(id);
  }, [input]);

  const query = useOrgProducts(org, q, filter);
  const items = query.data?.pages.flatMap((p) => p.items) ?? [];
  const total = query.data?.pages[0]?.total;

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
                {/* The stock API has no warehouse filter: say so instead of showing the warehouse chip. */}
                {space === 'depo' ? `${t('m.all_warehouses')} · ` : ''}
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
            emptyIcon={Boxes}
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
    </>
  );
}
