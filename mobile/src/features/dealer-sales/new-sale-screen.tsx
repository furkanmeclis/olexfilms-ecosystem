import { useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, TextInput, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useCan } from '@/core/spaces';
import { Chips, useDebounced, useFormat } from '@/features/admin/kit';
import { fullName } from '@/features/services/lib';
import { Sheet } from '@/features/services/sheet';
import { SearchField } from '@/features/services/ui';
import { ContinuousScanner, type ScanOutcome } from '@/features/warehouse/continuous-scanner';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { createSale, dealerSalesKeys, lookupSaleItem, useProductSearch } from './api';
import { alertError, failMessage, success } from './errors';
import {
  addLine,
  type CartLine,
  cartTotals,
  hasBarcode,
  isBelowCost,
  lineFromLookup,
  normalizeMoney,
  SALE_PAYMENT_METHODS,
  saleErrors,
  type SalePaymentMethod,
  unitBarcode,
} from './lib';

type Picked = { uuid: string; label: string };

/** Lookup failures in the cart's words (web QuickSaleForm.lookupError). */
function lookupMessage(e: unknown) {
  if (isApiError(e, 'STOCK_UNAVAILABLE')) return 'quick_sale.not_in_stock';
  if (isApiError(e, 'NOT_FOUND')) return 'quick_sale.not_found';
  return null;
}

function CustomerPicker({ onPick, onClose }: { onPick: (c: Picked) => void; onClose: () => void }) {
  const { t } = useTranslation('sales');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const res = useQuery({
    queryKey: ['customers', 'sale-picker', query],
    queryFn: () =>
      unwrap(api.GET('/v1/customers', { params: { query: { q: query || undefined, status: 'active', limit: 20, offset: 0 } } })),
  });
  return (
    <Sheet title={t('fields.customer')} saveTitle={t('common:close')} onSave={onClose} onClose={onClose}>
      <SearchField value={q} onChange={setQ} placeholder={t('quick_sale.customer_placeholder')} autoFocus />
      <View className="overflow-hidden rounded-lg border border-border bg-card">
        {(res.data?.items ?? []).map((c, i, all) => (
          <Pressable
            key={c.uuid}
            accessibilityRole="button"
            disabled={c.anonymized}
            onPress={() => onPick({ uuid: c.uuid, label: c.type === 'corporate' && c.company_name ? c.company_name : fullName(c) })}
            className={`min-h-11 gap-0.5 px-4 py-2.5 active:bg-muted ${i < all.length - 1 ? 'border-b border-border' : ''}`}>
            <Text variant="body" numberOfLines={1}>
              {c.type === 'corporate' && c.company_name ? c.company_name : fullName(c)}
            </Text>
            <Text variant="footnote" tone="muted" numberOfLines={1}>
              {[c.phone, c.email].filter(Boolean).join(' · ')}
            </Text>
          </Pressable>
        ))}
        {res.data && res.data.items.length === 0 ? (
          <Text variant="callout" tone="muted" className="px-4 py-3">
            {t('quick_sale.customers_empty')}
          </Text>
        ) : null}
      </View>
    </Sheet>
  );
}

/** Product search (the price catalog); a pick is resolved to a unit like a scan. */
function ProductPicker({ onPick, onClose }: { onPick: (productUuid: string) => void; onClose: () => void }) {
  const { t } = useTranslation('sales');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const res = useProductSearch(query, true);
  return (
    <Sheet title={t('quick_sale.search_label')} saveTitle={t('common:close')} onSave={onClose} onClose={onClose}>
      <SearchField value={q} onChange={setQ} placeholder={t('quick_sale.search_placeholder')} autoFocus />
      <View className="overflow-hidden rounded-lg border border-border bg-card">
        {(res.data?.items ?? []).map((p, i, all) => (
          <Pressable
            key={p.product_uuid}
            accessibilityRole="button"
            onPress={() => onPick(p.product_uuid)}
            className={`min-h-11 gap-0.5 px-4 py-2.5 active:bg-muted ${i < all.length - 1 ? 'border-b border-border' : ''}`}>
            <Text variant="body" numberOfLines={1}>
              {p.name}
            </Text>
            <Text variant="footnote" tone="muted" className="font-mono" numberOfLines={1}>
              {p.sku}
            </Text>
          </Pressable>
        ))}
        {res.data && res.data.items.length === 0 ? (
          <Text variant="callout" tone="muted" className="px-4 py-3">
            {t('quick_sale.products_empty')}
          </Text>
        ) : null}
      </View>
    </Sheet>
  );
}

