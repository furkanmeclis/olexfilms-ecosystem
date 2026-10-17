import { Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { formatNumber, go } from '@/features/warehouse/format';
import { ListStatus } from '@/features/warehouse/parts';
import { unitStatusTone } from '@/features/warehouse/scan-result-card';

import { type StockUnitRow, useProductUnits } from './api';

/** Units of one product in the active organization, with status and location. */
export function StockProductScreen({ base }: { base: string }) {
  const { t } = useTranslation('warehouse');
  const { product, name } = useLocalSearchParams<{ product: string; name?: string }>();
  const org = useSession((s) => s.me?.active_organization_uuid ?? '');
  const query = useProductUnits(org, product);
  const items = query.data?.pages.flatMap((p) => p.items) ?? [];
  const total = query.data?.pages[0]?.total;

  return (
    <>
      <Stack.Screen options={{ title: name ?? t('fields.product'), headerLargeTitle: false }} />
      <FlatList
        data={items}
        keyExtractor={(i) => i.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10 pt-2"
        refreshControl={<RefreshControl refreshing={query.isRefetching && !query.isFetchingNextPage} onRefresh={() => void query.refetch()} />}
        onEndReachedThreshold={0.5}
        onEndReached={() => query.hasNextPage && !query.isFetchingNextPage && void query.fetchNextPage()}
        ListHeaderComponent={
          total !== undefined ? (
            <Text variant="footnote" tone="muted" className="pb-1">
              {t('m.units_count', { count: total })}
            </Text>
          ) : undefined
        }
        ListEmptyComponent={
          <ListStatus
            isLoading={query.isLoading}
            error={query.error}
            empty
            onRetry={() => void query.refetch()}
            emptyIcon="stock"
            emptyTitle={t('stock.list.empty_title')}
          />
        }
        renderItem={({ item }) => <UnitRow unit={item} onPress={() => go(`${base}/unit/${encodeURIComponent(item.barcode)}`)} />}
      />
    </>
  );
}

export function UnitRow({ unit, onPress }: { unit: StockUnitRow; onPress: () => void }) {
  const { t } = useTranslation('warehouse');
  const amount =
    unit.remaining_meters !== null
      ? `${formatNumber(unit.remaining_meters, 2)} m`
      : unit.unit_kind === 'fixed'
        ? `× ${formatNumber(unit.quantity)}`
        : null;
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="gap-1.5 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
      <View className="flex-row items-center gap-2">
        <Text variant="headline" className="flex-1 font-mono" numberOfLines={1}>
          {unit.barcode}
        </Text>
        {amount ? <Text variant="headline">{amount}</Text> : null}
      </View>
      <View className="flex-row items-center gap-2">
        <StatusPill tone={unitStatusTone(unit.status)} label={t(`stock.status.${unit.status}`)} />
        <Icon name="location" tone="muted-foreground" size={14} />
        <Text variant="footnote" tone="muted" numberOfLines={1} className="flex-1">
          {unit.location ? unit.location.code || unit.location.name : t('scan.unplaced')}
        </Text>
      </View>
    </Pressable>
  );
}
