import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDate, PAGE_SIZE, useDebounced, usePagedList } from '@/features/services/lib';
import { Chips, ListBody, ListFooter, SearchField } from '@/features/services/ui';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { accountingKey, useAccountingAccess, useAccountingLinks } from './hooks';
import { balanceKind, balanceTone, counterpartyKind, formatMoney } from './lib';

const KINDS = ['center', 'distributor', 'dealer', 'fleet', 'customer'] as const;
type Kind = 'all' | (typeof KINDS)[number];

/**
 * Cari accounts of the active book (TEC-176 on the web). The backend scopes the
 * list: a dealer sees its parent and its customers, a distributor its parent and
 * its dealers, the center every child. Active accounts only, as the web starts.
 */
export function CariListScreen() {
  const { t, i18n } = useTranslation('accounting');
  const access = useAccountingAccess();
  const links = useAccountingLinks();
  const [kind, setKind] = useState<Kind>('all');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedList([...accountingKey(access.orgUuid), 'cari', { kind, q: query }], (offset) =>
    unwrap(
      api.GET('/v1/accounting/cari', {
        params: {
          query: {
            active: true,
            counterparty_kind: kind === 'all' ? undefined : kind,
            q: query || undefined,
            sort: 'name',
            limit: PAGE_SIZE,
            offset,
          },
        },
      }),
    ),
  );
  const filtered = !!query || kind !== 'all';

  return (
    <>
      <Stack.Screen options={{ title: t('cari.title') }} />
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
            <SearchField value={q} onChange={setQ} placeholder={t('cari.search')} />
            <Chips<Kind>
              value={kind}
              onChange={setKind}
              options={[
                { value: 'all', label: t('filter_all') },
                ...KINDS.map((k) => ({ value: k, label: t(`counterparty.${k}`) })),
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
              icon: 'accounting',
              title: filtered ? t('empty_filtered') : t('cari.empty'),
              body: filtered ? undefined : t('cari.empty_body'),
            }}
          />
        }
        renderItem={({ item: c }) => {
          const side = balanceKind(c.balance);
          return (
            <Pressable
              accessibilityRole="button"
              onPress={() => router.push(links.cari(c.uuid))}
              className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="headline" numberOfLines={1} className="flex-1">
                  {c.counterparty.name}
                </Text>
                <StatusPill label={t(`balance.${side}`)} tone={balanceTone(side)} />
              </View>
              <Text variant="callout" className="font-semibold tabular-nums">
                {formatMoney(Math.abs(Number(c.balance)), c.currency, i18n.language)}
              </Text>
              <Text variant="footnote" tone="muted" numberOfLines={1}>
                {[
                  t(`counterparty.${counterpartyKind(c)}`),
                  t('entry_count', { count: c.entry_count }),
                  c.last_entry_at ? formatDate(c.last_entry_at, i18n.language) : null,
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </Text>
            </Pressable>
          );
        }}
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
