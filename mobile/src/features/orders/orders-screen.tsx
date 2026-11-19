import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack, useLocalSearchParams, usePathname } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { useSessionCurrencies } from '@/core/currency';
import type { paths } from '@/core/api/schema';
import { hasAmount } from '@/core/price-visibility';
import { usePricesVisible } from '@/core/use-prices-visible';
import { isCenterWorkplace, kindsOf } from '@/core/spaces';
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
import { RefRow, RefText } from '@/features/entity/ref-row';
import { CartButton, CATALOG_PRODUCTS, useCanOrder } from '@/features/catalog/catalog-screen';
import { OrderReturnsSection } from '@/features/transfers/order-returns';
import { confirmAction } from '@/ui/confirm';
import { Button } from '@/ui/button';
import { List, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { PullRefresh } from '@/ui/pull-refresh';
import { toast } from '@/ui/toast';

import { useCart } from './cart';
import { MeasureBadge } from './line-fields';
import { LinePlotter, OrderLinesEditor } from './order-edit';
import { creatorOf, liveTryAmount, showsMeasure } from './order-summary';
import { OrderTimeline, useActivityText, useAmountText } from './order-timeline';
import { BackorderRows, DiscrepancySection, ReceiptCheckSheet } from './discrepancy';
import { canStartSupplement, orderSubtitle, supplementParentOf } from './supplement';

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
  // TEC-1214: the small figure is in the local display currency.
  const { display: local } = useSessionCurrencies();
  const { t } = useTranslation('admin');
  const f = useFormat();
  const href = useChildHref();
  // Dealers order from the catalog: the cart opens the new-order screen.
  const pathname = usePathname();
  const canOrder = useCanOrder();
  const cart = canOrder && pathname.startsWith('/bayi');
  // TEC-1055 (revize 5.1): a new order starts in the product list itself; the
  // header cart icon still opens the cart.
  const openNewOrder = () => router.push(CATALOG_PRODUCTS as never);
  const amountText = useAmountText();
  const activityText = useActivityText();
  // TEC-1025: the center prepares what it sells in the warehouse screens
  // (assign units, ready, ship, deliver); a seller row opens there when it can.
  const me = useSession((s) => s.me);
  // In the warehouse space (Menu > Orders) a seller row always opens the
  // preparation screen, not the read-only detail.
  const openInWarehouse = (o: Order) =>
    o.role === 'seller' &&
    kindsOf(me).includes('depo') &&
    (pathname.startsWith('/depo') || (isCenterWorkplace(me) && pathname.startsWith('/bayi')));
  // TEC-1051: no totals in the warehouse space or for a warehouse role.
  const prices = usePricesVisible();
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
          headerRight: cart ? () => <CartButton /> : undefined,
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
          <View className="gap-3">
            {cart ? (
              // A written button next to the header cart icon: ordering starts here.
              <View className="px-4">
                <Button title={t('orders.new')} icon="add" onPress={openNewOrder} />
              </View>
            ) : null}
            <Chips
              options={(Object.keys(FILTERS) as Filter[]).map((k) => ({ key: k, label: t(`orders.filter.${k}`) }))}
              value={filter}
              onChange={setFilter}
            />
          </View>
        }
        empty={
          query
            ? { icon: 'cart', title: t('orders.no_results') }
            : {
                icon: 'cart',
                title: t('orders.empty'),
                body: cart ? t('orders.empty_body') : undefined,
                action: cart ? { title: t('orders.new'), onPress: openNewOrder } : undefined,
              }
        }
        renderItem={({ item }) => {
          const other = item.role === 'buyer' ? item.seller : item.buyer;
          // The dealer's name leads the card so everyone recognises the order at a glance; the number follows.
          const party = item.role === 'observer' ? `${item.seller.name} → ${item.buyer.name}` : other.name;
          // TEC-1055 (revize 5): warehouse, amount, brand currency with a small live local figure, who handles it.
          const tl = liveTryAmount(item, local);
          const money =
            prices && hasAmount(item.total)
              ? `${f.money(item.total, item.currency)}${tl && local ? ` (≈ ${f.money(tl, local)})` : ''}`
              : null;
          return (
            <CardRow
              title={party}
              subtitle={orderSubtitle(item, {
                supplement: t('orders:supplement.badge'),
                warehouse: item.seller_warehouse ? t('orders:card.warehouse', { name: item.seller_warehouse.name }) : null,
              })}
              meta={[amountText(item), money, f.date(item.created_at)].filter(Boolean).join(' · ')}
              detail={activityText(item.last_activity)}
              trailing={<StatusPill label={item.status_label} tone={orderTone(item.status)} />}
              onPress={() => router.push(openInWarehouse(item) ? (`/depo/jobs/order/${item.uuid}` as never) : href(item.uuid))}
            />
          );
        }}
      />
    </>
  );
}

