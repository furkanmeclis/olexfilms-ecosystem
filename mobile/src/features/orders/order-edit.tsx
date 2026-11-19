import { type QueryClient, useMutation, useQueries, useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardAvoidingView, Modal, Platform, Pressable, ScrollView, Switch, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { useCan } from '@/core/spaces';
import { useDebounced, useFormat } from '@/features/admin/kit';
import { catalogKeys, useProducts } from '@/features/catalog/api';
import { PickerSheet } from '@/features/picker-sheet';
import { Button } from '@/ui/button';
import { confirmAction } from '@/ui/confirm';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { AmountStepper, LineCreditsField, MeasureBadge, OtherMeasureButton, WholeBoxHint } from './line-fields';
import {
  addEditLine,
  byMeter,
  type CartProduct,
  cartIsValid,
  convertToBoxes,
  editChanged,
  type EditLine,
  isAmountError,
  isCutLine,
  isRoll,
  lineKey,
  lineMeasure,
  linesBelowAssigned,
  linesFromItems,
  otherMeasure,
  plotterCreditsInvalid,
  productOf,
  type SaleMeasure,
  toItemInputs,
  validateCart,
  withProductInfo,
} from './order-lines';
import { canMarkPlotter } from './order-summary';

type Order = components['schemas']['Order'];

/**
 * The same order lives under two cache keys: the order detail of the menus
 * and the warehouse preparation screen. A write stores the returned order in
 * both and refreshes the lists.
 */
export function storeOrder(qc: QueryClient, order: Order) {
  qc.setQueryData(['orders', 'detail', order.uuid], order);
  qc.setQueryData(['warehouse', 'order', order.uuid], order);
  void qc.invalidateQueries({ queryKey: ['orders', 'list'] });
  void qc.invalidateQueries({ queryKey: ['warehouse', 'orders'] });
  // Pick list, locations and suggestions of the order follow its lines.
  void qc.invalidateQueries({ queryKey: ['warehouse', 'order', order.uuid], predicate: (q) => q.queryKey.length > 3 });
}

/** Whether the signed-in user may switch the plotter mark of this order's lines (TEC-1059). */
export function useCanMarkPlotter(order: Pick<Order, 'role' | 'status'> | undefined) {
  const write = useCan('orders.write');
  const ship = useCan('orders.ship');
  return !!order && canMarkPlotter(order, { write, ship });
}

/**
 * PATCH of one line's "sent plotter cut" mark (TEC-1059) and, on a plotter
 * cut line, its usage credits (TEC-1221; 0 clears); the returned order goes
 * to the cache.
 */
export function usePlotterMark(orderUuid: string) {
  const { t } = useTranslation('orders');
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { item: string; plotter_cut: boolean; plotter_credits?: number }) =>
      unwrap(
        api.PATCH('/v1/orders/{uuid}/items/{item_uuid}', {
          params: { path: { uuid: orderUuid, item_uuid: v.item } },
          body: { plotter_cut: v.plotter_cut, plotter_credits: v.plotter_credits },
        }),
      ),
    onSuccess: (order) => {
      void Haptics.selectionAsync().catch(() => undefined);
      storeOrder(qc, order);
    },
    onError: (e) => toast.apiError(e, { title: t('plotter.failed') }),
  });
}

