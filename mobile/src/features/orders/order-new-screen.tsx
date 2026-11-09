import { useQuery, useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack } from 'expo-router';
import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, Modal, Platform, Pressable, TextInput, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { activeMembership, kindsOf } from '@/core/spaces';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { cn } from '@/ui/cn';
import { confirmAction } from '@/ui/confirm';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { useCart } from './cart';
import { cartIsValid, isRoll, NOTE_MAX, submitPreview, toItemInputs, validateCart } from './order-lines';

const CATALOG = '/bayi/menu/catalog/products';

type Dealer = { uuid: string; name: string };

/** Dealers of the center's network, the buyers it can order for (TEC-1024). */
function useDealers(enabled: boolean) {
  return useQuery({
    queryKey: ['tenant', 'organizations', 'dealers'],
    enabled,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/organizations', { params: { query: { type: 'dealer', limit: 200, include_pending: true } } })),
    select: (d) => (d.items ?? []).map((o) => ({ uuid: o.uuid, name: o.name })) as Dealer[],
  });
}

/** Required buyer of a center order: a row that opens a searchable sheet, like the other pickers. */
function DealerPicker({ value, onChange, error }: { value: Dealer | null; onChange: (d: Dealer) => void; error?: string }) {
  const { t } = useTranslation('orders');
  const [open, setOpen] = useState(false);
  return (
    <View className="gap-2">
      <SectionHeader title={t('buyer_label')} />
      <Pressable
        accessibilityRole="button"
        onPress={() => setOpen(true)}
        className={cn(
          'min-h-14 flex-row items-center gap-3 rounded-lg border bg-card px-4 active:opacity-80',
          error ? 'border-destructive' : 'border-border',
        )}>
        <Icon name="building" tone="primary" size={20} />
        <Text variant="body" tone={value ? 'default' : 'muted'} className="flex-1" numberOfLines={1}>
          {value?.name ?? t('buyer_pick')}
        </Text>
        <Icon name="chevronDown" tone="muted-foreground" size={14} />
      </Pressable>
      {error ? (
        <Text variant="footnote" tone="danger">
          {error}
        </Text>
      ) : null}
      <DealerSheet
        visible={open}
        value={value}
        onClose={() => setOpen(false)}
        onPick={(d) => {
          onChange(d);
          setOpen(false);
        }}
      />
    </View>
  );
}

function DealerSheet({
  visible,
  value,
  onClose,
  onPick,
}: {
  visible: boolean;
  value: Dealer | null;
  onClose: () => void;
  onPick: (d: Dealer) => void;
}) {
  const { t } = useTranslation('orders');
  const insets = useSafeAreaInsets();
  const dealers = useDealers(visible);
  const [q, setQ] = useState('');
  const needle = q.trim().toLocaleLowerCase();
  const results = (dealers.data ?? []).filter((d) => !needle || d.name.toLocaleLowerCase().includes(needle));
  const close = () => {
    setQ('');
    onClose();
  };
  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={close}>
      {/* iOS presents a page sheet below the status bar; Android shows it full screen. */}
      <View className="flex-1 bg-background" style={{ paddingTop: Platform.OS === 'ios' ? 0 : insets.top }}>
        <View className="flex-row items-center justify-between ps-5 pe-2 pb-2 pt-4">
          <Text variant="title">{t('buyer_label')}</Text>
          <Button title={t('common:close')} variant="ghost" onPress={close} />
        </View>
        <View className="px-4 pb-3">
          <View className="min-h-11 flex-row items-center gap-2 rounded-xl bg-secondary px-3">
            <Icon name="search" tone="muted-foreground" size={17} />
            <TextInput
              value={q}
              onChangeText={setQ}
              placeholder={t('buyer_search')}
              placeholderTextColorClassName="accent-muted-foreground"
              accessibilityLabel={t('buyer_search')}
              autoCorrect={false}
              clearButtonMode="while-editing"
              returnKeyType="search"
              className="min-h-11 flex-1 text-[17px] text-foreground"
            />
          </View>
        </View>
        <FlatList
          data={results}
          keyExtractor={(d) => d.uuid}
          keyboardShouldPersistTaps="handled"
          keyboardDismissMode="on-drag"
          contentContainerStyle={{ paddingBottom: insets.bottom + 16 }}
          ItemSeparatorComponent={() => <View className="ms-14 h-px bg-border" />}
          ListEmptyComponent={
            <Text variant="callout" tone="muted" className="px-5 py-8 text-center">
              {dealers.isPending ? t('common:loading') : t('buyer_none')}
            </Text>
          }
          renderItem={({ item }) => (
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ selected: item.uuid === value?.uuid }}
              onPress={() => {
                void Haptics.selectionAsync();
                setQ('');
                onPick(item);
              }}
              className="min-h-[52px] flex-row items-center gap-3 px-4 active:bg-secondary">
              <View className="size-8 items-center justify-center rounded-md bg-secondary">
                <Icon name="building" tone="primary" size={16} />
              </View>
              <Text variant="body" className="flex-1" numberOfLines={1}>
                {item.name}
              </Text>
              {item.uuid === value?.uuid ? <Icon name="check" tone="primary" size={18} weight="semibold" /> : null}
            </Pressable>
          )}
        />
      </View>
    </Modal>
  );
}