export function OrderDetailScreen() {
  const { display: local } = useSessionCurrencies();
  const { t } = useTranslation('admin');
  const f = useFormat();
  const qc = useQueryClient();
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const prices = usePricesVisible();
  const amountText = useAmountText();
  const [editing, setEditing] = useState(false);
  // TEC-1227: receive with a check (missing / damaged units).
  const [checking, setChecking] = useState(false);
  // TEC-1222: "Add supplementary shipment" fills the cart for a new order of the same dealer.
  const pathname = usePathname();
  const canOrder = useCanOrder();
  const cartLines = useCart((s) => s.lines.length);
  const startSupplement = useCart((s) => s.startSupplement);
  const q = useQuery({
    queryKey: ['orders', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/orders/{uuid}', { params: { path: { uuid: uuid! } } })),
    enabled: !!uuid,
  });
  const move = useMutation({
    mutationFn: (status: OrderStatus) =>
      unwrap(api.POST('/v1/orders/{uuid}/transitions', { params: { path: { uuid: uuid! } }, body: { status } })),
    onSuccess: (data, status) => {
      qc.setQueryData(['orders', 'detail', uuid], data);
      void qc.invalidateQueries({ queryKey: ['orders', 'list'] });
      // The approvals inbox lists submitted orders under its own key.
      void qc.invalidateQueries({ queryKey: ['approvals'] });
      if (status === 'approved') {
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
        toast.success(t('orders.approved_done'), { description: o?.order_no });
      }
    },
    onError: (e) => toast.apiError(e),
  });
  const o = q.data;
  const actions = (o?.available_transitions ?? []).filter((s) => SIMPLE_TRANSITIONS.includes(s));
  const supplementable = canStartSupplement(o, { pathname, canOrder });
  const addSupplement = () => {
    const parent = supplementParentOf(o);
    if (!parent) return;
    const go = () => {
      startSupplement(parent);
      router.push('/bayi/menu/orders/new' as never);
    };
    // The supplement starts with an empty cart: products already in it are dropped.
    if (cartLines) {
      confirmAction({
        title: t('orders:supplement.replace_cart_title'),
        body: t('orders:supplement.replace_cart_body', { n: cartLines }),
        action: t('orders:supplement.add'),
        destructive: true,
        onConfirm: go,
      });
    } else go();
  };

  // TEC-1062/1063: the seller receives for a pool recipient without members.
  const onBehalf = (status: OrderStatus) => status === 'received' && !!o?.receive_on_behalf;
  const actionLabel = (status: OrderStatus) => t(onBehalf(status) ? 'orders.action.received_on_behalf' : `orders.action.${status}`);

  const storeOrder = (data: NonNullable<typeof o>) => {
    qc.setQueryData(['orders', 'detail', uuid], data);
    void qc.invalidateQueries({ queryKey: ['orders', 'list'] });
  };

  const confirm = (status: OrderStatus) =>
    // TEC-1227: the buyer receives through the check sheet (missing / damaged units).
    status === 'received' && !onBehalf(status) && o?.role === 'buyer'
      ? setChecking(true)
      : Alert.alert(t(onBehalf(status) ? 'orders.confirm.received_on_behalf' : `orders.confirm.${status}`), o?.order_no, [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: actionLabel(status),
        style: status === 'cancelled' ? 'destructive' : 'default',
        onPress: () => move.mutate(status),
      },
    ]);

  return (
    <>
      <Stack.Screen options={{ title: o?.order_no ?? '', headerLargeTitle: false }} />
      <Screen refreshControl={<PullRefresh busy={q.isRefetching} onRefresh={() => void q.refetch()} />}>
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
              {prices && hasAmount(o.total) ? (
                <View className="gap-0.5">
                  <Text variant="title" className="tabular-nums">
                    {f.money(o.total, o.currency)}
                  </Text>
                  {liveTryAmount(o, local) && local ? (
                    <Text variant="footnote" tone="muted" className="tabular-nums">
                      {t('orders:card.try_approx', { amount: f.money(liveTryAmount(o, local), local) })}
                    </Text>
                  ) : null}
                </View>
              ) : null}
            </View>

            {supplementable ? (
              <Button title={t('orders:supplement.add')} icon="add" variant="outline" onPress={addSupplement} />
            ) : null}

            {actions.length ? (
              <View className="gap-2">
                {actions.map((s) => (
                  <Button
                    key={s}
                    title={actionLabel(s)}
                    variant={s === 'cancelled' ? 'outline' : 'primary'}
                    loading={move.isPending && move.variables === s}
                    disabled={move.isPending}
                    onPress={() => confirm(s)}
                  />
                ))}
              </View>
            ) : null}

            <BackorderRows order={o} />
            <DiscrepancySection order={o} onUpdated={storeOrder} />

            <List>
              {o.parent_order ? (
                <RefRow
                  kind="order"
                  label={t('orders:supplement.parent')}
                  value={{ uuid: o.parent_order.uuid, name: o.parent_order.order_no, sub: o.parent_order.status_label }}
                />
              ) : null}
              <RefRow kind="organization" label={t('orders.seller')} value={o.seller} />
              <RefRow kind="organization" label={t('orders.buyer')} value={o.buyer} />
              {o.seller_warehouse ? <InfoRow label={t('orders:seller_warehouse')} value={o.seller_warehouse.name} /> : null}
              <RefRow kind="user" label={t('orders:detail.created_by')} value={o.created_by ?? creatorOf(o.timeline)} />
              {amountText(o) ? <InfoRow label={t('orders:detail.amount')} value={amountText(o)!} /> : null}
              <InfoRow label={t('orders.created')} value={f.dateTime(o.created_at)} />
              {o.delivery_method ? <InfoRow label={t('orders.delivery')} value={o.delivery_method.name} /> : null}
              {o.tracking_no ? <InfoRow label={t('orders.tracking')} value={o.tracking_no} last={!prices} /> : null}
              {prices ? (
                <>
                  <InfoRow label={t('orders.subtotal')} value={f.money(o.subtotal, o.currency)} />
                  <InfoRow label={t('orders.tax')} value={f.money(o.tax_total, o.currency)} last />
                </>
              ) : null}
            </List>

            {o.supplements?.length ? (
              <>
                <SectionHeader title={t('orders:supplement.list')} />
                <List>
                  {o.supplements.map((sup, i, arr) => (
                    <RefRow
                      key={sup.uuid}
                      kind="order"
                      label={sup.status_label}
                      value={{ uuid: sup.uuid, name: sup.order_no }}
                      last={i === arr.length - 1}
                    />
                  ))}
                </List>
              </>
            ) : null}

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
            {/* TEC-1057: before the order is ready (or the buyer's draft) the lines can be fixed. */}
            {o.can_edit_items ? (
              <Button title={t('orders:edit.button')} icon="edit" variant="outline" onPress={() => setEditing(true)} />
            ) : null}
            {o.items?.length ? (
              <List>
                {o.items.map((it, i, arr) => (
                  <View key={it.uuid} className={i < arr.length - 1 ? 'gap-0.5 border-b border-border px-4 py-3' : 'gap-0.5 px-4 py-3'}>
                    <RefText kind="product" value={it.product} numberOfLines={2} />
                    {prices && it.price_missing ? (
                      <View className="self-start">
                        <StatusPill tone="problem" label={t('orders.price_missing')} />
                      </View>
                    ) : null}
                    {showsMeasure(it, arr) ? <MeasureBadge measure={it.sale_measure} /> : null}
                    <View className="flex-row justify-between gap-3">
                      <Text variant="footnote" tone="muted" className="flex-1">
                        {it.product.sku} ·{' '}
                        {it.meters !== null ? t('orders.meters', { value: f.number(it.meters, 2) }) : t('orders.pieces', { count: it.quantity ?? 0 })}
                        {/* TEC-1240 (K3): the usage credits each cut piece of a meter line carries. */}
                        {it.line_credits != null ? ` · ${t('orders:measure.credits_value', { value: it.line_credits })}` : ''}
                      </Text>
                      {prices && hasAmount(it.line_total) ? (
                        <Text variant="footnote" className="font-semibold tabular-nums">
                          {f.money(it.line_total, o.currency)}
                        </Text>
                      ) : null}
                    </View>
                    {it.units?.length ? (
                      <View className="flex-row flex-wrap gap-x-3 gap-y-0.5">
                        {it.units.map((u) => (
                          <View key={u.unit_uuid} className="flex-row items-center gap-1">
                            <RefText kind="stock_unit" value={{ uuid: u.unit_uuid, name: u.barcode }} id={u.barcode} variant="footnote" mono />
                            {/* TEC-1240: the meters of each piece that fulfils a meter line. */}
                            {it.sale_measure === 'meter' && u.meters != null ? (
                              <Text variant="footnote" tone="muted" className="tabular-nums">
                                {t('orders.meters', { value: f.number(u.meters, 2) })}
                              </Text>
                            ) : null}
                          </View>
                        ))}
                      </View>
                    ) : null}
                    <LinePlotter order={o} item={it} credits={it.line_credits == null} />
                  </View>
                ))}
              </List>
            ) : (
              <Text variant="callout" tone="muted" className="px-4">
                {t('warehouse:orders.detail.items_empty')}
              </Text>
            )}

            {/* TEC-1220 (web TEC-1085): the returns linked to this order and "Create return". */}
            <OrderReturnsSection order={o} />

            {/* TEC-1059 (revize 5.4.1): who did what and when; the status history stays for orders without one. */}
            {o.timeline?.length ? (
              <OrderTimeline timeline={o.timeline} />
            ) : o.history?.length ? (
              <>
                <SectionHeader title={t('orders.history')} />
                <List>
                  {o.history.map((h, i, arr) => (
                    <InfoRow
                      key={`${h.created_at}-${i}`}
                      label={
                        h.on_behalf_of_buyer
                          ? `${t(`orders.status.${h.to_status}`)} · ${t('orders.on_behalf_note')}`
                          : t(`orders.status.${h.to_status}`)
                      }
                      value={f.dateTime(h.created_at)}
                      last={i === arr.length - 1}
                    />
                  ))}
                </List>
              </>
            ) : null}
            <OrderLinesEditor order={o} visible={editing} onClose={() => setEditing(false)} />
            {checking ? <ReceiptCheckSheet order={o} onClose={() => setChecking(false)} onDone={storeOrder} /> : null}
          </>
        )}
      </Screen>
    </>
  );
}
