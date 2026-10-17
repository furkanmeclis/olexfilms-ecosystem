import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { paths } from '@/core/api/schema';
import { errorMessage } from '@/core/i18n';
import {
  CardRow,
  Chips,
  ErrorState,
  InfoRow,
  ListSkeleton,
  PagedList,
  PAGE_SIZE,
  useChildHref,
  useDebounced,
  useFormat,
  usePagedQuery,
} from '@/features/admin/kit';
import { ShoppingCart } from '@/ui/icons';
import { Button } from '@/ui/button';
import { List, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

type ListOp = paths['/v1/orders']['get'];
export type Order = ListOp['responses'][200]['content']['application/json']['data']['items'][number];
export type OrderStatus = Order['status'];

type Filter = 'all' | 'awaiting' | 'active' | 'shipped' | 'done' | 'cancelled' | 'draft';

const FILTERS: Record<Filter, OrderStatus[] | undefined> = {
  all: undefined,
  awaiting: ['submitted'],
  active: ['approved', 'preparing', 'ready', 'processing'],
  shipped: ['shipped'],
  done: ['received', 'delivered'],
  cancelled: ['cancelling', 'cancelled'],
  draft: ['draft'],
};

/** Transitions offered on mobile: the ones that need no warehouse or shipping form. */
const SIMPLE_TRANSITIONS: OrderStatus[] = ['submitted', 'approved', 'received', 'cancelled'];

export function orderTone(s: OrderStatus): StatusTone {
  if (s === 'received' || s === 'delivered') return 'done';
  if (s === 'cancelled' || s === 'cancelling') return 'problem';
  if (s === 'draft') return 'neutral';
  return 'progress';
}

export function OrdersScreen() {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const href = useChildHref();
  const [filter, setFilter] = useState<Filter>('all');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());

  const list = usePagedQuery<Order>(['orders', 'list', filter, query], (offset) =>
    unwrap(
      api.GET('/v1/orders', {
        params: {
          query: { status: FILTERS[filter]?.join(','), q: query || undefined, limit: PAGE_SIZE, offset },
        },
      }),
    ),
  );

  return (
    <>
      <Stack.Screen
        options={{
          title: t('orders.title'),
          headerSearchBarOptions: {
            placeholder: t('orders.search'),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(''),
          },
        }}
      />
      <PagedList
        query={list}
        keyOf={(o) => o.uuid}
        header={
          <Chips
            options={(Object.keys(FILTERS) as Filter[]).map((k) => ({ key: k, label: t(`orders.filter.${k}`) }))}
            value={filter}
            onChange={setFilter}
          />
        }
        empty={{ icon: ShoppingCart, title: query ? t('orders.no_results') : t('orders.empty') }}
        renderItem={({ item }) => {
          const other = item.role === 'buyer' ? item.seller : item.buyer;
          return (
            <CardRow
              title={item.order_no}
              subtitle={
                item.role === 'observer'
                  ? `${item.seller.name} → ${item.buyer.name}`
                  : t(item.role === 'seller' ? 'orders.to' : 'orders.from', { name: other.name })
              }
              meta={`${f.money(item.total, item.currency)} · ${f.date(item.created_at)}`}
              trailing={<StatusPill label={item.status_label} tone={orderTone(item.status)} />}
              onPress={() => router.push(href(item.uuid))}
            />
          );
        }}
      />
    </>
  );
}