export function OrderNewScreen() {
  const { t } = useTranslation('orders');
  const qc = useQueryClient();
  const { lines, note, setAmount, setPrice, remove, setNote, clear } = useCart();
  const me = useSession((s) => s.me);
  // TEC-1024: the center orders for one of its dealers; the order starts approved.
  const center = activeMembership(me)?.type === 'center';
  const warehouse = kindsOf(me).includes('depo');
  const [buyer, setBuyer] = useState<Dealer | null>(null);
  const [touched, setTouched] = useState(false);
  const [busy, setBusy] = useState<'draft' | 'submit' | null>(null);
  const errors = useMemo(() => validateCart(lines), [lines]);

  // Refs, not state: a quick double tap must not reach the API twice, and a
  // draft created before a failed submit is retried, never created again.
  const inFlight = useRef(false);
  const created = useRef<string | null>(null);

  /** Center: one approved order for the chosen dealer, then straight to preparing it. */
  const createForDealer = async () => {
    setTouched(true);
    if (inFlight.current || !cartIsValid(errors) || !buyer) return;
    inFlight.current = true;
    setBusy('submit');
    try {
      const order = await unwrap(
        api.POST('/v1/orders', {
          body: {
            buyer_org_uuid: buyer.uuid,
            items: toItemInputs(lines, true),
            ...(note.trim() ? { note: note.trim() } : {}),
          },
        }),
      );
      void qc.invalidateQueries({ queryKey: ['orders', 'list'] });
      void qc.invalidateQueries({ queryKey: ['warehouse', 'orders'] });
      clear();
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      const missing = order.items?.filter((i) => i.price_missing).length ?? 0;
      router.replace((warehouse ? `/depo/jobs/order/${order.uuid}` : `/bayi/menu/orders/${order.uuid}`) as never);
      if (missing) Alert.alert(t('price_missing_title'), t('price_missing_body', { count: missing }));
    } catch (e) {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      Alert.alert(t('save_failed'), errorMessage(isApiError(e) ? e.code : undefined));
    } finally {
      inFlight.current = false;
      setBusy(null);
    }
  };

  const save = async (submit: boolean) => {
    setTouched(true);
    if (inFlight.current || !cartIsValid(errors)) return;
    inFlight.current = true;
    setBusy(submit ? 'submit' : 'draft');
    try {
      let uuid = created.current;
      let order = null;
      if (!uuid) {
        order = await unwrap(
          api.POST('/v1/orders', { body: { items: toItemInputs(lines), ...(note.trim() ? { note: note.trim() } : {}) } }),
        );
        uuid = order.uuid;
        created.current = uuid;
      } else {
        // Retry after a failed submit: the draft takes the cart as it is now.
        order = await unwrap(
          api.PUT('/v1/orders/{uuid}/items', { params: { path: { uuid } }, body: { items: toItemInputs(lines) } }),
        );
      }
      if (submit) {
        order = await unwrap(api.POST('/v1/orders/{uuid}/transitions', { params: { path: { uuid } }, body: { status: 'submitted' } }));
      }
      if (order) qc.setQueryData(['orders', 'detail', uuid], order);
      void qc.invalidateQueries({ queryKey: ['orders', 'list'] });
      clear();
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      router.replace(`/bayi/menu/orders/${uuid}` as never);
    } catch (e) {
      void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error);
      // The draft exists: say so, so a retry (send or open) is understood.
      const msg = errorMessage(isApiError(e) ? e.code : undefined);
      Alert.alert(t('save_failed'), created.current ? `${msg}\n\n${t('draft_kept')}` : msg);
    } finally {
      inFlight.current = false;
      setBusy(null);
    }
  };

  /** Dealer submit: the lines and amounts are confirmed first (prices follow on approval). */
  const askSubmit = () => {
    setTouched(true);
    if (inFlight.current || !cartIsValid(errors)) return;
    const { rows, more } = submitPreview(lines);
    const list = rows.map((r) => t(r.roll ? 'confirm_line_meters' : 'confirm_line_pieces', { name: r.name, amount: r.amount }));
    if (more) list.push(t('confirm_more', { count: more }));
    confirmAction({
      title: t('confirm_submit_title', { count: lines.length }),
      body: `${list.join('\n')}\n\n${t('confirm_no_price')}`,
      action: t('submit'),
      onConfirm: () => void save(true),
    });
  };

  const askClear = () =>
    Alert.alert(t('clear_title'), undefined, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('clear'), style: 'destructive', onPress: clear },
    ]);

  return (
    <>
      <Stack.Screen
        options={{
          title: t('new_title'),
          headerLargeTitle: false,
          headerRight: lines.length
            ? () => (
                <Pressable accessibilityRole="button" hitSlop={8} onPress={askClear}>
                  <Text tone="primary">{t('clear')}</Text>
                </Pressable>
              )
            : undefined,
        }}
      />
      <Screen keyboardShouldPersistTaps="handled">
        {!lines.length ? (
          <EmptyState
            icon="cart"
            title={t('cart_empty_title')}
            body={t('cart_empty_body')}
            action={{ title: t('browse_catalog'), onPress: () => router.push(CATALOG as never) }}
          />
        ) : (
          <>
            {center ? <DealerPicker value={buyer} onChange={setBuyer} error={touched && !buyer ? t('buyer_required') : undefined} /> : null}
            <SectionHeader title={t('lines', { count: lines.length })} />
            <View className="gap-2.5">
              {lines.map((l) => {
                const err = touched ? errors.lines[l.product_uuid] : undefined;
                const roll = isRoll(l.unit_type);
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
                      </View>
                      <Pressable
                        accessibilityRole="button"
                        accessibilityLabel={t('remove')}
                        hitSlop={8}
                        onPress={() => remove(l.product_uuid)}
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
                        onChangeText={(v) => setAmount(l.product_uuid, v)}
                        accessibilityLabel={roll ? t('amount_meters') : t('amount_pieces')}
                        placeholder={roll ? t('placeholder_meters') : t('placeholder_pieces')}
                        placeholderTextColorClassName="accent-muted-foreground"
                        keyboardType={roll ? 'decimal-pad' : 'number-pad'}
                        selectTextOnFocus
                        className="min-h-11 w-28 rounded-md border border-border bg-background px-3 text-right text-[17px] text-foreground tabular-nums"
                      />
                    </View>
                    {center ? (
                      <View className="flex-row items-center gap-3">
                        <Text variant="callout" tone="muted" className="flex-1">
                          {t('unit_price')}
                        </Text>
                        <TextInput
                          value={l.unit_price ?? ''}
                          onChangeText={(v) => setPrice(l.product_uuid, v)}
                          accessibilityLabel={t('unit_price')}
                          placeholder={t('unit_price_auto')}
                          placeholderTextColorClassName="accent-muted-foreground"
                          keyboardType="decimal-pad"
                          selectTextOnFocus
                          className="min-h-11 w-28 rounded-md border border-border bg-background px-3 text-right text-[17px] text-foreground tabular-nums"
                        />
                      </View>
                    ) : null}
                    {err ? (
                      <Text variant="footnote" tone="danger">
                        {t(`errors.${err}`)}
                      </Text>
                    ) : null}
                  </View>
                );
              })}
            </View>
            <Button title={t('add_products')} icon="add" variant="ghost" onPress={() => router.push(CATALOG as never)} />
            {touched && errors.form === 'too_many' ? (
              <Text variant="footnote" tone="danger">
                {t('errors.too_many')}
              </Text>
            ) : null}

            <Input
              label={t('note')}
              icon="edit"
              value={note}
              onChangeText={setNote}
              placeholder={t('note_placeholder')}
              multiline
              maxLength={NOTE_MAX}
            />
            <Text variant="footnote" tone="muted">
              {t(center ? 'price_rule_center' : 'price_rule')}
            </Text>
            {center ? (
              <Button
                title={t('create_for_dealer')}
                size="lg"
                loading={busy === 'submit'}
                disabled={!!busy}
                onPress={() => void createForDealer()}
              />
            ) : (
              <View className="gap-2.5">
                <Button title={t('submit')} size="lg" loading={busy === 'submit'} disabled={!!busy} onPress={askSubmit} />
                <Button
                  title={t('save_draft')}
                  variant="outline"
                  loading={busy === 'draft'}
                  disabled={!!busy}
                  onPress={() => void save(false)}
                />
              </View>
            )}
          </>
        )}
      </Screen>
    </>
  );
}
