import { type QueryClient, useMutation, useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { KeyboardAvoidingView, Modal, Platform, Pressable, ScrollView, Switch, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { useCan } from '@/core/spaces';
import { useDebounced, useFormat } from '@/features/admin/kit';
import { useProducts } from '@/features/catalog/api';
import { PickerSheet } from '@/features/picker-sheet';
import { Button } from '@/ui/button';
import { confirmAction } from '@/ui/confirm';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  addEditLine,
  cartIsValid,
  editChanged,
  type EditLine,
  isRoll,
  linesBelowAssigned,
  linesFromItems,
  plotterCreditsInvalid,
  toItemInputs,
  validateCart,
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
}: {
  order: Pick<Order, 'uuid' | 'role' | 'status'>;
  item: { uuid: string; plotter_cut: boolean; plotter_credits?: number | null };
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
        {on ? <LinePlotterCredits item={item} mark={mark} /> : null}
      </>
    );
  }
  if (!item.plotter_cut) return null;
  return (
    <View className="flex-row items-center gap-1.5 self-start rounded-md bg-secondary px-2 py-0.5">
      <Icon name="check" tone="primary" size={12} weight="semibold" />
      <Text variant="footnote">
        {item.plotter_credits != null ? t('plotter.badge_credits', { value: item.plotter_credits }) : t('plotter.badge')}
      </Text>
    </View>
  );
}

/** Product list of the editor: the brand catalog by name or SKU; products already on the order are disabled. */
function ProductPicker({
  picked,
  onPick,
  onClose,
}: {
  picked: Set<string>;
  onPick: (p: { uuid: string; sku: string; name: string; unit_type: string }) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('catalog');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const res = useProducts('', query);
  return (
    <PickerSheet
      visible
      title={t('orders:add_products')}
      options={res.items.map((p) => ({
        key: p.uuid,
        title: p.name,
        subtitle: p.sku,
        disabled: picked.has(p.uuid),
        product: { uuid: p.uuid, sku: p.sku, name: p.name, unit_type: p.unit_type },
      }))}
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
      onPick={(o) => onPick(o.product)}
      onClose={onClose}
    />
  );
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
  const [lines, setLines] = useState<EditLine[]>(initial);
  const [touched, setTouched] = useState(false);
  const [picking, setPicking] = useState(false);
  const errors = useMemo(() => validateCart(lines), [lines]);
  const below = useMemo(() => linesBelowAssigned(lines), [lines]);
  const changed = editChanged(initial, lines);
  const picked = useMemo(() => new Set(lines.map((l) => l.product_uuid)), [lines]);

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

  const update = (product: string, patch: Partial<EditLine>) =>
    setLines((ls) => ls.map((l) => (l.product_uuid === product ? { ...l, ...patch } : l)));
  const drop = (product: string) => setLines((ls) => ls.filter((l) => l.product_uuid !== product));
  const remove = (l: EditLine) =>
    l.assigned > 0
      ? confirmAction({
          title: t('remove'),
          body: t('edit.remove_assigned', { name: l.name }),
          action: t('remove'),
          destructive: true,
          onConfirm: () => drop(l.product_uuid),
        })
      : drop(l.product_uuid);

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
          const roll = isRoll(l.unit_type);
          const err = touched ? errors.lines[l.product_uuid] : undefined;
          const isBelow = below.has(l.product_uuid);
          return (
            <View key={l.product_uuid} className="gap-2 rounded-lg border border-border bg-card p-3.5">
              <View className="flex-row items-start gap-3">
                <View className="flex-1 gap-0.5">
                  <Text variant="headline" numberOfLines={2}>
                    {l.name}
                  </Text>
                  <Text variant="footnote" tone="muted" className="font-mono">
                    {l.sku}
                  </Text>
                  {l.assigned > 0 ? (
                    <Text variant="footnote" tone="muted">
                      {t('edit.assigned', { amount: roll ? `${f.number(l.assigned, 2)} m` : f.number(l.assigned) })}
                    </Text>
                  ) : null}
                </View>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('remove')}
                  hitSlop={8}
                  onPress={() => remove(l)}
                  className="size-8 items-center justify-center rounded-md active:bg-muted">
                  <Icon name="delete" tone="muted-foreground" size={18} />
                </Pressable>
              </View>
              <View className="flex-row items-center gap-3">
                <Text variant="callout" tone="muted" className="flex-1">
                  {roll ? t('amount_meters') : t('amount_pieces')}
                </Text>
                <TextInput
                  value={l.amount}
                  onChangeText={(v) => update(l.product_uuid, { amount: v })}
                  accessibilityLabel={roll ? t('amount_meters') : t('amount_pieces')}
                  placeholder={roll ? t('placeholder_meters') : t('placeholder_pieces')}
                  placeholderTextColorClassName="accent-muted-foreground"
                  keyboardType={roll ? 'decimal-pad' : 'number-pad'}
                  selectTextOnFocus
                  className="min-h-11 w-28 rounded-md border border-border bg-background px-3 text-right text-[17px] text-foreground tabular-nums"
                />
              </View>
              {seller ? (
                <PlotterSwitch value={l.plotter_cut === true} onChange={(v) => update(l.product_uuid, { plotter_cut: v })} />
              ) : null}
              {seller && l.plotter_cut ? (
                <PlotterCreditsInput
                  value={l.plotter_credits ?? ''}
                  onChange={(v) => update(l.product_uuid, { plotter_credits: v })}
                  invalid={err === 'credits_invalid'}
                />
              ) : null}
              {err && err !== 'credits_invalid' ? (
                <Text variant="footnote" tone="danger">
                  {t(`errors.${err}`)}
                </Text>
              ) : isBelow ? (
                <Text variant="footnote" tone="danger">
                  {t('edit.below_assigned')}
                </Text>
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
          onPick={(p) => setLines((ls) => addEditLine(ls, p, seller))}
          onClose={() => setPicking(false)}
        />
      ) : null}
    </KeyboardAvoidingView>
  );
}
