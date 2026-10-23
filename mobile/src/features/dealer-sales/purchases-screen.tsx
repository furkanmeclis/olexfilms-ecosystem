import { useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { useCan } from '@/core/spaces';
import {
  CardRow,
  Chips,
  ErrorState,
  InfoRow,
  ListSkeleton,
  PagedList,
  useChildHref,
  useDebounced,
  useFormat,
} from '@/features/admin/kit';
import { Sheet } from '@/features/services/sheet';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { createPurchase, dealerSalesKeys, type PurchaseListItem, usePurchases, useSupplierOptions } from './api';
import { alertError, success } from './errors';
import { normalizeMoney, PURCHASE_PAYMENT_METHODS, type PurchasePaymentMethod, purchaseErrors } from './lib';

/** One purchase, read only (the web list has no detail page; its columns are shown here). */
function PurchaseSheet({ item, onClose }: { item: PurchaseListItem; onClose: () => void }) {
  const { t } = useTranslation('sales');
  const f = useFormat();
  return (
    <Sheet title={item.supplier_name} saveTitle={t('common:close')} onSave={onClose} onClose={onClose}>
      <Text variant="title">{f.money(item.amount, item.currency)}</Text>
      <List>
        <InfoRow label={t('fields.purchased_on')} value={f.date(item.purchased_on)} />
        <InfoRow label={t('fields.supplier')} value={item.supplier_name} />
        <InfoRow label={t('fields.payment_method')} value={t(`payment_methods.${item.payment_method}`)} />
        <InfoRow label={t('fields.description')} value={item.description} />
        <InfoRow label={t('fields.note')} value={item.note} />
        <InfoRow label={t('fields.created_at')} value={f.dateTime(item.created_at)} last />
      </List>
    </Sheet>
  );
}

/**
 * Bayi › Purchases (web Dealer sales › Purchases, TEC-348): external
 * purchases (an expense on the dealer's book, no stock movement, K12), newest
 * first, with search and a payment filter. A row opens its details; "+" opens
 * a new purchase.
 */
export function PurchasesScreen() {
  const { t } = useTranslation('sales');
  const f = useFormat();
  const href = useChildHref();
  const canWrite = useCan('purchases.write');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const [payment, setPayment] = useState<'' | PurchasePaymentMethod>('');
  const list = usePurchases(query, payment);
  const [open, setOpen] = useState<PurchaseListItem | null>(null);

  return (
    <>
      <Stack.Screen
        options={{
          title: t('purchases.title'),
          headerSearchBarOptions: {
            placeholder: t('purchases.search'),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(''),
          },
          headerRight: canWrite
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('purchases.new')}
                  hitSlop={8}
                  onPress={() => router.push(href('new'))}
                  className="size-8 items-center justify-center">
                  <Icon name="add" tone="primary" />
                </Pressable>
              )
            : undefined,
        }}
      />
      <PagedList
        query={list}
        keyOf={(p) => p.uuid}
        header={
          <Chips
            options={[
              { key: '' as const, label: t('filter.all') },
              ...PURCHASE_PAYMENT_METHODS.map((m) => ({ key: m, label: t(`payment_methods.${m}`) })),
            ]}
            value={payment}
            onChange={setPayment}
          />
        }
        empty={{
          icon: 'truck',
          title: query || payment ? t('purchases.no_results') : t('purchases.empty_title'),
          body: query || payment ? undefined : t('purchases.empty_description'),
        }}
        renderItem={({ item }) => (
          <CardRow
            title={item.supplier_name}
            subtitle={item.description || undefined}
            meta={`${f.date(item.purchased_on)} · ${t(`payment_methods.${item.payment_method}`)}`}
            trailing={
              <Text variant="headline" className="tabular-nums">
                {f.money(item.amount, item.currency)}
              </Text>
            }
            onPress={() => setOpen(item)}
          />
        )}
      />
      {open ? <PurchaseSheet key={open.uuid} item={open} onClose={() => setOpen(null)} /> : null}
    </>
  );
}

/**
 * New purchase (web PurchaseForm): an active supplier, the amount, the
 * payment method and optional description and note. The supplier list needs
 * suppliers.manage like on the web; without it the screen says so.
 */
