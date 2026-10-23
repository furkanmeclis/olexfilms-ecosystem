import type { Href } from 'expo-router';
import { router, Stack, useSegments } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useCan } from '@/core/spaces';
import { formatDate, formatDateTime, PAGE_SIZE, useDebounced, usePagedList } from '@/features/services/lib';
import { Chips, ListBody, ListFooter, SearchField } from '@/features/services/ui';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { type Campaign, CAMPAIGN_STATUSES, type CampaignStatus, campaignTone } from './lib';

/** Routes of the campaign screens inside the current space (`/<space>/menu/campaigns`). */
export function useCampaignLinks() {
  const space = (useSegments() as string[])[1] ?? 'bayi';
  const base = `/${space}/menu/campaigns`;
  return {
    detail: (uuid: string, review?: boolean) => (review ? `${base}/${uuid}?review=1` : `${base}/${uuid}`) as Href,
    approvals: () => `${base}/approvals` as Href,
  };
}

export const campaignKeys = {
  all: ['campaigns'] as const,
  detail: (uuid: string) => ['campaigns', 'detail', uuid] as const,
};

/** One campaign row of the list and the approval queue. */
export function CampaignRow({ c, onPress, showOrg }: { c: Campaign; onPress: () => void; showOrg?: boolean }) {
  const { t, i18n } = useTranslation('campaigns');
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="headline" numberOfLines={1} className="flex-1">
          {c.name}
        </Text>
        <StatusPill label={t(`status.${c.status}`)} tone={campaignTone(c.status)} />
      </View>
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {[showOrg ? c.organization_name : null, c.channels.map((ch) => t(`channel.${ch}`)).join(', ')]
          .filter(Boolean)
          .join(' · ')}
      </Text>
      <Text variant="footnote" tone="muted" numberOfLines={1}>
        {[
          t('recipients_count', { count: c.recipients_total }),
          c.scheduled_at
            ? t('scheduled_on', { date: formatDateTime(c.scheduled_at, i18n.language) })
            : formatDate(c.created_at, i18n.language),
        ].join(' · ')}
      </Text>
    </Pressable>
  );
}

type Filter = 'all' | CampaignStatus;

/** Campaigns of the organization: status filter, search, infinite scroll. */
export function CampaignsListScreen() {
  const { t } = useTranslation('campaigns');
  const links = useCampaignLinks();
  const canApprove = useCan('campaigns.approve');
  const [filter, setFilter] = useState<Filter>('all');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedList([...campaignKeys.all, 'list', { filter, q: query }], (offset) =>
    unwrap(
      api.GET('/v1/campaigns', {
        params: {
          query: {
            status: filter === 'all' ? undefined : [filter],
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
      <Stack.Screen
        options={{
          title: t('title'),
          headerRight: canApprove
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('actions.approvals')}
                  hitSlop={10}
                  onPress={() => router.push(links.approvals())}>
                  <Icon name="inbox" tone="primary" />
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
                ...CAMPAIGN_STATUSES.map((s) => ({ value: s, label: t(`status.${s}`) })),
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
              icon: 'campaign',
              title: filtered ? t('empty_filtered') : t('empty_title'),
              body: filtered ? undefined : t('empty_mobile'),
            }}
          />
        }
        renderItem={({ item }) => <CampaignRow c={item} onPress={() => router.push(links.detail(item.uuid))} />}
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

/** Campaigns waiting for this organization's decision (campaigns.approve). */
export function CampaignApprovalsScreen() {
  const { t } = useTranslation('campaigns');
  const links = useCampaignLinks();
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const list = usePagedList([...campaignKeys.all, 'approvals', { q: query }], (offset) =>
    unwrap(
      api.GET('/v1/campaigns/approvals', {
        params: { query: { q: query || undefined, sort: 'created_at', limit: PAGE_SIZE, offset } },
      }),
    ),
  );

  return (
    <>
      <Stack.Screen options={{ title: t('approvals.title') }} />
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
            <Text variant="callout" tone="muted">
              {t('approvals.description')}
            </Text>
            <SearchField value={q} onChange={setQ} placeholder={t('search')} />
          </View>
        }
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{
              icon: 'inbox',
              title: query ? t('empty_filtered') : t('approvals.empty_title'),
              body: query ? undefined : t('approvals.empty_description'),
            }}
          />
        }
        renderItem={({ item }) => <CampaignRow c={item} showOrg onPress={() => router.push(links.detail(item.uuid, true))} />}
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
