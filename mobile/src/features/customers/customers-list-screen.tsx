import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { fullName, PAGE_SIZE, useDebounced, useLinks, usePagedList } from '@/features/services/lib';
import { ListBody, ListFooter, SearchField } from '@/features/services/ui';
import { Text } from '@/ui/text';

/** Customers of the scope: search by name, phone or e-mail, infinite scroll. */
export function CustomersListScreen() {
  const { t } = useTranslation('dealer');
  const links = useLinks();
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedList(['customers', 'list', { q: query }], (offset) =>
    unwrap(api.GET('/v1/customers', { params: { query: { q: query || undefined, limit: PAGE_SIZE, offset } } })),
  );

  return (
    <>
      <Stack.Screen options={{ title: t('customers_title') }} />
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
            <SearchField value={q} onChange={setQ} placeholder={t('customer_search')} />
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
            empty={{ icon: 'customers', title: query ? t('customers_empty_filtered') : t('customers_empty') }}
          />
        }
        renderItem={({ item: c }) => (
          <Pressable
            accessibilityRole="button"
            disabled={c.anonymized}
            onPress={() => router.push(links.customer(c.uuid))}
            className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
            <View className="size-10 items-center justify-center rounded-full bg-secondary">
              <Text variant="callout" className="font-bold">
                {`${c.name?.[0] ?? ''}${c.surname?.[0] ?? ''}`.toUpperCase() || '?'}
              </Text>
            </View>
            <View className="flex-1 gap-0.5">
              <Text variant="body" numberOfLines={1}>
                {c.type === 'corporate' && c.company_name ? c.company_name : fullName(c)}
              </Text>
              <Text variant="footnote" tone="muted" numberOfLines={1}>
                {[c.type === 'corporate' && c.company_name ? fullName(c) : null, c.phone, c.email]
                  .filter(Boolean)
                  .join(' · ') || t(`customer_status.${c.status}`)}
              </Text>
            </View>
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
