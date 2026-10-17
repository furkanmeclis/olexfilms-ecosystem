import { Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { Boxes } from '@/ui/icons';
import { Text } from '@/ui/text';
import { go } from '@/features/warehouse/format';
import { ListStatus } from '@/features/warehouse/parts';

import { useLocationProducts } from './api';
import { stockAmount } from './stock-screen';

/** What is on one location (opened from a scanned location QR). */
export function LocationStockScreen({ base }: { base: string }) {
  const { t } = useTranslation('warehouse');
  const { location, name } = useLocalSearchParams<{ location: string; name?: string }>();
  const query = useLocationProducts(location);
  const items = query.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <>
      <Stack.Screen options={{ title: name ?? t('stock_panel.title'), headerLargeTitle: false }} />
      <FlatList
        data={items}
        keyExtractor={(i) => i.product.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10 pt-2"
        refreshControl={<RefreshControl refreshing={query.isRefetching && !query.isFetchingNextPage} onRefresh={() => void query.refetch()} />}
        onEndReachedThreshold={0.5}
        onEndReached={() => query.hasNextPage && !query.isFetchingNextPage && void query.fetchNextPage()}
        ListEmptyComponent={
          <ListStatus
            isLoading={query.isLoading}
            error={query.error}
            empty
            onRetry={() => void query.refetch()}
            emptyIcon={Boxes}
            emptyTitle={t('stock_panel.empty_title')}
            emptyBody={t('stock_panel.empty_description')}
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
              <Text variant="footnote" tone="muted" className="font-mono">
                {item.product.sku}
              </Text>
              {item.fixed_barcodes.length ? (
                <Text variant="footnote" tone="muted" numberOfLines={2}>
                  {item.fixed_barcodes.map((f) => `${f.barcode} × ${f.quantity}`).join(', ')}
                </Text>
              ) : null}
            </View>
            <Text variant="title">{stockAmount(item)}</Text>
          </Pressable>
        )}
      />
    </>
  );
}
