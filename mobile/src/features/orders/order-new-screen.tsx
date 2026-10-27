import { useQuery, useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack } from 'expo-router';
import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, TextInput, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { activeMembership, kindsOf } from '@/core/spaces';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { useCart } from './cart';
import { cartIsValid, isRoll, NOTE_MAX, toItemInputs, validateCart } from './order-lines';

const CATALOG = '/bayi/menu/catalog/products';

type Dealer = { uuid: string; name: string };

/** Dealers of the center's network, the buyers it can order for (TEC-1024). */
function useDealers(enabled: boolean) {
  return useQuery({
    queryKey: ['tenant', 'organizations', 'dealers'],
    enabled,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/tenant/organizations', { params: { query: { type: 'dealer', limit: 200 } } })),
    select: (d) => (d.items ?? []).map((o) => ({ uuid: o.uuid, name: o.name })) as Dealer[],
  });
}

/** Required buyer of a center order: the chosen dealer, or a searchable list. */
function DealerPicker({ value, onChange, error }: { value: Dealer | null; onChange: (d: Dealer) => void; error?: string }) {
  const { t } = useTranslation('orders');
  const dealers = useDealers(true);
  const [open, setOpen] = useState(!value);
  const [q, setQ] = useState('');
  const needle = q.trim().toLocaleLowerCase();
  const shown = (dealers.data ?? []).filter((d) => !needle || d.name.toLocaleLowerCase().includes(needle)).slice(0, 50);
  return (
    <View className="gap-2">
      <SectionHeader title={t('buyer_label')} />
      <Pressable
        accessibilityRole="button"
        onPress={() => setOpen((o) => !o)}
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
      {open ? (
        <View className="gap-2 rounded-lg border border-border bg-card p-2">
          <TextInput
            value={q}
            onChangeText={setQ}
            placeholder={t('buyer_search')}
            placeholderTextColorClassName="accent-muted-foreground"
            className="min-h-11 rounded-md border border-border bg-background px-3 text-[17px] text-foreground"
          />
          {shown.map((d) => (
            <Pressable
              key={d.uuid}
              accessibilityRole="button"
              accessibilityState={{ selected: d.uuid === value?.uuid }}
              onPress={() => {
                onChange(d);
                setOpen(false);
                setQ('');
              }}
              className="min-h-11 flex-row items-center justify-between rounded-md px-3 active:bg-muted">
              <Text variant="body" numberOfLines={1} className="flex-1">
                {d.name}
              </Text>
              {d.uuid === value?.uuid ? <Icon name="check" tone="primary" size={16} /> : null}
            </Pressable>
          ))}
          {!dealers.isPending && !shown.length ? (
            <Text variant="footnote" tone="muted" className="px-3 py-2">
              {t('buyer_none')}
            </Text>
          ) : null}
        </View>
      ) : null}
    </View>
  );
}

/**
 * Cart → order (web orders/new, TEC-170): the dealer picks catalog products
 * and amounts; the server resolves the prices (K8). Saving makes a draft;
 * sending moves it to submitted for the seller to approve.
 */
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
                <Button title={t('submit')} size="lg" loading={busy === 'submit'} disabled={!!busy} onPress={() => void save(true)} />
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
