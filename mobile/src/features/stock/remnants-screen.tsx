import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { FlatList } from 'react-native';

import { useSession } from '@/core/auth/session';
import { PullRefresh } from '@/ui/pull-refresh';
import { Text } from '@/ui/text';
import { go } from '@/features/warehouse/format';
import { ListStatus } from '@/features/warehouse/parts';

import { useRemnantUnits } from './api';
import { UnitRow } from './stock-product-screen';

/**
 * TEC-1243 (K7, K14): the remnants on hand: rolls left under the
 * product's minimum cut. They are not offered to orders; the warehouse
 * writes them off with a stock adjustment document.
 */
export function RemnantsScreen({ base }: { base: string }) {
  const { t } = useTranslation('warehouse');
  const org = useSession((s) => s.me?.active_organization_uuid ?? '');
  const query = useRemnantUnits(org);
  const items = query.data?.pages.flatMap((p) => p.items) ?? [];
  const total = query.data?.pages[0]?.total;
  return (
    <>
      <Stack.Screen options={{ title: t('m.remnants_title'), headerLargeTitle: false }} />
      <FlatList
        data={items}
        keyExtractor={(i) => i.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10 pt-2"
        refreshControl={<PullRefresh busy={query.isRefetching && !query.isFetchingNextPage} onRefresh={() => void query.refetch()} />}
        onEndReachedThreshold={0.5}
        onEndReached={() => query.hasNextPage && !query.isFetchingNextPage && void query.fetchNextPage()}
        ListHeaderComponent={
          <Text variant="footnote" tone="muted" className="pb-1">
            {total !== undefined ? `${t('m.units_count', { count: total })} · ` : ''}
            {t('m.remnants_hint')}
          </Text>
        }
        ListEmptyComponent={
          <ListStatus
            isLoading={query.isLoading}
            error={query.error}
            empty
            onRetry={() => void query.refetch()}
            emptyIcon="stock"
            emptyTitle={t('m.remnants_empty')}
          />
        }
        renderItem={({ item }) => (
          <UnitRow unit={item} showProduct onPress={() => go(`${base}/unit/${encodeURIComponent(item.barcode)}`)} />
        )}
      />
    </>
  );
}
