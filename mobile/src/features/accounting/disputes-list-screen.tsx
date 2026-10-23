import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDate, PAGE_SIZE, useDebounced, usePagedList } from '@/features/services/lib';
import { Chips, ListBody, ListFooter, SearchField } from '@/features/services/ui';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { accountingKey, useAccountingAccess, useAccountingLinks, useCategoryLabels } from './hooks';
import { DISPUTE_STATUSES, type DisputeStatus, disputeTone, formatMoney } from './lib';

type Filter = 'all' | DisputeStatus;

/**
 * Disputes the read scope reaches (K24): a child sees the ones it opened, the
 * parent the ones addressed to it. Open is the default filter, as on the web.
 */
export function DisputesListScreen() {
  const { t, i18n } = useTranslation('accounting');
  const access = useAccountingAccess();
  const links = useAccountingLinks();
  const categories = useCategoryLabels(access.orgUuid, access.canRead);
  const [filter, setFilter] = useState<Filter>('open');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedList([...accountingKey(access.orgUuid), 'disputes', { filter, q: query }], (offset) =>
    unwrap(
      api.GET('/v1/accounting/disputes', {
        params: {
          query: {
            status: filter === 'all' ? undefined : filter,
            q: query || undefined,
            sort: '-created_at',
            limit: PAGE_SIZE,
            offset,
          },
        },
      }),
    ),
  );
  const filtered = !!query || filter !== 'all';

  return (
    <>
      <Stack.Screen options={{ title: t('disputes.title') }} />
      <FlatList
        data={list.items}
        keyExtractor={(d) => d.uuid}
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10"
        ListHeaderComponent={
          <View className="gap-3 pb-1 pt-2">
            <SearchField value={q} onChange={setQ} placeholder={t('disputes.search')} />
            <Chips<Filter>
              value={filter}
              onChange={setFilter}
              options={[
                { value: 'all', label: t('filter_all') },
                ...DISPUTE_STATUSES.map((s) => ({ value: s, label: t(`disputes.status.${s}`) })),
              ]}
            />
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
              icon: 'alert',
              title: filtered ? t('empty_filtered') : t('disputes.empty'),
              body: filtered ? undefined : t('disputes.empty_body'),
            }}
          />
        }
        renderItem={({ item: d }) => (
          <Pressable
            accessibilityRole="button"
            onPress={() => router.push(links.dispute(d.uuid))}
            className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
            <View className="flex-row items-center justify-between gap-3">
              <Text variant="headline" numberOfLines={1} className="flex-1">
                {d.organization.name}
              </Text>
              <StatusPill label={t(`disputes.status.${d.status}`)} tone={disputeTone(d.status)} />
            </View>
            <View className="flex-row items-center justify-between gap-3">
              <Text variant="callout" numberOfLines={1} className="flex-1">
                {categories.get(d.entry.category) ?? d.entry.category}
              </Text>
              <Text variant="callout" className="font-semibold tabular-nums">
                {formatMoney(d.entry.orig_amount, d.entry.orig_currency, i18n.language)}
              </Text>
            </View>
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {[t('disputes.to', { name: d.counterparty_organization.name }), formatDate(d.created_at, i18n.language)].join(' · ')}
            </Text>
          </Pressable>
        )}
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
