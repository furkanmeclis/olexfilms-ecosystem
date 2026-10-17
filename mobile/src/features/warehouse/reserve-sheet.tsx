import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, KeyboardAvoidingView, Modal, Platform, Pressable, ScrollView, TextInput, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';

import { whError } from './format';
import { ListSkeleton } from './parts';

type Product = components['schemas']['CatalogProduct'];

/** Active catalog products by name or SKU (GET /v1/catalog/products). */
function useProductSearch(q: string) {
  return useQuery({
    queryKey: ['warehouse', 'catalog', q],
    placeholderData: keepPreviousData,
    queryFn: () =>
      unwrap(api.GET('/v1/catalog/products', { params: { query: { q: q || undefined, active: true, limit: 30 } } })),
    select: (d) => d.items,
  });
}

/** Search-and-pick list of catalog products. */
function ProductPicker({ value, onPick }: { value?: string; onPick: (p: Product) => void }) {
  const { t } = useTranslation('warehouse');
  const [input, setInput] = useState('');
  const [q, setQ] = useState('');
  useEffect(() => {
    const id = setTimeout(() => setQ(input.trim()), 300);
    return () => clearTimeout(id);
  }, [input]);
  const res = useProductSearch(q);
  return (
    <View className="gap-2">
      <View className="min-h-11 flex-row items-center gap-2 rounded-lg border border-border bg-card px-3">
        <Icon name="search" tone="muted-foreground" size={18} />
        <TextInput
          value={input}
          onChangeText={setInput}
          placeholder={t('stock.filters.product_search')}
          placeholderTextColorClassName="accent-muted-foreground"
          autoCorrect={false}
          className="min-h-11 flex-1 text-[17px] text-foreground"
        />
      </View>
      {res.isLoading ? (
        <ListSkeleton rows={3} />
      ) : (
        <FlatList
          data={res.data ?? []}
          keyExtractor={(p) => p.uuid}
          scrollEnabled={false}
          contentContainerClassName="gap-1.5"
          ListEmptyComponent={
            <Text variant="callout" tone="muted">
              {res.error ? whError(res.error) : t('orders.form.product_none')}
            </Text>
          }
          renderItem={({ item }) => (
            <Pressable
              accessibilityRole="button"
              onPress={() => onPick(item)}
              className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-2.5 active:opacity-70">
              <View className="flex-1 gap-0.5">
                <Text variant="callout" numberOfLines={2}>
                  {item.name}
                </Text>
                <Text variant="footnote" tone="muted" className="font-mono">
                  {item.sku} · {t(`unit_type.${item.unit_type}`)}
                </Text>
              </View>
              {item.uuid === value ? <Icon name="check" tone="primary" size={18} /> : null}
            </Pressable>
          )}
        />
      )}
    </View>
  );
}

export type ReserveInput = {
  product_uuid: string;
  quantity: number;
  meters?: string;
  prefix?: string;
  fixed_quantity?: number;
};

/**
 * Reserve new barcodes on a generate_new entry (center only): product,
 * how many, roll length or fixed quantity, optional prefix.
 */
export function ReserveSheet({ onClose, onSubmit }: { onClose: () => void; onSubmit: (v: ReserveInput) => Promise<unknown> }) {
  const { t } = useTranslation('warehouse');
  const [product, setProduct] = useState<Product | null>(null);
  const [count, setCount] = useState('1');
  const [meters, setMeters] = useState('');
  const [fixedQty, setFixedQty] = useState('1');
  const [prefix, setPrefix] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tried, setTried] = useState(false);

  const roll = product?.unit_type === 'roll_meter';
  const n = Number(count);
  const countOk = Number.isInteger(n) && n >= 1 && n <= 1000;
  const metersOk = !roll || /^\d+([.,]\d{1,2})?$/.test(meters.trim());
  const prefixOk = !prefix.trim() || /^[A-Za-z0-9]{2,8}$/.test(prefix.trim());
  const fq = Number(fixedQty);
  const fixedOk = !product?.uses_fixed_barcode || (Number.isInteger(fq) && fq >= 1);

  const submit = async () => {
    setTried(true);
    if (!product || !countOk || !metersOk || !prefixOk || !fixedOk) return;
    setBusy(true);
    setError(null);
    try {
      await onSubmit({
        product_uuid: product.uuid,
        quantity: n,
        meters: roll ? meters.trim().replace(',', '.') : undefined,
        prefix: prefix.trim() ? prefix.trim().toUpperCase() : undefined,
        fixed_quantity: product.uses_fixed_barcode ? fq : undefined,
      });
      onClose();
    } catch (e) {
      setError(whError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal visible animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} className="flex-1 bg-background">
        <View className="flex-row items-center justify-between px-4 pb-2 pt-5">
          <Text variant="title" className="flex-1" numberOfLines={1}>
            {t('entry.generate')}
          </Text>
          <Button title={t('common:cancel')} variant="ghost" onPress={onClose} />
        </View>
        <ScrollView contentContainerClassName="gap-4 px-4 pb-10" keyboardShouldPersistTaps="handled">
          {product ? (
            <Pressable
              accessibilityRole="button"
              onPress={() => setProduct(null)}
              className="gap-0.5 rounded-lg border border-primary bg-secondary px-4 py-3">
              <Text variant="caption" tone="muted">
                {t('fields.product')}
              </Text>
              <Text variant="headline">{product.name}</Text>
              <Text variant="footnote" tone="muted" className="font-mono">
                {product.sku} · {t(`unit_type.${product.unit_type}`)}
              </Text>
            </Pressable>
          ) : (
            <>
              <ProductPicker onPick={setProduct} />
              {tried ? (
                <Text variant="footnote" tone="danger">
                  {t('validation.product')}
                </Text>
              ) : null}
            </>
          )}
          {product ? (
            <>
              <Input
                label={roll ? t('m.roll_count') : t('m.barcode_count')}
                value={count}
                onChangeText={setCount}
                keyboardType="number-pad"
                error={tried && !countOk ? t('validation.quantity', { max: 1000 }) : undefined}
              />
              {roll ? (
                <Input
                  label={t('fields.meters')}
                  value={meters}
                  onChangeText={setMeters}
                  keyboardType="decimal-pad"
                  placeholder="15.00"
                  error={tried && !metersOk ? t('validation.meters') : undefined}
                />
              ) : null}
              {product.uses_fixed_barcode ? (
                <Input
                  label={t('fixed.quantity_label')}
                  value={fixedQty}
                  onChangeText={setFixedQty}
                  keyboardType="number-pad"
                  error={tried && !fixedOk ? t('fixed.quantity_invalid') : undefined}
                />
              ) : null}
              <Input
                label={t('fields.prefix')}
                value={prefix}
                onChangeText={setPrefix}
                autoCapitalize="characters"
                autoCorrect={false}
                hint={t('m.prefix_hint')}
                error={tried && !prefixOk ? t('validation.prefix') : undefined}
              />
              <Text variant="footnote" tone="muted">
                {t('m.labels_web')}
              </Text>
              {error ? (
                <Text variant="footnote" tone="danger">
                  {error}
                </Text>
              ) : null}
              <Button size="lg" title={t('entry.generate')} loading={busy} onPress={() => void submit()} />
            </>
          ) : null}
        </ScrollView>
      </KeyboardAvoidingView>
    </Modal>
  );
}
