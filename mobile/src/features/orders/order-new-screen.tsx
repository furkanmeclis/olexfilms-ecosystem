import { useQueryClient } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, Stack } from 'expo-router';
import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, TextInput, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { useCart } from './cart';
import { cartIsValid, isRoll, NOTE_MAX, toItemInputs, validateCart } from './order-lines';

const CATALOG = '/bayi/menu/catalog/products';

/**
 * Cart → order (web orders/new, TEC-170): the dealer picks catalog products
 * and amounts; the server resolves the prices (K8). Saving makes a draft;
 * sending moves it to submitted for the seller to approve.
 */
export function OrderNewScreen() {
  const { t } = useTranslation('orders');
  const qc = useQueryClient();
  const { lines, note, setAmount, remove, setNote, clear } = useCart();
  const [touched, setTouched] = useState(false);
  const [busy, setBusy] = useState<'draft' | 'submit' | null>(null);
  const errors = useMemo(() => validateCart(lines), [lines]);

  // Refs, not state: a quick double tap must not reach the API twice, and a
  // draft created before a failed submit is retried, never created again.
  const inFlight = useRef(false);
  const created = useRef<string | null>(null);

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
              {t('price_rule')}
            </Text>
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
          </>
        )}
      </Screen>
    </>
  );
}
