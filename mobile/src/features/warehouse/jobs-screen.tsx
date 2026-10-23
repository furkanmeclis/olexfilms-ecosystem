import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Pressable, RefreshControl, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { StatusPill } from '@/ui/status-pill';

import { type StockCount, type StockEntry, type Transfer, useCounts, useEntries, useTransfers } from './api';
import { countTone, entryTone, formatDate, go, transferTone } from './format';
import { orderTone } from './order-screen';
import { type Order, type OrderScope, useOrders } from './orders-api';
import { Chips, DocCard, ListStatus, Segmented } from './parts';

export type JobsTab = 'transfers' | 'counts' | 'entries' | 'orders';
type Scope = 'open' | 'done' | 'all';

const STATUS: Record<Exclude<JobsTab, 'orders'>, Record<Scope, string>> = {
  transfers: { open: 'draft,in_transit', done: 'completed,cancelled', all: '' },
  counts: { open: 'draft,in_progress,pending_review', done: 'approved,cancelled', all: '' },
  entries: { open: 'draft', done: 'confirmed,cancelled,undone', all: '' },
};

/** Order scope per chip: open = to prepare (seller), closed = to receive and shelve (buyer). */
const ORDER_SCOPE: Record<Scope, OrderScope> = { open: 'prepare', done: 'receive', all: 'all' };

/** Warehouse jobs: transfers, stock counts, stock entries and orders, open ones first. */
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
  const orders = useCan('orders.read');

  const newPath =
    tab === 'orders'
      ? null
      : { transfers: '/depo/jobs/transfer/new', counts: '/depo/jobs/count/new', entries: '/depo/jobs/entry/new' }[tab];

  return (
    <>
      <Stack.Screen
        options={{
          title: t('nav:jobs'),
          headerRight:
            write && newPath
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t(tab === 'transfers' ? 'transfers.new' : tab === 'counts' ? 'counts.new' : 'entries.new')}
                  hitSlop={8}
                  onPress={() => go(newPath)}
                  className="size-8 items-center justify-center">
                  <Icon name="add" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <JobsList
        tab={tab}
        status={tab === 'orders' ? '' : STATUS[tab][scope]}
        orderScope={ORDER_SCOPE[scope]}
        header={
          <View className="gap-3 pb-2">
            <Segmented
              value={tab}
              onChange={setTab}
              options={[
                { value: 'transfers', label: t('m.tab_transfers') },
                { value: 'counts', label: t('m.tab_counts') },
                { value: 'entries', label: t('m.tab_entries') },
                ...(orders ? [{ value: 'orders' as const, label: t('orders.nav') }] : []),
              ]}
            />
            <Chips
              value={scope}
              onChange={setScope}
              options={
                tab === 'orders'
                  ? [
                      { value: 'open', label: t('m.to_prepare') },
                      { value: 'done', label: t('m.to_receive') },
                      { value: 'all', label: t('m.all') },
                    ]
                  : [
                      { value: 'open', label: t('m.open') },
                      { value: 'done', label: t('m.closed') },
                      { value: 'all', label: t('m.all') },
                    ]
              }
            />
          </View>
        }
        onCreate={write && newPath ? () => go(newPath) : undefined}
      />
    </>
  );
}

function JobsList({
  tab,
  status,
  orderScope,
  header,
  onCreate,
}: {
  tab: JobsTab;
  status: string;
  orderScope: OrderScope;
  header: React.ReactElement;
  onCreate?: () => void;
}) {
  const { t } = useTranslation('warehouse');
  // Only the visible tab fetches; the others keep their cache.
  const transfers = useTransfers(status, tab === 'transfers');
  const counts = useCounts(status, tab === 'counts');
  const entries = useEntries(status, tab === 'entries');
  const orders = useOrders(orderScope, tab === 'orders');
  const q = tab === 'transfers' ? transfers : tab === 'counts' ? counts : tab === 'entries' ? entries : orders;

  const items: (Transfer | StockCount | StockEntry | Order)[] =
    tab === 'transfers'
      ? (transfers.data?.pages.flatMap((p) => p.items) ?? [])
      : tab === 'counts'
        ? (counts.data?.pages.flatMap((p) => p.items) ?? [])
        : tab === 'entries'
          ? (entries.data?.pages.flatMap((p) => p.items) ?? [])
          : (orders.data?.pages.flatMap((p) => p.items) ?? []);

  const empties: Record<JobsTab, { icon: IconName; title: string; body: string; action: string }> = {
    transfers: { icon: 'transfer', title: t('transfers.empty_title'), body: t('transfers.empty_description'), action: t('transfers.new') },
    counts: { icon: 'task', title: t('counts.empty_title'), body: t('counts.empty_description'), action: t('counts.new') },
    entries: { icon: 'packageIn', title: t('entries.empty_title'), body: t('entries.empty_description'), action: t('entries.new') },
    orders: { icon: 'package', title: t('orders.list.empty_title'), body: t('orders.list.empty_description'), action: '' },
  };
  const empty = empties[tab];

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
        ) : tab === 'entries' ? (
          <EntryCard entry={item as StockEntry} />
        ) : (
          <OrderCard order={item as Order} />
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

export function OrderCard({ order: x }: { order: Order }) {
  const { t } = useTranslation('warehouse');
  const other = x.role === 'buyer' ? x.seller : x.buyer;
  return (
    <DocCard
      title={x.order_no}
      subtitle={`${t(`orders.role.${x.role}`)} · ${other.name}`}
      meta={[formatDate(x.created_at), x.delivery_method?.name, x.note].filter(Boolean).join(' · ')}
      status={<StatusPill tone={orderTone(x.status)} label={t(`orders.status.${x.status}`)} />}
      onPress={() => go(`/depo/jobs/order/${x.uuid}`)}
    />
  );
}
