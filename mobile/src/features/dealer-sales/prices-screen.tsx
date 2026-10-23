import { useQueryClient } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { CardRow, Chips, InfoRow, PagedList, useDebounced, useFormat } from '@/features/admin/kit';
import { formatDeviation } from '@/features/pricing/lib';
import { Sheet } from '@/features/services/sheet';
import { Input } from '@/ui/input';
import { List } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { dealerSalesKeys, type PriceCatalogItem, type PricedFilter, setDealerPrice, usePriceCatalog } from './api';
import { alertError, success } from './errors';
import { isBelowCost, normalizeMoney, profitOf } from './lib';

/** The recommended price in force for the dealer's country, else the currency-wide list price. */
function recommendedOf(item: PriceCatalogItem) {
  return item.recommended?.price ?? item.recommended_sale_price ?? null;
}

/** Edit of one product's sale price (web inline cell edit, PUT /v1/dealer-prices). */
function PriceSheet({ item, onClose }: { item: PriceCatalogItem; onClose: () => void }) {
  const { t } = useTranslation('sales');
  const f = useFormat();
  const qc = useQueryClient();
  const seeCost = useCan('pricing.purchase.read');
  const seeRecommended = useCan('pricing.recommended.read');
  const [value, setValue] = useState(item.sale_price ?? '');
  const [touched, setTouched] = useState(false);
  const [busy, setBusy] = useState(false);
  const price = normalizeMoney(value);
  const recommended = recommendedOf(item);
  const preview = price ? profitOf(price, item.purchase_price) : null;

  const save = async () => {
    setTouched(true);
    if (price === null) return;
    if (price === item.sale_price) return onClose();
    setBusy(true);
    try {
      await setDealerPrice(item.product_uuid, price);
      await qc.invalidateQueries({ queryKey: dealerSalesKeys.all });
      success();
      if (seeCost && isBelowCost(price, item.purchase_price)) Alert.alert(t('prices.below_cost_hint'));
      onClose();
    } catch (e) {
      alertError(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Sheet title={item.name} busy={busy} onSave={() => void save()} onClose={onClose}>
      <List>
        <InfoRow label={t('fields.sku')} value={item.sku} />
        {seeCost ? (
          <InfoRow
            label={t('fields.purchase_price')}
            value={item.purchase_price ? f.money(item.purchase_price, item.currency) : null}
          />
        ) : null}
        {seeRecommended ? (
          <InfoRow
            label={t('fields.recommended_price')}
            value={
              recommended
                ? `${f.money(recommended, item.recommended?.currency ?? item.currency)}${
                    item.deviation_pct ? ` · ${formatDeviation(item.deviation_pct, f.locale)}` : ''
                  }`
                : null
            }
          />
        ) : null}
        <InfoRow
          label={t('fields.updated_at')}
          value={item.updated_at ? f.dateTime(item.updated_at) : t('prices.not_set')}
          last
        />
      </List>
      <Input
        label={`${t('fields.sale_price')} (${item.currency})`}
        icon="accounting"
        value={value}
        onChangeText={setValue}
        placeholder={t('price_placeholder')}
        keyboardType="decimal-pad"
        autoFocus
        error={touched && price === null ? t('validation.price') : undefined}
        hint={t('prices.edit_hint')}
      />
      {seeCost && preview !== null ? (
        <View className="flex-row items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-3">
          <Text variant="callout" tone="muted">
            {t('fields.estimated_profit')}
          </Text>
          <Text variant="headline" className="tabular-nums">
            {f.money(preview, item.currency)}
          </Text>
        </View>
      ) : null}
      {seeCost && price && isBelowCost(price, item.purchase_price) ? (
        <StatusPill label={t('prices.below_cost')} tone="problem" />
      ) : null}
    </Sheet>
  );
}

/**
 * Bayi › Sale prices (web Dealer sales › Sale prices, TEC-348): the brand's
 * products with the dealer's own sale price, searchable, with a priced
 * filter. A row opens its price sheet (dealer_pricing.write is the gate of
 * both the list and the edit). Purchase price and profit need
 * pricing.purchase.read (the API leaves them empty otherwise).
 */
export function SalePricesScreen() {
  const { t } = useTranslation('sales');
  const f = useFormat();
  const seeCost = useCan('pricing.purchase.read');
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const [priced, setPriced] = useState<PricedFilter>('');
  const list = usePriceCatalog(query, priced);
  const [open, setOpen] = useState<PriceCatalogItem | null>(null);

  return (
    <>
      <Stack.Screen
        options={{
          title: t('prices.title'),
          headerSearchBarOptions: {
            placeholder: t('prices.search'),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(''),
          },
        }}
      />
      <PagedList
        query={list}
        keyOf={(p) => p.product_uuid}
        header={
          <Chips
            options={[
              { key: '' as const, label: t('filter.all') },
              { key: 'true' as const, label: t('prices.priced') },
              { key: 'false' as const, label: t('prices.unpriced') },
            ]}
            value={priced}
            onChange={setPriced}
          />
        }
        empty={{
          icon: 'accounting',
          title: query || priced ? t('prices.no_results') : t('prices.empty_title'),
          body: query || priced ? undefined : t('prices.empty_description'),
        }}
        renderItem={({ item }) => (
          <CardRow
            title={item.name}
            subtitle={item.sku}
            meta={
              item.sale_price
                ? [
                    f.money(item.sale_price, item.currency),
                    seeCost && item.estimated_profit
                      ? t('prices.profit_short', { amount: f.money(item.estimated_profit, item.currency) })
                      : null,
                  ]
                    .filter(Boolean)
                    .join(' · ')
                : t('prices.not_set')
            }
            trailing={
              seeCost && isBelowCost(item.sale_price, item.purchase_price) ? (
                <StatusPill label={t('prices.below_cost')} tone="problem" />
              ) : !item.sale_price ? (
                <StatusPill label={t('prices.unpriced_one')} tone="neutral" />
              ) : undefined
            }
            onPress={() => setOpen(item)}
          />
        )}
      />
      {open ? <PriceSheet key={open.product_uuid} item={open} onClose={() => setOpen(null)} /> : null}
    </>
  );
}