export function OrderDetailScreen() {
  const { t } = useTranslation('admin');
  const f = useFormat();
  const qc = useQueryClient();
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const q = useQuery({
    queryKey: ['orders', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/orders/{uuid}', { params: { path: { uuid: uuid! } } })),
    enabled: !!uuid,
  });
  const move = useMutation({
    mutationFn: (status: OrderStatus) =>
      unwrap(api.POST('/v1/orders/{uuid}/transitions', { params: { path: { uuid: uuid! } }, body: { status } })),
    onSuccess: (data) => {
      qc.setQueryData(['orders', 'detail', uuid], data);
      void qc.invalidateQueries({ queryKey: ['orders', 'list'] });
    },
    onError: (e) => Alert.alert(errorMessage(isApiError(e) ? e.code : undefined)),
  });
  const o = q.data;
  const actions = (o?.available_transitions ?? []).filter((s) => SIMPLE_TRANSITIONS.includes(s));

  const confirm = (status: OrderStatus) =>
    Alert.alert(t(`orders.confirm.${status}`), o?.order_no, [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t(`orders.action.${status}`),
        style: status === 'cancelled' ? 'destructive' : 'default',
        onPress: () => move.mutate(status),
      },
    ]);

  return (
    <>
      <Stack.Screen options={{ title: o?.order_no ?? '', headerLargeTitle: false }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : !o ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <View className="gap-2">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="title" selectable>
                  {o.order_no}
                </Text>
                <StatusPill label={o.status_label} tone={orderTone(o.status)} />
              </View>
              <Text variant="title" className="tabular-nums">
                {f.money(o.total, o.currency)}
              </Text>
            </View>

            {actions.length ? (
              <View className="gap-2">
                {actions.map((s) => (
                  <Button
                    key={s}
                    title={t(`orders.action.${s}`)}
                    variant={s === 'cancelled' ? 'outline' : 'primary'}
                    loading={move.isPending && move.variables === s}
                    disabled={move.isPending}
                    onPress={() => confirm(s)}
                  />
                ))}
              </View>
            ) : null}

            <List>
              <InfoRow label={t('orders.seller')} value={o.seller.name} />
              <InfoRow label={t('orders.buyer')} value={o.buyer.name} />
              <InfoRow label={t('orders.created')} value={f.dateTime(o.created_at)} />
              {o.delivery_method ? <InfoRow label={t('orders.delivery')} value={o.delivery_method.name} /> : null}
              {o.tracking_no ? <InfoRow label={t('orders.tracking')} value={o.tracking_no} /> : null}
              <InfoRow label={t('orders.subtotal')} value={f.money(o.subtotal, o.currency)} />
              <InfoRow label={t('orders.tax')} value={f.money(o.tax_total, o.currency)} last />
            </List>

            {o.note || o.cancel_reason ? (
              <View className="gap-1 rounded-lg border border-border bg-card px-4 py-3">
                {o.note ? <Text variant="callout">{o.note}</Text> : null}
                {o.cancel_reason ? (
                  <Text variant="callout" tone="danger">
                    {o.cancel_reason}
                  </Text>
                ) : null}
              </View>
            ) : null}

            <SectionHeader title={t('orders.items', { count: o.items?.length ?? 0 })} />
            {o.items?.length ? (
              <List>
                {o.items.map((it, i, arr) => (
                  <View key={it.uuid} className={i < arr.length - 1 ? 'gap-0.5 border-b border-border px-4 py-3' : 'gap-0.5 px-4 py-3'}>
                    <Text variant="body" numberOfLines={2}>
                      {it.product.name}
                    </Text>
                    <View className="flex-row justify-between gap-3">
                      <Text variant="footnote" tone="muted">
                        {it.product.sku} ·{' '}
                        {it.meters !== null ? t('orders.meters', { value: f.number(it.meters, 2) }) : t('orders.pieces', { count: it.quantity ?? 0 })}
                      </Text>
                      <Text variant="footnote" className="font-semibold tabular-nums">
                        {f.money(it.line_total, o.currency)}
                      </Text>
                    </View>
                  </View>
                ))}
              </List>
            ) : (
              <Text variant="callout" tone="muted" className="px-4">
                {t('warehouse:orders.detail.items_empty')}
              </Text>
            )}

            {o.history?.length ? (
              <>
                <SectionHeader title={t('orders.history')} />
                <List>
                  {o.history.map((h, i, arr) => (
                    <InfoRow
                      key={`${h.created_at}-${i}`}
                      label={t(`orders.status.${h.to_status}`)}
                      value={f.dateTime(h.created_at)}
                      last={i === arr.length - 1}
                    />
                  ))}
                </List>
              </>
            ) : null}
          </>
        )}
      </Screen>
    </>
  );
}
