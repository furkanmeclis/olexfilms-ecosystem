import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDate, PAGE_SIZE, useDebounced, usePagedList } from '@/features/services/lib';
import { Chips, ListBody, ListFooter, SearchField } from '@/features/services/ui';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { useAccountingLinks } from './hooks';
import { EINVOICE_STATUSES, type EinvoiceStatus, einvoiceTone, formatMoney } from './lib';

type Filter = 'all' | EinvoiceStatus;

/**
 * E-invoices of the brand center (read only on mobile). The backend keeps its
 * gates: einvoice.read, the center organization and the e_invoice add-on
 * (FEATURE_DISABLED shows as the list error).
 */
export function EinvoicesListScreen() {
  const { t, i18n } = useTranslation('accounting');
  const links = useAccountingLinks();
  const [filter, setFilter] = useState<Filter>('all');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedList(['einvoices', 'list', { filter, q: query }], (offset) =>
    unwrap(
      api.GET('/v1/einvoices', {
        params: {
          query: {
            status: filter === 'all' ? undefined : [filter],
            q: query || undefined,
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
      <Stack.Screen options={{ title: t('einvoice.title') }} />
      <FlatList
        data={list.items}
        keyExtractor={(i) => i.uuid}
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2 px-4 pb-10"
        ListHeaderComponent={
          <View className="gap-3 pb-1 pt-2">
            <SearchField value={q} onChange={setQ} placeholder={t('einvoice.search')} />
            <Chips<Filter>
              value={filter}
              onChange={setFilter}
              options={[
                { value: 'all', label: t('filter_all') },
                ...EINVOICE_STATUSES.map((s) => ({ value: s, label: t(`einvoice.status.${s}`) })),
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
              icon: 'report',
              title: filtered ? t('empty_filtered') : t('einvoice.empty'),
              body: filtered ? undefined : t('einvoice.empty_body'),
            }}
          />
        }
        renderItem={({ item: inv }) => (
          <Pressable
            accessibilityRole="button"
            onPress={() => router.push(links.einvoice(inv.uuid))}
            className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
            <View className="flex-row items-center justify-between gap-3">
              <Text variant="headline" numberOfLines={1} className="flex-1 font-mono">
                {inv.number ?? t('einvoice.draft_number')}
              </Text>
              <StatusPill label={t(`einvoice.status.${inv.status}`)} tone={einvoiceTone(inv.status)} />
            </View>
            <View className="flex-row items-center justify-between gap-3">
              <Text variant="callout" numberOfLines={1} className="flex-1">
                {inv.buyer_organization?.name ?? inv.buyer.name}
              </Text>
              <Text variant="callout" className="font-semibold tabular-nums">
                {formatMoney(inv.payable, inv.currency, i18n.language)}
              </Text>
            </View>
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {[t(`einvoice.profile.${inv.profile}`), formatDate(inv.issue_date, i18n.language)].join(' · ')}
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
