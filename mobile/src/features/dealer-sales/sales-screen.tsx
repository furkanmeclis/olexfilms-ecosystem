import { useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { CardRow, Chips, InfoRow, PagedList, useChildHref, useDebounced, useFormat } from '@/features/admin/kit';
import { Sheet } from '@/features/services/sheet';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { List } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { dealerSalesKeys, type ProductSaleListItem, useProductSales, voidSale } from './api';
import { alertError, success } from './errors';
import { canVoidSale, SALE_PAYMENT_METHODS, type SalePaymentMethod } from './lib';

/** One sale: its fields and, on the sale day, the void with a reason (web VoidSaleDialog). */
function SaleSheet({ sale, onClose }: { sale: ProductSaleListItem; onClose: () => void }) {
  const { t } = useTranslation('sales');
  const f = useFormat();
  const qc = useQueryClient();
  const [voiding, setVoiding] = useState(false);
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const voidable = canVoidSale(sale);

  const run = async () => {
    setBusy(true);
    try {
      await voidSale(sale.uuid, reason.trim());
      await qc.invalidateQueries({ queryKey: dealerSalesKeys.all });
      void qc.invalidateQueries({ queryKey: ['stock'] });
      success();
      onClose();
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Sheet
      title={voiding ? t('sales.void_title') : t('sales.detail')}
      saveTitle={voiding ? t('sales.void') : t('common:close')}
      busy={busy}
      onSave={voiding ? () => void run() : onClose}
      onClose={voiding ? () => setVoiding(false) : onClose}>
      {voiding ? (
        <>
          <Text variant="callout" tone="muted">
            {t('sales.void_description')}
          </Text>
          <Input
            label={t('fields.reason')}
            icon="edit"
            value={reason}
            onChangeText={setReason}
            placeholder={t('sales.reason_placeholder')}
            multiline
            maxLength={500}
            autoFocus
          />
        </>
      ) : (
        <>
          <View className="flex-row items-center justify-between gap-3">
            <Text variant="title" className="flex-1">
              {f.money(sale.total, sale.currency)}
            </Text>
            <StatusPill
              label={sale.voided ? t('sales.status_voided') : t('sales.status_completed')}
              tone={sale.voided ? 'problem' : 'done'}
            />
          </View>
          <List>
            <InfoRow label={t('fields.sold_at')} value={f.dateTime(sale.sold_at)} />
            <InfoRow label={t('fields.products')} value={sale.products} />
            <InfoRow label={t('fields.line_count')} value={String(sale.line_count)} />
            <InfoRow label={t('fields.customer')} value={sale.customer_name} />
            <InfoRow label={t('fields.payment_method')} value={t(`payment_methods.${sale.payment_method}`)} />
            {sale.profit ? <InfoRow label={t('fields.profit')} value={f.money(sale.profit, sale.currency)} /> : null}
            <InfoRow label={t('fields.note')} value={sale.note} last />
          </List>
          {voidable ? (
            <Button
              title={t('sales.void')}
              variant="destructive"
              icon={<Icon name="blocked" tone="primary-foreground" size={18} />}
              onPress={() => setVoiding(true)}
            />
          ) : (
            <Text variant="footnote" tone="muted">
              {sale.voided ? t('sales.already_voided') : t('sales.void_window')}
            </Text>
          )}
        </>
      )}
    </Sheet>
  );
}

/**
 * Bayi › Quick sale (web Dealer sales › Quick sale, TEC-348): the dealer's
 * sales, newest first, with search and a payment filter. A new sale opens the
 * scan cart; a sale opens its sheet, where it is voided on its sale day.
 */
export function SalesScreen() {
  const { t } = useTranslation('sales');
  const f = useFormat();
  const href = useChildHref();
  const canSell = useCan('product_sales.write');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const [payment, setPayment] = useState<'' | SalePaymentMethod>('');
  const list = useProductSales(query, payment);
  const [open, setOpen] = useState<ProductSaleListItem | null>(null);

  return (
    <>
      <Stack.Screen
        options={{
          title: t('quick_sale.title'),
          headerSearchBarOptions: {
            placeholder: t('sales.search'),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(''),
          },
          headerRight: canSell
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('quick_sale.new')}
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
        keyOf={(s) => s.uuid}
        header={
          <View>
            {canSell ? (
              <View className="px-4 pt-2">
                <Button
                  title={t('quick_sale.new')}
                  size="lg"
                  icon={<Icon name="scan" tone="primary-foreground" size={20} />}
                  onPress={() => router.push(href('new'))}
                />
              </View>
            ) : null}
            <Chips
              options={[
                { key: '' as const, label: t('filter.all') },
                ...SALE_PAYMENT_METHODS.map((m) => ({ key: m, label: t(`payment_methods.${m}`) })),
              ]}
              value={payment}
              onChange={setPayment}
            />
          </View>
        }
        empty={{
          icon: 'cart',
          title: query || payment ? t('sales.no_results') : t('sales.empty_title'),
          body: query || payment ? undefined : t('sales.empty_description'),
        }}
        renderItem={({ item }) => (
          <CardRow
            title={f.money(item.total, item.currency)}
            subtitle={item.products}
            meta={[f.dateTime(item.sold_at), t(`payment_methods.${item.payment_method}`), item.customer_name]
              .filter(Boolean)
              .join(' · ')}
            trailing={
              <StatusPill
                label={item.voided ? t('sales.status_voided') : t('sales.status_completed')}
                tone={item.voided ? 'problem' : 'done'}
              />
            }
            onPress={() => setOpen(item)}
          />
        )}
      />
      {open ? <SaleSheet key={open.uuid} sale={open} onClose={() => setOpen(null)} /> : null}
    </>
  );
}
