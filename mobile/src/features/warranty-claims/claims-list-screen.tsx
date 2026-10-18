import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDate, PAGE_SIZE, useDebounced, usePagedList } from '@/features/services/lib';
import { Chips, ListBody, ListFooter, SearchField } from '@/features/services/ui';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { useClaimGrants, useClaimLinks } from './hooks';
import { CLAIM_STATUSES, type ClaimStatus, claimTone, statusParam } from './lib';

type Filter = 'all' | ClaimStatus;

/** Warranty claims of the scope: filter by status, search, infinite scroll. */
export function ClaimsListScreen() {
  const { t, i18n } = useTranslation('claims');
  const links = useClaimLinks();
  const { grants, orgType } = useClaimGrants();
  const [filter, setFilter] = useState<Filter>('all');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedList(['warranty-claims', 'list', { filter, q: query }], (offset) =>
    unwrap(
      api.GET('/v1/warranty-claims', {
        params: { query: { status: statusParam(filter), q: query || undefined, limit: PAGE_SIZE, offset } },
      }),
    ),
  );
  // The center decides on claims; dealers and distributors open them.
  const canOpen = grants.write && orgType !== 'center';

  return (
    <>
      <Stack.Screen
        options={{
          title: t('title'),
          headerRight: canOpen
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('new')}
                  hitSlop={10}
                  onPress={() => router.push(links.create())}>
                  <Icon name="add" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <FlatList
        data={list.items}
        keyExtractor={(c) => c.uuid}
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10"
        ListHeaderComponent={
          <View className="gap-3 pb-1 pt-2">
            <SearchField value={q} onChange={setQ} placeholder={t('search')} />
            <Chips<Filter>
              value={filter}
              onChange={setFilter}
              options={[
                { value: 'all', label: t('filter_all') },
                ...CLAIM_STATUSES.map((s) => ({ value: s, label: t(`status.${s}`) })),
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
              icon: 'warrantyClaim',
              title: query || filter !== 'all' ? t('empty_filtered') : t('empty'),
              body: query || filter !== 'all' ? undefined : t('empty_body'),
            }}
          />
        }
        renderItem={({ item: c }) => (
          <Pressable
            accessibilityRole="button"
            onPress={() => router.push(links.detail(c.uuid))}
            className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
            <View className="flex-row items-center justify-between gap-3">
              <Text variant="headline" numberOfLines={1} className="flex-1">
                {t('claim_no', { no: c.claim_no })}
              </Text>
              <StatusPill label={t(`status.${c.status}`)} tone={claimTone(c.status)} />
            </View>
            {c.product_name ? (
              <Text variant="callout" numberOfLines={1}>
                {c.product_name}
              </Text>
            ) : null}
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {[c.plate, c.organization_name, formatDate(c.created_at, i18n.language)].filter(Boolean).join(' · ')}
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
