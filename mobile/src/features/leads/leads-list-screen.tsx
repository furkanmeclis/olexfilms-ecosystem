import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useCan } from '@/core/spaces';
import { formatDate, PAGE_SIZE, useDebounced, usePagedList } from '@/features/services/lib';
import { Chips, ListBody, ListFooter, SearchField } from '@/features/services/ui';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { useLeadLinks } from './hooks';
import { isOverdue, LEAD_STATUSES, type LeadStatus, leadName, leadStatusTone, leadTemperatureTone } from './lib';

type Filter = 'all' | LeadStatus;
type FollowUp = 'any' | 'today' | 'overdue';

/** Leads of the scope: filter by status and follow-up, search, infinite scroll. */
export function LeadsListScreen() {
  const { t, i18n } = useTranslation('leads');
  const links = useLeadLinks();
  const canWrite = useCan('leads.write');
  const [filter, setFilter] = useState<Filter>('all');
  const [followUp, setFollowUp] = useState<FollowUp>('any');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedList(['leads', 'list', { filter, followUp, q: query }], (offset) =>
    unwrap(
      api.GET('/v1/leads', {
        params: {
          query: {
            status: filter === 'all' ? undefined : filter,
            follow_up: followUp === 'any' ? undefined : followUp,
            q: query || undefined,
            sort: '-created_at',
            limit: PAGE_SIZE,
            offset,
          },
        },
      }),
    ),
  );
  const filtered = !!query || filter !== 'all' || followUp !== 'any';

  return (
    <>
      <Stack.Screen
        options={{
          title: t('title'),
          headerRight: canWrite
            ? () => (
                <Pressable accessibilityRole="button" accessibilityLabel={t('new')} hitSlop={10} onPress={() => router.push(links.create())}>
                  <Icon name="add" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <FlatList
        data={list.items}
        keyExtractor={(l) => l.uuid}
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
                ...LEAD_STATUSES.map((s) => ({ value: s, label: t(`status.${s}`) })),
              ]}
            />
            <Chips<FollowUp>
              value={followUp}
              onChange={setFollowUp}
              options={[
                { value: 'any', label: t('follow_up.any') },
                { value: 'today', label: t('follow_up.today') },
                { value: 'overdue', label: t('follow_up.overdue') },
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
              icon: 'lead',
              title: filtered ? t('empty_filtered') : t('empty'),
              body: filtered ? undefined : t('empty_body'),
              action: !filtered && canWrite ? { title: t('new'), onPress: () => router.push(links.create()) } : undefined,
            }}
          />
        }
        renderItem={({ item: l }) => (
          <Pressable
            accessibilityRole="button"
            onPress={() => router.push(links.detail(l.uuid))}
            className="gap-1 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
            <View className="flex-row items-center justify-between gap-3">
              <Text variant="headline" numberOfLines={1} className="flex-1">
                {leadName(l) || l.candidate_phone_e164 || l.candidate_email || t(`target.${l.target_type}`)}
              </Text>
              <StatusPill label={t(`status.${l.status}`)} tone={leadStatusTone(l.status)} />
            </View>
            <View className="flex-row flex-wrap items-center gap-x-2 gap-y-1">
              <StatusPill label={t(`temperature.${l.temperature}`)} tone={leadTemperatureTone(l.temperature)} />
              <Text variant="footnote" tone="muted">
                {t(`source.${l.source}`)}
              </Text>
            </View>
            <Text variant="footnote" tone={isOverdue(l.follow_up_date, l.status) ? 'danger' : 'muted'} numberOfLines={1}>
              {[
                leadName(l) ? l.candidate_phone_e164 : null,
                leadName(l) || l.candidate_phone_e164 ? l.candidate_email : null,
                l.follow_up_date ? t('follow_up_on', { date: formatDate(l.follow_up_date, i18n.language) }) : null,
              ]
                .filter(Boolean)
                .join(' · ') || formatDate(l.created_at, i18n.language)}
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
