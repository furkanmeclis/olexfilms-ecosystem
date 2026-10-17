import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import {
  formatDate,
  PAGE_SIZE,
  useDebounced,
  useLinks,
  usePagedList,
  type Warranty,
  WARRANTY_STATUSES,
  type WarrantyStatus,
  warrantyTone,
} from '@/features/services/lib';
import { Chips, ListBody, ListFooter, SearchField } from '@/features/services/ui';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

type Filter = 'all' | WarrantyStatus;

function daysLeft(end: string) {
  return Math.ceil((new Date(end).getTime() - Date.now()) / 86_400_000);
}

function WarrantyRow({ w, onPress }: { w: Warranty; onPress: () => void }) {
  const { t, i18n } = useTranslation('dealer');
  const left = w.status === 'active' ? daysLeft(w.end_at) : null;
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="headline" numberOfLines={1} className="flex-1">
          {w.vehicle.plate || w.public_code}
        </Text>
        <StatusPill label={t(`warranty_status.${w.status}`)} tone={warrantyTone(w.status)} />
      </View>
      <Text variant="callout" numberOfLines={1}>
        {w.product.name}
      </Text>
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {[
          w.public_code,
          `${formatDate(w.start_at, i18n.language)} – ${formatDate(w.end_at, i18n.language)}`,
          left !== null && left >= 0 ? t('days_left', { count: left }) : null,
        ]
          .filter(Boolean)
          .join(' · ')}
      </Text>
    </Pressable>
  );
}

/** Warranties of the scope: active ones first by the soonest end (API default). */
export function WarrantiesListScreen() {
  const { t } = useTranslation('dealer');
  const links = useLinks();
  const [q, setQ] = useState('');
  const [filter, setFilter] = useState<Filter>('all');
  const query = useDebounced(q.trim());

  const list = usePagedList(['warranties', 'list', { q: query, filter }], (offset) =>
    unwrap(
      api.GET('/v1/warranties', {
        params: {
          query: { q: query || undefined, status: filter === 'all' ? undefined : filter, limit: PAGE_SIZE, offset },
        },
      }),
    ),
  );

  const chips = [
    { value: 'all' as Filter, label: t('filter_all') },
    ...WARRANTY_STATUSES.map((s) => ({ value: s as Filter, label: t(`warranty_status.${s}`) })),
  ];

  return (
    <>
      <Stack.Screen options={{ title: t('warranties') }} />
      <FlatList
        data={list.items}
        keyExtractor={(w) => w.uuid}
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10"
        ListHeaderComponent={
          <View className="gap-3 pb-1 pt-2">
            <SearchField value={q} onChange={setQ} placeholder={t('warranty_search')} />
            <Chips options={chips} value={filter} onChange={setFilter} />
            {list.total !== undefined && list.items.length > 0 ? (
              <Text variant="footnote" tone="muted">
                {t('count_results', { count: list.total })}
              </Text>
            ) : null}
          </View>
        }
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{
              icon: 'warranty',
              title: query || filter !== 'all' ? t('warranties_empty_filtered') : t('warranties_empty'),
              body: query || filter !== 'all' ? undefined : t('warranties_empty_body'),
            }}
          />
        }
        renderItem={({ item }) => <WarrantyRow w={item} onPress={() => router.push(links.warranty(item.uuid))} />}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
        }}
        ListFooterComponent={<ListFooter loading={list.isFetchingNextPage} />}
        refreshControl={
          <RefreshControl refreshing={list.isRefetching && !list.isFetchingNextPage} onRefresh={() => void list.refetch()} />
        }
      />
    </>
  );
}