/** "Sent plotter cut" switch row of one order line (information only). */
export function PlotterSwitch({
  value,
  onChange,
  disabled,
}: {
  value: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation('orders');
  return (
    <View className="min-h-11 flex-row items-center justify-between gap-3">
      <Text variant="callout" tone="muted" className="flex-1">
        {t('plotter.label')}
      </Text>
      <Switch value={value} onValueChange={onChange} disabled={disabled} accessibilityLabel={t('plotter.label')} />
    </View>
  );
}

/**
 * TEC-1221: usage credits of the barcodes shipped on a plotter cut line; empty
 * keeps the product default. Shown only under a plotter cut line.
 */
export function PlotterCreditsInput({
  value,
  onChange,
  onEndEditing,
  invalid,
  editable = true,
}: {
  value: string;
  onChange: (v: string) => void;
  onEndEditing?: () => void;
  invalid?: boolean;
  editable?: boolean;
}) {
  const { t } = useTranslation('orders');
  return (
    <View className="gap-1">
      <View className="flex-row items-center gap-3">
        <Text variant="callout" tone="muted" className="flex-1">
          {t('plotter.credits')}
        </Text>
        <TextInput
          value={value}
          onChangeText={onChange}
          onEndEditing={onEndEditing}
          editable={editable}
          accessibilityLabel={t('plotter.credits')}
          placeholder={t('plotter.credits_default')}
          placeholderTextColorClassName="accent-muted-foreground"
          keyboardType="number-pad"
          selectTextOnFocus
          className="min-h-11 w-28 rounded-md border border-border bg-background px-3 text-right text-[17px] text-foreground tabular-nums"
        />
      </View>
      <Text variant="footnote" tone={invalid ? 'danger' : 'muted'}>
        {invalid ? t('errors.credits_invalid') : t('plotter.credits_hint')}
      </Text>
    </View>
  );
}

/** Credits field of a plotter cut order line, saved on its own (PATCH) when editing ends. */
function LinePlotterCredits({
  item,
  mark,
}: {
  item: { uuid: string; plotter_credits?: number | null };
  mark: ReturnType<typeof usePlotterMark>;
}) {
  const saved = item.plotter_credits != null ? String(item.plotter_credits) : '';
  const [value, setValue] = useState(saved);
  const [prev, setPrev] = useState(saved);
  // A refetched order (or another device) moves the field along.
  if (prev !== saved) {
    setPrev(saved);
    setValue(saved);
  }
  const invalid = plotterCreditsInvalid(value);
  const save = () => {
    const v = value.trim();
    if (invalid || v === saved) return;
    mark.mutate({ item: item.uuid, plotter_cut: true, plotter_credits: v ? Number(v) : 0 });
  };
  return <PlotterCreditsInput value={value} onChange={setValue} onEndEditing={save} invalid={invalid} editable={!mark.isPending} />;
}

/**
 * Plotter mark of an order line: a switch (and the credits of a plotter cut
 * line, TEC-1221) for who may set it, else a small badge when marked.
 */
export function LinePlotter({
  order,
  item,
  credits = true,
}: {
  order: Pick<Order, 'uuid' | 'role' | 'status'>;
  item: { uuid: string; plotter_cut: boolean; plotter_credits?: number | null };
  /** TEC-1240 (K12): false on a meter line, whose plotter mark shares the line credits shown with the line. */
  credits?: boolean;
}) {
  const { t } = useTranslation('orders');
  const can = useCanMarkPlotter(order);
  const mark = usePlotterMark(order.uuid);
  if (can) {
    const pending = mark.isPending && mark.variables?.item === item.uuid;
    const on = pending ? !!mark.variables?.plotter_cut : item.plotter_cut;
    return (
      <>
        <PlotterSwitch value={on} disabled={mark.isPending} onChange={(v) => mark.mutate({ item: item.uuid, plotter_cut: v })} />
        {on && credits ? <LinePlotterCredits item={item} mark={mark} /> : null}
      </>
    );
  }
  if (!item.plotter_cut) return null;
  return (
    <View className="flex-row items-center gap-1.5 self-start rounded-md bg-secondary px-2 py-0.5">
      <Icon name="check" tone="primary" size={12} weight="semibold" />
      <Text variant="footnote">
        {credits && item.plotter_credits != null ? t('plotter.badge_credits', { value: item.plotter_credits }) : t('plotter.badge')}
      </Text>
    </View>
  );
}

/**
 * Product list of the editor: the brand catalog by name or SKU; a product sold
 * by the meter is listed twice, "Box" and "Meter" (one tap adds that line,
 * TEC-1240). Lines already on the order are disabled.
 */
function ProductPicker({
  picked,
  onPick,
  onClose,
}: {
  picked: Set<string>;
  onPick: (p: CartProduct, measure: SaleMeasure) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('catalog');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const res = useProducts('', query);
  const options = res.items.flatMap((p) => {
    const product: CartProduct = { uuid: p.uuid, sku: p.sku, name: p.name, unit_type: p.unit_type, roll: p.roll, usage_credits: p.usage_credits };
    const natural: SaleMeasure = isRoll(p.unit_type) ? 'meter' : 'box';
    const measures: SaleMeasure[] = p.roll?.sellable_by_meter && !isRoll(p.unit_type) ? ['box', 'meter'] : [natural];
    return measures.map((m) => ({
      key: `${p.uuid}:${m}`,
      title: p.name,
      subtitle: measures.length > 1 ? `${p.sku} · ${t(`orders:measure.${m}`)}` : p.sku,
      disabled: picked.has(`${p.uuid}:${m}`),
      product,
      measure: m,
    }));
  });
  return (
    <PickerSheet
      visible
      title={t('orders:add_products')}
      options={options}
      search={{ value: q, onChange: setQ }}
      searchPlaceholder={t('search')}
      loading={res.isPending}
      error={res.error}
      onRetry={() => void res.refetch()}
      fetchingMore={res.isFetchingNextPage}
      onEndReached={() => {
        if (res.hasNextPage && !res.isFetchingNextPage) void res.fetchNextPage();
      }}
      emptyText={t('no_results')}
      onPick={(o) => onPick(o.product, o.measure)}
      onClose={onClose}
    />
  );
}

/**
 * TEC-1240: catalog settings (box length, minimum cut, meter switch, usage
 * credits) of the order's non-roll products: an order line carries only the
 * product ref. Read from the catalog cache; a failed read leaves the line as
 * it is (the server still checks the minimum cut).
 */
function useLineProducts(uuids: string[]) {
  const results = useQueries({
    queries: uuids.map((uuid) => ({
      queryKey: catalogKeys.product(uuid),
      queryFn: () => unwrap(api.GET('/v1/catalog/products/{uuid}', { params: { path: { uuid } } })),
      staleTime: 5 * 60_000,
    })),
  });
  const map = new Map<string, CartProduct>();
  for (const r of results) if (r.data) map.set(r.data.uuid, r.data);
  return map;
}

/**
 * TEC-1057: the seller (sales or warehouse staff) fixes the lines of an order
 * before it is ready, the buyer its draft: add a product from the catalog,
 * change an amount, remove a line. The request is the full line set (PUT
 * items); a line may not go below what is already assigned. No price is
 * shown or typed: a new line is priced by the server.
 */
export function OrderLinesEditor({ order, visible, onClose }: { order: Order; visible: boolean; onClose: () => void }) {
  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      {/* Mounted only while open: every edit starts from the order as it is now. */}
      {visible ? <EditorBody order={order} onClose={onClose} /> : null}
    </Modal>
  );
}

function EditorBody({ order, onClose }: { order: Order; onClose: () => void }) {
  const { t } = useTranslation('orders');
  const f = useFormat();
  const qc = useQueryClient();
  const insets = useSafeAreaInsets();
  // The seller sends the plotter marks back; on the buyer's draft the API keeps them.
  const seller = order.role === 'seller';
  const initial = useMemo(() => linesFromItems(order.items ?? [], seller), [order.items, seller]);
  const [raw, setLines] = useState<EditLine[]>(initial);
  const productUuids = useMemo(
    () => [...new Set(initial.filter((l) => !isRoll(l.unit_type)).map((l) => l.product_uuid))],
    [initial],
  );
  const products = useLineProducts(productUuids);
  // Lines of the order learn their product settings once read; added lines know them already.
  const lines = useMemo(() => withProductInfo(raw, products), [raw, products]);
  const [touched, setTouched] = useState(false);
  const [picking, setPicking] = useState(false);
  const errors = useMemo(() => validateCart(lines), [lines]);
  const below = useMemo(() => linesBelowAssigned(lines), [lines]);
  const changed = editChanged(initial, lines);
  const picked = useMemo(() => new Set(lines.map(lineKey)), [lines]);

  const save = useMutation({
    mutationFn: () =>
      unwrap(
        api.PUT('/v1/orders/{uuid}/items', { params: { path: { uuid: order.uuid } }, body: { items: toItemInputs(lines) } }),
      ),
    onSuccess: (updated) => {
      storeOrder(qc, updated);
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      onClose();
      toast.success(t('edit.saved'));
    },
    onError: (e) => {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      toast.apiError(e, { title: t('edit.failed') });
    },
  });

  const submit = () => {
    setTouched(true);
    if (save.isPending || !cartIsValid(errors) || below.size > 0) return;
    save.mutate();
  };

  const close = () => {
    if (!changed || save.isPending) {
      if (!save.isPending) onClose();
      return;
    }
    confirmAction({
      title: t('common:leave_title'),
      body: t('common:leave_body'),
      action: t('common:leave_discard'),
      destructive: true,
      onConfirm: onClose,
    });
  };

  const update = (key: string, patch: Partial<EditLine>) =>
    setLines(() => lines.map((l) => (lineKey(l) === key ? { ...l, ...patch } : l)));
  const drop = (key: string) => setLines(() => lines.filter((l) => lineKey(l) !== key));
  const remove = (l: EditLine) =>
    l.assigned > 0
      ? confirmAction({
          title: t('remove'),
          body: t('edit.remove_assigned', { name: l.name }),
          action: t('remove'),
          destructive: true,
          onConfirm: () => drop(lineKey(l)),
        })
      : drop(lineKey(l));

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      className="flex-1 bg-background"
      style={{ paddingTop: Platform.OS === 'ios' ? 0 : insets.top }}>
      <View className="flex-row items-center justify-between gap-2 pb-2 pe-2 ps-5 pt-4">
        <Text variant="title" numberOfLines={1} className="flex-1">
          {t('edit.title')}
        </Text>
        <Button title={t('common:close')} variant="ghost" onPress={close} />
      </View>
      <ScrollView
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        contentContainerClassName="gap-3 px-4"
        contentContainerStyle={{ paddingBottom: insets.bottom + 24 }}>
        <Text variant="footnote" tone="muted">
          {order.order_no} · {t('edit.hint')}
        </Text>
        {lines.length === 0 ? (
          <Text variant="callout" tone={touched ? 'danger' : 'muted'}>
            {t('edit.empty')}
          </Text>
        ) : null}
        {lines.map((l) => {
          const k = lineKey(l);
          const meter = byMeter(l);
          const cut = isCutLine(l);
          const err = errors.lines[k];
          const shown = touched || err === 'below_min_cut' ? err : undefined;
          const isBelow = below.has(k);
          const other = otherMeasure(lines, l);
          return (
            <View key={k} testID={`edit-line-${k}`} className="gap-2.5 rounded-lg border border-border bg-card p-3.5">
              <View className="flex-row items-start gap-3">
                <View className="flex-1 gap-1">
                  <Text variant="headline" numberOfLines={2}>
                    {l.name}
                  </Text>
                  <Text variant="footnote" tone="muted" className="font-mono">
                    {l.sku}
                  </Text>
                  {l.sellable_by_meter || cut ? <MeasureBadge measure={lineMeasure(l)} /> : null}
                  {l.assigned > 0 ? (
                    <Text variant="footnote" tone="muted">
                      {t('edit.assigned', { amount: meter ? `${f.number(l.assigned, 2)} m` : f.number(l.assigned) })}
                    </Text>
                  ) : null}
                </View>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('remove')}
                  hitSlop={8}
                  onPress={() => remove(l)}
                  className="size-11 items-center justify-center rounded-md active:bg-muted">
                  <Icon name="delete" tone="muted-foreground" size={20} />
                </Pressable>
              </View>
              <AmountStepper
                line={l}
                onChange={(v) => update(k, { amount: v })}
                invalid={isAmountError(shown) || isBelow}
              />
              {/* A line with units assigned is not converted here: the warehouse unassigns first. */}
              <WholeBoxHint
                line={l}
                disabled={l.assigned > 0}
                onConvert={() => setLines(() => convertToBoxes(lines, k, { assigned: 0, ...(seller ? { plotter_cut: false } : {}) }))}
              />
              <LineCreditsField line={l} onChange={(v) => update(k, { line_credits: v })} invalid={shown === 'line_credits_invalid'} />
              {seller ? <PlotterSwitch value={l.plotter_cut === true} onChange={(v) => update(k, { plotter_cut: v })} /> : null}
              {seller && l.plotter_cut && !cut ? (
                <PlotterCreditsInput
                  value={l.plotter_credits ?? ''}
                  onChange={(v) => update(k, { plotter_credits: v })}
                  invalid={shown === 'credits_invalid'}
                />
              ) : null}
              {shown && shown !== 'credits_invalid' && shown !== 'line_credits_invalid' ? (
                <Text variant="footnote" tone="danger">
                  {t(`errors.${shown}`, { value: l.min_cut ?? '' })}
                </Text>
              ) : isBelow ? (
                <Text variant="footnote" tone="danger">
                  {t('edit.below_assigned')}
                </Text>
              ) : null}
              {other ? (
                <OtherMeasureButton measure={other} onPress={() => setLines(() => addEditLine(lines, productOf(l), seller, other))} />
              ) : null}
            </View>
          );
        })}
        <Button title={t('add_products')} icon="add" variant="ghost" onPress={() => setPicking(true)} />
        {touched && errors.form === 'too_many' ? (
          <Text variant="footnote" tone="danger">
            {t('errors.too_many')}
          </Text>
        ) : null}
        <Button
          title={t('edit.save')}
          size="lg"
          loading={save.isPending}
          disabled={save.isPending || !changed}
          onPress={submit}
        />
      </ScrollView>
      {picking ? (
        <ProductPicker
          picked={picked}
          onPick={(p, m) => setLines(() => addEditLine(lines, p, seller, m))}
          onClose={() => setPicking(false)}
        />
      ) : null}
    </KeyboardAvoidingView>
  );
}