export function NewPurchaseScreen() {
  const { t } = useTranslation('sales');
  const qc = useQueryClient();
  const canSuppliers = useCan('suppliers.manage');
  const suppliers = useSupplierOptions(canSuppliers);
  const [supplier, setSupplier] = useState('');
  const [amount, setAmount] = useState('');
  const [payment, setPayment] = useState<PurchasePaymentMethod>('cash');
  const [description, setDescription] = useState('');
  const [note, setNote] = useState('');
  const [touched, setTouched] = useState(false);
  const [server, setServer] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const errs = touched ? purchaseErrors(supplier, amount) : {};

  const submit = async () => {
    setTouched(true);
    const amountValue = normalizeMoney(amount);
    if (Object.keys(purchaseErrors(supplier, amount)).length || amountValue === null) return;
    setBusy(true);
    setServer({});
    try {
      await createPurchase({
        supplier_uuid: supplier,
        amount: amountValue,
        payment_method: payment,
        ...(description.trim() ? { description: description.trim() } : {}),
        ...(note.trim() ? { note: note.trim() } : {}),
      });
      void qc.invalidateQueries({ queryKey: dealerSalesKeys.all });
      success();
      router.back();
    } catch (e) {
      if (isApiError(e)) setServer(e.fieldErrors());
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  const items = suppliers.data?.items ?? [];

  return (
    <>
      <Stack.Screen options={{ title: t('purchases.new'), headerLargeTitle: false }} />
      <Screen keyboardShouldPersistTaps="handled">
        <Text variant="callout" tone="muted">
          {t('purchases.description')}
        </Text>
        <SectionHeader title={t('fields.supplier')} />
        {!canSuppliers ? (
          <Text variant="callout" tone="muted">
            {t('purchases.no_supplier_access')}
          </Text>
        ) : suppliers.isPending ? (
          <ListSkeleton rows={2} inset={false} />
        ) : suppliers.isError ? (
          <ErrorState error={suppliers.error} onRetry={() => void suppliers.refetch()} />
        ) : !items.length ? (
          <Text variant="callout" tone="muted">
            {t('purchases.no_suppliers')}
          </Text>
        ) : (
          <List>
            {items.map((s, i, arr) => (
              <ListRow
                key={s.uuid}
                icon="truck"
                title={s.name}
                subtitle={s.phone_e164 ?? s.email ?? undefined}
                onPress={() => setSupplier(s.uuid)}
                trailing={supplier === s.uuid ? <Icon name="check" tone="primary" size={18} /> : undefined}
                last={i === arr.length - 1}
              />
            ))}
          </List>
        )}
        {errs.supplier || server.supplier_uuid ? (
          <Text variant="footnote" tone="danger">
            {errs.supplier ? t('validation.supplier') : server.supplier_uuid}
          </Text>
        ) : null}

        <Input
          label={t('fields.amount')}
          icon="accounting"
          value={amount}
          onChangeText={setAmount}
          placeholder={t('price_placeholder')}
          keyboardType="decimal-pad"
          error={errs.amount ? t('validation.amount') : server.amount}
        />
        <View className="gap-1.5">
          <Text variant="footnote" tone="muted" className="font-medium">
            {t('fields.payment_method')}
          </Text>
          <Chips
            className="-mx-4"
            options={PURCHASE_PAYMENT_METHODS.map((m) => ({ key: m, label: t(`payment_methods.${m}`) }))}
            value={payment}
            onChange={setPayment}
          />
        </View>
        <Input
          label={t('fields.description')}
          icon="report"
          value={description}
          onChangeText={setDescription}
          placeholder={t('purchases.description_placeholder')}
          maxLength={1000}
          error={server.description}
        />
        <Input
          label={t('fields.note')}
          icon="edit"
          value={note}
          onChangeText={setNote}
          placeholder={t('note_placeholder')}
          multiline
          maxLength={5000}
          error={server.note}
        />
        <Button title={t('common:save')} size="lg" loading={busy} onPress={() => void submit()} />
      </Screen>
    </>
  );
}
