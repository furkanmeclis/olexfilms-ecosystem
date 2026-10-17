import { useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, Pressable, RefreshControl, ScrollView, Switch, TextInput, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { resolveScan } from './api';
import { ContinuousScanner, type ScanOutcome } from './continuous-scanner';
import { confirmAction, formatDate, formatNumber, isCode, whError } from './format';
import {
  type Order,
  orderApi,
  type OrderItem,
  orderKeys,
  type OrderStatus,
  useDeliveryMethods,
  useOrder,
  usePickList,
  usePickLocations,
  useSuggestions,
  useUnplaced,
} from './orders-api';
import { Card, ErrorState, Field, IconButton, ListSkeleton, Segmented } from './parts';
import { QtyStepper } from './qty-stepper';
import { useDocAction } from './use-action';
import { createRetryKey } from './uuid';

export function orderTone(s: OrderStatus): StatusTone {
  if (s === 'received' || s === 'delivered') return 'done';
  if (s === 'cancelled' || s === 'cancelling') return 'problem';
  if (s === 'draft' || s === 'submitted') return 'neutral';
  return 'progress';
}

const isRoll = (i: OrderItem) => i.product.unit_type === 'roll_meter';
const required = (i: OrderItem) => Number(isRoll(i) ? (i.meters ?? 0) : (i.quantity ?? 0));
const covered = (i: OrderItem) => Number(i.assigned) >= required(i) - 1e-9;
const amount = (i: OrderItem, v: number | string | null) =>
  isRoll(i) ? `${formatNumber(v, 2)} m` : formatNumber(v);

/**
 * Order preparation in the warehouse. Seller: start preparing, assign units
 * by scan or suggestion, pick list and locations, mark ready, ship with a
 * delivery method. Buyer: receive, then shelve the received units.
 */
export function OrderScreen() {
  const { t } = useTranslation('warehouse');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const qc = useQueryClient();
  const canShip = useCan('orders.ship');
  const canReceive = useCan('orders.receive');
  const canPlace = useCan('warehouse.write');
  const q = useOrder(uuid);
  const o = q.data;
  const { busy, run } = useDocAction<Order>(orderKeys.order(uuid));
  const [view, setView] = useState<'lines' | 'pick'>('lines');
  const [scanning, setScanning] = useState<'assign' | 'place' | null>(null);
  const [qty, setQty] = useState(1);
  const [meters, setMeters] = useState('');
  const [suggestFor, setSuggestFor] = useState<OrderItem | null>(null);
  const [locationsFor, setLocationsFor] = useState<string | null>(null);
  const [shipping, setShipping] = useState(false);
  const [target, setTarget] = useState<{ uuid: string; code: string } | null>(null);
  const [splitKey] = useState(createRetryKey);

  const seller = o?.role === 'seller';
  const preparing = o?.status === 'preparing';
  const received = o?.status === 'received' && o.role === 'buyer';
  const pick = usePickList(uuid, !!o && seller && view === 'pick');
  const locations = usePickLocations(uuid, locationsFor);
  const unplaced = useUnplaced(uuid, !!received);

  const store = (next: Order) => qc.setQueryData(orderKeys.order(uuid), next);
  const can = (s: OrderStatus) => !!o?.available_transitions.includes(s);

  const transition = (status: OrderStatus, key: string, extra?: { delivery_method_uuid?: string; tracking_no?: string }) =>
    confirmAction({
      title: t(`orders.actions.${key}.title`),
      body: t(`orders.actions.${key}.description`),
      action: t(`orders.actions.${key}.confirm`),
      onConfirm: () =>
        void run(key, () => orderApi.transition(uuid, { status, ...extra }), {
          done: () => {
            setShipping(false);
            void qc.invalidateQueries({ queryKey: ['warehouse', 'orders'] });
          },
        }),
    });

  /** Scan a unit: it goes to the first open line of its product. */
  const onAssign = async (code: string): Promise<ScanOutcome> => {
    if (!o?.items) return { ok: false, title: t('scan.failed') };
    try {
      const r = await resolveScan(code);
      if (r.status !== 'ok' || r.type !== 'unit' || !r.unit) return { ok: false, title: t('m.scan_unit_not_product') };
      const lines = o.items.filter((i) => i.product.uuid === r.product?.uuid);
      if (!lines.length) return { ok: false, title: t('m.not_on_order', { barcode: r.unit.barcode }) };
      const item = lines.find((i) => !covered(i)) ?? lines[0];
      const body: { barcode: string; quantity?: number; meters?: string; idempotency_key?: string } = {
        barcode: r.unit.barcode,
      };
      if (r.unit.unit_kind === 'fixed') body.quantity = qty;
      if (isRoll(item) && meters.trim()) {
        body.meters = meters.trim().replace(',', '.');
        body.idempotency_key = splitKey.for(`${item.uuid}:${r.unit.barcode}:${body.meters}`);
      }
      store(await orderApi.assign(uuid, item.uuid, body));
      splitKey.done();
      if (body.quantity) setQty(1);
      if (body.meters) setMeters('');
      return {
        ok: true,
        title: r.product?.name ?? r.unit.barcode,
        detail: body.meters ? `${body.meters} m` : body.quantity ? `× ${body.quantity}` : r.unit.location?.full_code,
      };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  /** Buyer: a location QR sets the shelf; each unit scanned goes onto it. */
  const onPlace = async (code: string): Promise<ScanOutcome> => {
    try {
      const r = await resolveScan(code).catch((e: unknown) => {
        if (isCode(e, 'SCAN_NO_MATCH')) return null;
        throw e;
      });
      if (r?.type === 'location' && r.location) {
        setTarget({ uuid: r.location.uuid, code: r.location.full_code });
        return { ok: true, title: t('m.target_set', { location: r.location.full_code }) };
      }
      if (!target) return { ok: false, title: t('order_place.scan_location_first') };
      const barcode = r?.unit?.barcode ?? code;
      const res = await orderApi.place(uuid, { barcodes: [barcode], location_uuid: target.uuid });
      void unplaced.refetch();
      const skipped = res.units.find((u) => u.skipped);
      if (!res.units.length || (skipped && res.units.length === 1)) {
        return {
          ok: false,
          title:
            skipped?.skipped === 'fixed'
              ? t('order_place.fixed_scanned', { barcode })
              : t('order_place.already_placed', { barcode }),
        };
      }
      return { ok: true, title: r?.product?.name ?? barcode, detail: `→ ${target.code}` };
    } catch (e) {
      return { ok: false, title: whError(e) };
    }
  };

  if (!o) {
    return (
      <>
        <Stack.Screen options={{ title: t('orders.detail.title'), headerLargeTitle: false }} />
        <Screen>{q.isLoading ? <ListSkeleton rows={4} /> : <ErrorState error={q.error} onRetry={() => void q.refetch()} />}</Screen>
      </>
    );
  }

  const items = o.items ?? [];
  const allCovered = items.length > 0 && items.every(covered);
  const other = seller ? o.buyer : o.seller;

  return (
    <>
      <Stack.Screen options={{ title: o.order_no, headerLargeTitle: false }} />
      <Screen
        refreshControl={
          <RefreshControl
            refreshing={q.isRefetching}
            onRefresh={() => {
              void q.refetch();
              if (view === 'pick') void pick.refetch();
              if (received) void unplaced.refetch();
            }}
          />
        }>
        <Card>
          <View className="flex-row items-center gap-2 pb-1">
            <Icon name="package" tone="primary" size={18} />
            <Text variant="headline" className="flex-1" numberOfLines={2}>
              {other.name}
            </Text>
            <StatusPill tone={orderTone(o.status)} label={t(`orders.status.${o.status}`)} />
          </View>
          <Field label={t('orders.detail.role')} value={t(`orders.role.${o.role}`)} />
          <Field label={t('orders.shipping.method')} value={o.delivery_method?.name} />
          <Field label={t('orders.shipping.tracking_no')} value={o.tracking_no} mono />
          <Field label={t('fields.note')} value={o.note} />
          <Field label={t('orders.detail.created_at')} value={formatDate(o.created_at)} />
          <Field label={t('orders.detail.approved_at')} value={formatDate(o.approved_at)} />
          <Field label={t('orders.detail.shipped_at')} value={formatDate(o.shipped_at)} />
          <Field label={t('m.received')} value={formatDate(o.received_at)} />
        </Card>

        {/* Seller actions */}
        {seller && canShip ? (
          <View className="gap-2">
            {can('preparing') ? (
              <Button
                size="lg"
                title={t('orders.actions.start_preparing.button')}
                loading={busy === 'start_preparing'}
                // Nothing to pick on an order without items.
                disabled={items.length === 0}
                onPress={() => transition('preparing', 'start_preparing')}
              />
            ) : null}
            {preparing ? (
              <>
                <Button size="lg" title={t('m.scan_to_assign')} icon={<Icon name="scan" tone="primary-foreground" />} onPress={() => setScanning('assign')} />
                <Text variant="footnote" tone="muted">
                  {allCovered ? t('orders.assign.complete') : t('orders.assign.not_complete')}
                </Text>
              </>
            ) : null}
            {can('ready') ? (
              <Button
                variant="secondary"
                title={t('orders.actions.mark_ready.button')}
                disabled={!allCovered}
                loading={busy === 'mark_ready'}
                onPress={() => transition('ready', 'mark_ready')}
              />
            ) : null}
            {can('shipped') ? (
              <Button size="lg" title={t('orders.actions.ship.button')} icon={<Icon name="truck" tone="primary-foreground" />} onPress={() => setShipping(true)} />
            ) : null}
          </View>
        ) : null}

        {/* Buyer actions */}
        {!seller && canReceive && can('received') ? (
          <Button size="lg" title={t('orders.actions.receive.button')} loading={busy === 'receive'} onPress={() => transition('received', 'receive')} />
        ) : null}
        {received && canPlace ? (
          <Card>
            <Text variant="headline">{t('order_place.title')}</Text>
            {unplaced.isLoading ? (
              <ListSkeleton rows={1} />
            ) : unplaced.data ? (
              <>
                <Text variant="callout" tone="muted">
                  {unplaced.data.unplaced_count
                    ? t('order_place.unplaced_badge', { count: unplaced.data.unplaced_count })
                    : unplaced.data.units.length
                      ? t('order_place.all_placed', { count: unplaced.data.placed_count })
                      : t('order_place.nothing_to_place')}
                </Text>
                {unplaced.data.unplaced_count ? (
                  <Button
                    className="mt-2"
                    title={t('order_place.scan_location')}
                    icon={<Icon name="location" tone="primary-foreground" />}
                    onPress={() => setScanning('place')}
                  />
                ) : null}
                {unplaced.data.fixed.length ? (
                  <Text variant="footnote" tone="muted" className="pt-1">
                    {t('order_place.fixed_hint')}
                  </Text>
                ) : null}
              </>
            ) : (
              <ErrorState error={unplaced.error} onRetry={() => void unplaced.refetch()} />
            )}
          </Card>
        ) : null}

        {seller ? (
          <Segmented
            value={view}
            onChange={setView}
            options={[
              { value: 'lines', label: t('orders.picking.lines') },
              { value: 'pick', label: t('orders.picking.pick_list') },
            ]}
          />
        ) : (
          <SectionHeader title={t('orders.detail.items', { count: items.length })} />
        )}

        {view === 'pick' && seller ? (
          pick.isLoading ? (
            <ListSkeleton rows={4} />
          ) : !pick.data ? (
            <ErrorState error={pick.error} onRetry={() => void pick.refetch()} />
          ) : pick.data.rows.length === 0 ? (
            <Text variant="callout" tone="muted" className="px-4">
              {t('orders.picking.empty')}
            </Text>
          ) : (
            <View className="gap-2">
              {pick.data.rows.map((r, i) => (
                <View key={`${r.item_uuid}-${i}`} className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-2.5">
                  <View className="w-20">
                    <Text variant="callout" className="font-mono font-semibold" numberOfLines={1}>
                      {r.location_code ?? t('orders.picking.no_location')}
                    </Text>
                  </View>
                  <View className="flex-1 gap-0.5">
                    <Text variant="callout" numberOfLines={1}>
                      {r.product.name}
                    </Text>
                    <Text variant="footnote" tone="muted" numberOfLines={1}>
                      {r.kind === 'stock' ? t('orders.picking.stock_row') : (r.barcode ?? '')}
                    </Text>
                  </View>
                  <Text variant="headline">{r.meters ? `${formatNumber(r.meters, 2)} m` : formatNumber(r.quantity)}</Text>
                </View>
              ))}
            </View>
          )
        ) : items.length === 0 ? (
          <Text variant="callout" tone="muted" className="px-4">
            {t('orders.detail.items_empty')}
          </Text>
        ) : (
          <View className="gap-2">
            {items.map((i) => (
              <View key={i.uuid} className="gap-2 rounded-lg border border-border bg-card px-4 py-3">
                <View className="flex-row items-start gap-2">
                  <View className="flex-1 gap-0.5">
                    <Text variant="callout" className="font-semibold" numberOfLines={2}>
                      {i.product.name}
                    </Text>
                    <Text variant="footnote" tone="muted" className="font-mono">
                      {i.product.sku}
                    </Text>
                  </View>
                  <View className="items-end">
                    <Text variant="headline" className={covered(i) ? 'text-success' : ''}>
                      {amount(i, i.assigned)} / {amount(i, required(i))}
                    </Text>
                    <Text variant="footnote" tone="muted">
                      {t('orders.assign.assigned')}
                    </Text>
                  </View>
                </View>

                {i.units.map((u) => (
                  <View key={u.unit_uuid} className="flex-row items-center gap-2 border-t border-border pt-1.5">
                    <Text variant="footnote" className="flex-1 font-mono" numberOfLines={1}>
                      {u.barcode}
                      {u.meters ? ` · ${formatNumber(u.meters, 2)} m` : u.unit_kind === 'fixed' && u.quantity ? ` × ${u.quantity}` : ''}
                    </Text>
                    {preparing && canShip && !u.shipped ? (
                      <IconButton
                        icon="delete"
                        tone="destructive"
                        label={t('orders.assign.remove')}
                        busy={busy === `del:${u.unit_uuid}`}
                        onPress={() => void run(`del:${u.unit_uuid}`, () => orderApi.unassign(uuid, i.uuid, u.unit_uuid))}
                      />
                    ) : null}
                  </View>
                ))}

                {seller && canShip && (preparing || o.status === 'approved') ? (
                  <View className="flex-row gap-2">
                    {preparing && !covered(i) ? (
                      <Button className="flex-1" variant="secondary" title={t('orders.suggest.button')} onPress={() => setSuggestFor(i)} />
                    ) : null}
                    <Button
                      className="flex-1"
                      variant="outline"
                      title={t('orders.picking.locations')}
                      onPress={() => setLocationsFor(locationsFor === i.uuid ? null : i.uuid)}
                    />
                  </View>
                ) : null}

                {locationsFor === i.uuid ? (
                  locations.isLoading ? (
                    <ListSkeleton rows={1} />
                  ) : (locations.data?.length ?? 0) === 0 ? (
                    <Text variant="footnote" tone="muted">
                      {t('orders.picking.locations_empty')}
                    </Text>
                  ) : (
                    <View className="gap-1">
                      {locations.data?.map((l, k) => (
                        <View key={`${l.location_uuid}-${k}`} className="flex-row justify-between">
                          <Text variant="footnote" className="font-mono">
                            {l.location_code ?? t('orders.picking.no_location')}
                          </Text>
                          <Text variant="footnote">{l.meters ? `${formatNumber(l.meters, 2)} m` : formatNumber(l.quantity)}</Text>
                        </View>
                      ))}
                    </View>
                  )
                ) : null}
              </View>
            ))}
          </View>
        )}
      </Screen>

      <ContinuousScanner
        visible={scanning === 'assign'}
        onClose={() => setScanning(null)}
        title={t('m.scan_to_assign')}
        hint={t('orders.assign.hint')}
        onCode={onAssign}
        footer={
          <>
            <QtyStepper label={t('m.fixed_qty')} value={qty} onChange={setQty} />
            {items.some(isRoll) ? (
              <View className="flex-row items-center gap-2 rounded-lg bg-card p-2">
                <Text variant="footnote" tone="muted" className="flex-1 ps-2 font-medium">
                  {t('orders.assign.meters')}
                </Text>
                <TextInput
                  value={meters}
                  onChangeText={setMeters}
                  keyboardType="decimal-pad"
                  placeholder={t('orders.assign.meters_placeholder')}
                  placeholderTextColorClassName="accent-muted-foreground"
                  className="min-h-11 w-28 rounded-md border border-border px-2 text-center text-[17px] text-foreground"
                />
              </View>
            ) : null}
          </>
        }
      />
      <ContinuousScanner
        visible={scanning === 'place'}
        onClose={() => {
          setScanning(null);
          void unplaced.refetch();
        }}
        title={t('order_place.title')}
        hint={t('order_place.scan_location_hint')}
        onCode={onPlace}
        footer={
          <View className="gap-2 rounded-lg bg-card p-3">
            <Text variant="callout" className="font-semibold">
              {target ? t('m.target_bin', { location: target.code }) : t('order_place.no_target')}
            </Text>
            {target && unplaced.data?.unplaced_count ? (
              <Button
                variant="secondary"
                title={t('m.place_rest', { count: unplaced.data.unplaced_count })}
                loading={busy === 'place_all'}
                onPress={() =>
                  void run('place_all', async () => {
                    await orderApi.place(uuid, { location_uuid: target.uuid });
                    await unplaced.refetch();
                  })
                }
              />
            ) : null}
          </View>
        }
      />
      {suggestFor ? (
        <SuggestSheet
          order={uuid}
          item={suggestFor}
          onClose={() => setSuggestFor(null)}
          onApplied={(next) => {
            store(next);
            setSuggestFor(null);
          }}
        />
      ) : null}
      <ShipSheet
        visible={shipping}
        busy={busy === 'ship'}
        onClose={() => setShipping(false)}
        onShip={(extra) => transition('shipped', 'ship', extra)}
      />
    </>
  );
}

function SuggestSheet({
  order,
  item,
  onClose,
  onApplied,
}: {
  order: string;
  item: OrderItem;
  onClose: () => void;
  onApplied: (o: Order) => void;
}) {
  const { t } = useTranslation('warehouse');
  const [unlocated, setUnlocated] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const q = useSuggestions(order, item.uuid, unlocated);
  const s = q.data;

  const apply = async () => {
    if (!s?.suggestions.length) return;
    setBusy(true);
    setError(null);
    try {
      onApplied(
        await orderApi.applySuggestions(order, item.uuid, {
          units: s.suggestions.map((x) => ({
            unit_uuid: x.unit_uuid,
            quantity: x.unit_kind === 'fixed' && x.quantity ? x.quantity : undefined,
            meters: x.meters ?? undefined,
          })),
        }),
      );
    } catch (e) {
      setError(whError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <View className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={1}>
            {t('orders.suggest.title')}
          </Text>
          <Button title={t('common:close')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-3 px-4 pb-10">
          <Text variant="headline">{item.product.name}</Text>
          <Text variant="footnote" tone="muted">
            {t('orders.suggest.rule')}
          </Text>
          <View className="flex-row items-center justify-between gap-3">
            <Text variant="callout" className="flex-1">
              {t('orders.suggest.include_unlocated')}
            </Text>
            <Switch value={unlocated} onValueChange={setUnlocated} />
          </View>
          {q.isLoading ? (
            <ListSkeleton rows={3} />
          ) : !s ? (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          ) : s.suggestions.length === 0 ? (
            <Text variant="callout" tone="muted">
              {t('orders.suggest.empty')}
            </Text>
          ) : (
            <>
              <Text variant="callout" className={s.complete ? '' : 'text-destructive'}>
                {t('orders.suggest.coverage', {
                  covered: s.covered,
                  missing: s.missing,
                  unit: isRoll(item) ? t('orders.unit.m') : t('orders.unit.pcs'),
                })}
                {s.complete ? '' : ` · ${t('orders.suggest.incomplete')}`}
              </Text>
              <View className="gap-2">
                {s.suggestions.map((x) => (
                  <View key={x.unit_uuid} className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-2.5">
                    <Text variant="callout" className="w-20 font-mono font-semibold" numberOfLines={1}>
                      {x.location_code ?? t('orders.picking.no_location')}
                    </Text>
                    <Text variant="footnote" className="flex-1 font-mono" numberOfLines={1}>
                      {x.barcode}
                    </Text>
                    <Text variant="callout">{x.meters ? `${formatNumber(x.meters, 2)} m` : `× ${formatNumber(x.quantity)}`}</Text>
                  </View>
                ))}
              </View>
              {error ? (
                <Text variant="footnote" tone="danger">
                  {error}
                </Text>
              ) : null}
              <Button size="lg" title={t('orders.suggest.apply')} loading={busy} onPress={() => void apply()} />
            </>
          )}
        </ScrollView>
      </View>
    </Modal>
  );
}

function ShipSheet({
  visible,
  busy,
  onClose,
  onShip,
}: {
  visible: boolean;
  busy: boolean;
  onClose: () => void;
  onShip: (extra: { delivery_method_uuid?: string; tracking_no?: string }) => void;
}) {
  const { t } = useTranslation('warehouse');
  const methods = useDeliveryMethods(visible);
  const [method, setMethod] = useState<string | undefined>(undefined);
  const [tracking, setTracking] = useState('');
  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <View className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title">{t('orders.shipping.title')}</Text>
          <Button title={t('common:close')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-4 px-4 pb-10" keyboardShouldPersistTaps="handled">
          <Text variant="callout" tone="muted">
            {t('orders.actions.ship.description')}
          </Text>
          {(methods.data?.length ?? 0) > 0 ? (
            <View className="gap-2">
              <Text variant="footnote" tone="muted" className="font-medium">
                {t('orders.shipping.method')}
              </Text>
              {methods.data?.map((m) => (
                <Pressable
                  key={m.uuid}
                  accessibilityRole="radio"
                  accessibilityState={{ checked: method === m.uuid }}
                  onPress={() => setMethod(method === m.uuid ? undefined : m.uuid)}
                  className={`min-h-12 flex-row items-center rounded-lg border px-4 ${method === m.uuid ? 'border-primary bg-secondary' : 'border-border bg-card'}`}>
                  <Text variant="body" className="flex-1">
                    {m.name}
                  </Text>
                  {method === m.uuid ? <Icon name="truck" tone="primary" size={18} /> : null}
                </Pressable>
              ))}
            </View>
          ) : null}
          <Input
            label={t('orders.shipping.tracking_no')}
            placeholder={t('orders.shipping.tracking_placeholder')}
            value={tracking}
            onChangeText={setTracking}
            autoCapitalize="characters"
            autoCorrect={false}
          />
          <Text variant="footnote" tone="muted">
            {t('m.shipping_note_web')}
          </Text>
          <Button
            size="lg"
            title={t('orders.actions.ship.button')}
            loading={busy}
            onPress={() => onShip({ delivery_method_uuid: method, tracking_no: tracking.trim() || undefined })}
          />
        </ScrollView>
      </View>
    </Modal>
  );
}