/**
 * New quick sale (web QuickSaleForm, TEC-348): scan unit labels (or type a
 * barcode, or search a product), each resolved by /v1/product-sales/lookup to a
 * line with the dealer's price (else the recommended one). A barcode is added
 * once; a fixed barcode's quantity is raised on its line. The customer is
 * optional except for a cari sale.
 */
export function NewSaleScreen() {
  const { t } = useTranslation('sales');
  const f = useFormat();
  const qc = useQueryClient();
  const seeCost = useCan('pricing.purchase.read');
  const [lines, setLinesState] = useState<CartLine[]>([]);
  const linesRef = useRef<CartLine[]>([]);
  const pending = useRef(new Set<string>());
  const [manual, setManual] = useState('');
  const [scanning, setScanning] = useState(false);
  const [picking, setPicking] = useState<'product' | 'customer' | null>(null);
  const [customer, setCustomer] = useState<Picked | null>(null);
  const [payment, setPayment] = useState<SalePaymentMethod>('cash');
  const [note, setNote] = useState('');
  const [touched, setTouched] = useState(false);
  const [busy, setBusy] = useState(false);

  // The scanner calls back between renders: the ref sees lines added a moment ago.
  const setLines = (next: CartLine[]) => {
    linesRef.current = next;
    setLinesState(next);
  };
  const patch = (barcode: string, p: Partial<CartLine>) =>
    setLines(linesRef.current.map((l) => (l.barcode === barcode ? { ...l, ...p } : l)));

  /** Resolves a barcode or a product to a cart line; the outcome feeds the scanner feed. */
  const addItem = async (query: { barcode?: string; product_uuid?: string }): Promise<ScanOutcome> => {
    const key = query.barcode ?? query.product_uuid ?? '';
    if (!key) return { ok: false, title: t('quick_sale.not_found') };
    if (query.barcode && hasBarcode(linesRef.current, query.barcode)) {
      return { ok: false, title: t('quick_sale.already_added', { barcode: query.barcode }) };
    }
    if (pending.current.has(key)) return { ok: false, title: t('quick_sale.already_added', { barcode: key }) };
    pending.current.add(key);
    try {
      const item = await lookupSaleItem(query);
      const res = addLine(linesRef.current, lineFromLookup(item));
      if (!res.added) return { ok: false, title: t('quick_sale.already_added', { barcode: item.barcode }) };
      setLines(res.lines);
      return { ok: true, title: item.name, detail: item.barcode };
    } catch (e) {
      const own = lookupMessage(e);
      return { ok: false, title: own ? t(own) : failMessage(e) };
    } finally {
      pending.current.delete(key);
    }
  };

  /** Typed barcode or picked product: the same lookup, the outcome as an alert when it fails. */
  const addAndReport = async (query: { barcode?: string; product_uuid?: string }) => {
    const out = await addItem(query);
    if (out.ok) success();
    else Alert.alert(out.title);
  };

  const totals = cartTotals(lines);
  const currency = lines[0]?.currency ?? 'TRY';
  const errors = touched ? saleErrors(lines, payment, customer?.uuid ?? null) : {};

  const submit = async () => {
    setTouched(true);
    const errs = saleErrors(lines, payment, customer?.uuid ?? null);
    if (Object.keys(errs).length) return;
    setBusy(true);
    try {
      await createSale({
        customer_uuid: customer?.uuid ?? null,
        payment_method: payment,
        ...(note.trim() ? { note: note.trim() } : {}),
        lines: lines.map((l) => ({
          barcode: l.barcode,
          quantity: l.quantity,
          unit_price: normalizeMoney(l.unitPrice) ?? l.unitPrice,
        })),
      });
      void qc.invalidateQueries({ queryKey: dealerSalesKeys.all });
      void qc.invalidateQueries({ queryKey: ['stock'] });
      success();
      Alert.alert(t('quick_sale.saved'));
      router.back();
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  const askClear = () =>
    Alert.alert(t('quick_sale.clear_confirm'), undefined, [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('quick_sale.clear'), style: 'destructive', onPress: () => setLines([]) },
    ]);

  return (
    <>
      <Stack.Screen
        options={{
          title: t('quick_sale.new'),
          headerLargeTitle: false,
          headerRight: lines.length
            ? () => (
                <Pressable accessibilityRole="button" hitSlop={8} onPress={askClear}>
                  <Text tone="primary">{t('quick_sale.clear')}</Text>
                </Pressable>
              )
            : undefined,
        }}
      />
      <Screen keyboardShouldPersistTaps="handled">
        <Button
          title={t('quick_sale.scan')}
          size="lg"
          icon={<Icon name="scan" tone="primary-foreground" size={20} />}
          onPress={() => setScanning(true)}
        />
        <Input
          label={t('quick_sale.scan_label')}
          icon="barcode"
          value={manual}
          onChangeText={setManual}
          placeholder={t('quick_sale.scan_placeholder')}
          autoCapitalize="characters"
          autoCorrect={false}
          returnKeyType="done"
          onSubmitEditing={() => {
            const code = unitBarcode(manual);
            setManual('');
            if (code) void addAndReport({ barcode: code });
          }}
        />
        <Button
          title={t('quick_sale.search_label')}
          variant="secondary"
          icon={<Icon name="search" tone="primary" size={18} />}
          onPress={() => setPicking('product')}
        />

        <SectionHeader title={t('quick_sale.lines', { count: lines.length })} />
        {lines.length ? (
          <View className="gap-2">
            {lines.map((l) => {
              const priceError = normalizeMoney(l.unitPrice) === null;
              const below = seeCost && isBelowCost(normalizeMoney(l.unitPrice), l.purchasePrice);
              return (
                <View key={l.barcode} className="gap-2 rounded-lg border border-border bg-card px-3.5 py-3">
                  <View className="flex-row items-start gap-3">
                    <View className="flex-1 gap-0.5">
                      <Text variant="headline" numberOfLines={2}>
                        {l.name}
                      </Text>
                      <Text variant="footnote" tone="muted" className="font-mono" numberOfLines={1}>
                        {l.barcode}
                      </Text>
                    </View>
                    <Pressable
                      accessibilityRole="button"
                      accessibilityLabel={t('quick_sale.remove_line')}
                      hitSlop={8}
                      onPress={() => setLines(linesRef.current.filter((x) => x.barcode !== l.barcode))}
                      className="size-8 items-center justify-center rounded-md active:bg-muted">
                      <Icon name="close" tone="muted-foreground" size={16} />
                    </Pressable>
                  </View>
                  <View className="flex-row items-center gap-3">
                    {l.unitKind === 'fixed' && l.available > 1 ? (
                      <View className="flex-row items-center gap-1">
                        <Pressable
                          accessibilityRole="button"
                          accessibilityLabel={t('quick_sale.decrease')}
                          disabled={l.quantity <= 1}
                          onPress={() => patch(l.barcode, { quantity: l.quantity - 1 })}
                          className={`size-9 items-center justify-center rounded-md border border-border ${l.quantity <= 1 ? 'opacity-40' : 'active:bg-muted'}`}>
                          <Icon name="remove" tone="primary" size={16} />
                        </Pressable>
                        <Text variant="headline" className="w-8 text-center tabular-nums">
                          {l.quantity}
                        </Text>
                        <Pressable
                          accessibilityRole="button"
                          accessibilityLabel={t('quick_sale.increase')}
                          disabled={l.quantity >= l.available}
                          onPress={() => patch(l.barcode, { quantity: l.quantity + 1 })}
                          className={`size-9 items-center justify-center rounded-md border border-border ${l.quantity >= l.available ? 'opacity-40' : 'active:bg-muted'}`}>
                          <Icon name="add" tone="primary" size={16} />
                        </Pressable>
                      </View>
                    ) : null}
                    <View className="flex-1" />
                    <TextInput
                      value={l.unitPrice}
                      onChangeText={(v) => patch(l.barcode, { unitPrice: v })}
                      accessibilityLabel={t('fields.unit_price')}
                      placeholder={t('price_placeholder')}
                      placeholderTextColorClassName="accent-muted-foreground"
                      keyboardType="decimal-pad"
                      className={`min-h-10 w-32 rounded-md border bg-background px-2 text-right text-[17px] text-foreground tabular-nums ${priceError ? 'border-destructive' : 'border-border'}`}
                    />
                    <Text variant="callout" tone="muted">
                      {l.currency}
                    </Text>
                  </View>
                  {below ? <StatusPill label={t('prices.below_cost')} tone="problem" /> : null}
                </View>
              );
            })}
          </View>
        ) : (
          <Text variant="callout" tone="muted">
            {t('quick_sale.empty_cart')}
          </Text>
        )}
        {errors.lines ? (
          <Text variant="footnote" tone="danger">
            {t(`validation.${errors.lines}`)}
          </Text>
        ) : null}

        <SectionHeader title={t('quick_sale.payment')} />
        <List>
          <ListRow
            icon="customer"
            title={customer?.label ?? t('fields.customer_optional')}
            subtitle={customer ? t('fields.customer') : undefined}
            onPress={() => setPicking('customer')}
            trailing={
              customer ? (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('quick_sale.clear_customer')}
                  hitSlop={8}
                  onPress={() => setCustomer(null)}>
                  <Icon name="close" tone="muted-foreground" size={16} />
                </Pressable>
              ) : undefined
            }
            last
          />
        </List>
        {errors.customer ? (
          <Text variant="footnote" tone="danger">
            {t(`validation.${errors.customer}`)}
          </Text>
        ) : null}
        <Chips
          className="-mx-4"
          options={SALE_PAYMENT_METHODS.map((m) => ({ key: m, label: t(`payment_methods.${m}`) }))}
          value={payment}
          onChange={setPayment}
        />
        <Input
          label={t('fields.note')}
          icon="edit"
          value={note}
          onChangeText={setNote}
          placeholder={t('note_placeholder')}
          multiline
          maxLength={5000}
        />

        <View className="gap-1.5 rounded-lg border border-border bg-card px-4 py-3">
          <View className="flex-row justify-between gap-2">
            <Text variant="headline">{t('fields.total')}</Text>
            <Text variant="headline" className="tabular-nums">
              {f.money(totals.total, currency)}
            </Text>
          </View>
          {seeCost ? (
            <View className="flex-row justify-between gap-2">
              <Text variant="callout" tone="muted">
                {t('fields.profit_preview')}
              </Text>
              <Text variant="callout" className="tabular-nums">
                {totals.profit !== null ? f.money(totals.profit, currency) : '–'}
              </Text>
            </View>
          ) : null}
          {seeCost && totals.profitIncomplete ? (
            <Text variant="footnote" tone="muted">
              {t('quick_sale.profit_incomplete')}
            </Text>
          ) : null}
        </View>
        <Button title={t('quick_sale.submit')} size="lg" loading={busy} disabled={!lines.length} onPress={() => void submit()} />
      </Screen>
      <ContinuousScanner
        visible={scanning}
        onClose={() => setScanning(false)}
        title={t('quick_sale.scan')}
        hint={t('quick_sale.scan_hint')}
        onCode={(code) => addItem({ barcode: unitBarcode(code) })}
      />
      {picking === 'product' ? (
        <ProductPicker
          onClose={() => setPicking(null)}
          onPick={(uuid) => {
            setPicking(null);
            void addAndReport({ product_uuid: uuid });
          }}
        />
      ) : null}
      {picking === 'customer' ? (
        <CustomerPicker
          onClose={() => setPicking(null)}
          onPick={(c) => {
            setCustomer(c);
            setPicking(null);
          }}
        />
      ) : null}
    </>
  );
}
