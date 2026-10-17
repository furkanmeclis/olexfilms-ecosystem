import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { ArrowLeftRight, ClipboardList, PackagePlus, Plus } from '@/ui/icons';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';

import { type StockCount, type StockEntry, type Transfer, useCounts, useEntries, useTransfers } from './api';
import { countTone, entryTone, formatDate, go, transferTone } from './format';
import { Chips, DocCard, ListStatus, Segmented } from './parts';

export type JobsTab = 'transfers' | 'counts' | 'entries';
type Scope = 'open' | 'done' | 'all';

const STATUS: Record<JobsTab, Record<Scope, string>> = {
  transfers: { open: 'draft,in_transit', done: 'completed,cancelled', all: '' },
  counts: { open: 'draft,in_progress,pending_review', done: 'approved,cancelled', all: '' },
  entries: { open: 'draft', done: 'confirmed,cancelled,undone', all: '' },
};

/** Warehouse jobs: transfers, stock counts and stock entries, open ones first. */
export function JobsScreen() {
  const { t } = useTranslation('warehouse');
  const params = useLocalSearchParams<{ tab?: JobsTab }>();
  const [tab, setTab] = useState<JobsTab>(params.tab ?? 'transfers');
  // Home counters open a given tab, also when this screen is already mounted.
  const [seenTab, setSeenTab] = useState(params.tab);
  if (params.tab !== seenTab) {
    setSeenTab(params.tab);
    if (params.tab) setTab(params.tab);
  }
  const [scope, setScope] = useState<Scope>('open');
  const write = useCan('warehouse.write');

  const newPath = { transfers: '/depo/jobs/transfer/new', counts: '/depo/jobs/count/new', entries: '/depo/jobs/entry/new' }[tab];

  return (
    <>
      <Stack.Screen
        options={{
          title: t('nav:jobs'),
          headerRight: write
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t(tab === 'transfers' ? 'transfers.new' : tab === 'counts' ? 'counts.new' : 'entries.new')}
                  hitSlop={8}
                  onPress={() => go(newPath)}
                  className="size-8 items-center justify-center">
                  <Icon as={Plus} tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <JobsList
        tab={tab}
        status={STATUS[tab][scope]}
        header={
          <View className="gap-3 pb-2">
            <Segmented
              value={tab}
              onChange={setTab}
              options={[
                { value: 'transfers', label: t('nav_transfers') },
                { value: 'counts', label: t('nav_counts') },
                { value: 'entries', label: t('nav_entries') },
              ]}
            />
            <Chips
              value={scope}
              onChange={setScope}
              options={[
                { value: 'open', label: t('m.open') },
                { value: 'done', label: t('m.closed') },
                { value: 'all', label: t('m.all') },
              ]}
            />
          </View>
        }
        onCreate={write ? () => go(newPath) : undefined}
      />
    </>
  );
}

function JobsList({
  tab,
  status,
  header,
  onCreate,
}: {
  tab: JobsTab;
  status: string;
  header: React.ReactElement;
  onCreate?: () => void;
}) {
  const { t } = useTranslation('warehouse');
  // Only the visible tab fetches; the others keep their cache.
  const transfers = useTransfers(status, tab === 'transfers');
  const counts = useCounts(status, tab === 'counts');
  const entries = useEntries(status, tab === 'entries');
  const q = tab === 'transfers' ? transfers : tab === 'counts' ? counts : entries;

  const items: (Transfer | StockCount | StockEntry)[] =
    tab === 'transfers'
      ? (transfers.data?.pages.flatMap((p) => p.items) ?? [])
      : tab === 'counts'
        ? (counts.data?.pages.flatMap((p) => p.items) ?? [])
        : (entries.data?.pages.flatMap((p) => p.items) ?? []);

  const empty = {
    transfers: { icon: ArrowLeftRight, title: t('transfers.empty_title'), body: t('transfers.empty_description'), action: t('transfers.new') },
    counts: { icon: ClipboardList, title: t('counts.empty_title'), body: t('counts.empty_description'), action: t('counts.new') },
    entries: { icon: PackagePlus, title: t('entries.empty_title'), body: t('entries.empty_description'), action: t('entries.new') },
  }[tab];

  return (
    <FlatList
      data={items}
      keyExtractor={(i) => i.uuid}
      contentInsetAdjustmentBehavior="automatic"
      className="flex-1 bg-background"
      contentContainerClassName="gap-2 px-4 pb-10 pt-2"
      refreshControl={<RefreshControl refreshing={q.isRefetching && !q.isFetchingNextPage} onRefresh={() => void q.refetch()} />}
      onEndReachedThreshold={0.5}
      onEndReached={() => q.hasNextPage && !q.isFetchingNextPage && void q.fetchNextPage()}
      ListHeaderComponent={header}
      ListEmptyComponent={
        <ListStatus
          isLoading={q.isLoading}
          error={q.error}
          empty
          onRetry={() => void q.refetch()}
          emptyIcon={empty.icon}
          emptyTitle={empty.title}
          emptyBody={empty.body}
          emptyAction={onCreate ? { title: empty.action, onPress: onCreate } : undefined}
        />
      }
      renderItem={({ item }) =>
        tab === 'transfers' ? (
          <TransferCard transfer={item as Transfer} />
        ) : tab === 'counts' ? (
          <CountCard count={item as StockCount} />
        ) : (
          <EntryCard entry={item as StockEntry} />
        )
      }
    />
  );
}

export function TransferCard({ transfer: x }: { transfer: Transfer }) {
  const { t } = useTranslation('warehouse');
  return (
    <DocCard
      title={x.transfer_no}
      subtitle={`${x.from_warehouse.name} → ${x.to_warehouse.name}`}
      meta={[t('m.lines_count', { count: x.line_count }), formatDate(x.created_at), x.note].filter(Boolean).join(' · ')}
      status={<StatusPill tone={transferTone(x.status)} label={t(`transfer_status.${x.status}`)} />}
      onPress={() => go(`/depo/jobs/transfer/${x.uuid}`)}
    />
  );
}

export function CountCard({ count: x }: { count: StockCount }) {
  const { t } = useTranslation('warehouse');
  const scope =
    x.scope_type === 'warehouse'
      ? t('count_scope.warehouse')
      : (x.scope_room?.name ?? x.scope_location?.full_code ?? x.scope_product?.name ?? t(`count_scope.${x.scope_type}`));
  return (
    <DocCard
      title={`${x.warehouse.name} · ${scope}`}
      subtitle={`${t(`count_method.${x.method}`)} · ${t(`count_visibility.${x.visibility}`)}`}
      meta={[formatDate(x.created_at), x.note].filter(Boolean).join(' · ')}
      status={<StatusPill tone={countTone(x.status)} label={t(`count_status.${x.status}`)} />}
      onPress={() => go(`/depo/jobs/count/${x.uuid}`)}
    />
  );
}

export function EntryCard({ entry: x }: { entry: StockEntry }) {
  const { t } = useTranslation('warehouse');
  return (
    <DocCard
      title={x.warehouse?.name ?? t('entries.no_warehouse')}
      subtitle={t(`entry_mode.${x.mode}`)}
      meta={[t('m.lines_count', { count: x.line_count }), formatDate(x.created_at), x.note].filter(Boolean).join(' · ')}
      status={<StatusPill tone={entryTone(x.status)} label={t(`entry_status.${x.status}`)} />}
      onPress={() => go(`/depo/jobs/entry/${x.uuid}`)}
    />
  );
}
