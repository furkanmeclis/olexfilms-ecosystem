import * as Haptics from 'expo-haptics';
import { Image } from 'expo-image';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl, ScrollView, useWindowDimensions, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { ErrorState, ListSkeleton, PagedList, useDebounced } from '@/features/admin/kit';
import { useCart } from '@/features/orders/cart';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

import { type Category, firstImage, type Product, productImageUrl, useCategories, useProduct, useProducts } from './api';

const BASE = '/bayi/menu/catalog';
const NEW_ORDER = '/bayi/menu/orders/new';

/** A dealer may order (orders.write) only what the catalog shows (catalog.read). */
export function useCanOrder() {
  const write = useCan('orders.write');
  const read = useCan('catalog.read');
  return write && read;
}

/** Header cart button with the number of lines. */
export function CartButton() {
  const { t } = useTranslation('catalog');
  const count = useCart((s) => s.lines.length);
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={t('cart_open', { count })}
      hitSlop={8}
      onPress={() => router.push(NEW_ORDER as never)}
      className="h-8 min-w-8 flex-row items-center justify-center gap-1">
      <Icon name="cart" tone="primary" />
      {count ? (
        <Text variant="footnote" tone="primary" className="font-semibold tabular-nums">
          {count}
        </Text>
      ) : null}
    </Pressable>
  );
}

function Thumb({ uri, size = 52 }: { uri: string | null; size?: number }) {
  return (
    <View style={{ width: size, height: size }} className="items-center justify-center overflow-hidden rounded-md bg-secondary">
      {uri ? (
        <Image source={{ uri }} style={{ width: size, height: size }} contentFit="cover" transition={200} />
      ) : (
        <Icon name="package" tone="muted-foreground" size={size / 2.4} />
      )}
    </View>
  );
}

/** Catalog home: the brand's categories, plus every product. */
export function CatalogScreen() {
  const { t } = useTranslation('catalog');
  const canOrder = useCanOrder();
  const q = useCategories();

  const open = (c?: Category) =>
    router.push({ pathname: `${BASE}/products` as never, params: c ? { category: c.uuid, name: c.name } : {} });

  return (
    <>
      <Stack.Screen options={{ title: t('title'), headerRight: canOrder ? () => <CartButton /> : undefined }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        <List>
          <ListRow icon="search" title={t('all_products')} subtitle={t('all_products_hint')} onPress={() => open()} last />
        </List>
        <SectionHeader title={t('categories')} />
        {q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : q.isError && !q.data ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : q.data?.length ? (
          <List>
            {q.data.map((c, i) => (
              <ListRow key={c.uuid} icon="package" title={c.name} onPress={() => open(c)} last={i === q.data.length - 1} />
            ))}
          </List>
        ) : (
          <Text variant="callout" tone="muted" className="px-4">
            {t('no_categories')}
          </Text>
        )}
      </Screen>
    </>
  );
}

/** Products of one category (or all), searchable by name or SKU. */
export function ProductsScreen() {
  const { t } = useTranslation('catalog');
  const canOrder = useCanOrder();
  const params = useLocalSearchParams<{ category?: string; name?: string }>();
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const list = useProducts(params.category ?? '', query);
  const lines = useCart((s) => s.lines);
  const add = useCart((s) => s.add);

  return (
    <>
      <Stack.Screen
        options={{
          title: params.name || t('all_products'),
          headerLargeTitle: false,
          headerSearchBarOptions: {
            placeholder: t('search'),
            hideWhenScrolling: false,
            onChangeText: (e) => setQ(e.nativeEvent.text),
            onCancelButtonPress: () => setQ(''),
          },
          headerRight: canOrder ? () => <CartButton /> : undefined,
        }}
      />
      <PagedList
        query={list}
        keyOf={(p) => p.uuid}
        empty={{ icon: 'package', title: query ? t('no_results') : t('empty') }}
        renderItem={({ item }) => {
          const inCart = lines.some((l) => l.product_uuid === item.uuid);
          return (
            <Pressable
              accessibilityRole="button"
              onPress={() => router.push(`${BASE}/products/${item.uuid}` as never)}
              className="mx-4 flex-row items-center gap-3 rounded-lg border border-border bg-card p-3 active:opacity-70">
              <Thumb uri={firstImage(item)} />
              <View className="flex-1 gap-0.5">
                <Text variant="headline" numberOfLines={2}>
                  {item.name}
                </Text>
                <Text variant="footnote" tone="muted" numberOfLines={1}>
                  {item.sku} · {t(`unit_type.${item.unit_type}`)}
                </Text>
              </View>
              {canOrder ? (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={inCart ? t('in_cart') : t('add_to_cart')}
                  hitSlop={8}
                  disabled={inCart}
                  onPress={() => {
                    add(item);
                    void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
                  }}
                  className="size-9 items-center justify-center rounded-full bg-secondary active:opacity-70">
                  <Icon name={inCart ? 'check' : 'add'} tone="primary" size={18} />
                </Pressable>
              ) : null}
            </Pressable>
          );
        }}
      />
    </>
  );
}

function Gallery({ product }: { product: Product }) {
  const { width } = useWindowDimensions();
  const uris = [...product.images]
    .sort((a, b) => a.sort - b.sort)
    .map((i) => productImageUrl(i.key))
    .filter((u): u is string => !!u);
  const size = width - 32;
  if (!uris.length) {
    return (
      <View style={{ height: size * 0.6 }} className="items-center justify-center rounded-lg bg-secondary">
        <Icon name="package" tone="muted-foreground" size={48} />
      </View>
    );
  }
  return (
    <ScrollView horizontal pagingEnabled showsHorizontalScrollIndicator={false} className="rounded-lg">
      {uris.map((uri) => (
        <Image key={uri} source={{ uri }} style={{ width: size, height: size * 0.75, borderRadius: 12 }} contentFit="cover" transition={200} />
      ))}
    </ScrollView>
  );
}

export function ProductDetailScreen() {
  const { t } = useTranslation('catalog');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const canOrder = useCanOrder();
  const q = useProduct(uuid);
  const inCart = useCart((s) => s.lines.some((l) => l.product_uuid === uuid));
  const add = useCart((s) => s.add);
  const p = q.data;

  return (
    <>
      <Stack.Screen
        options={{ title: p?.name ?? '', headerLargeTitle: false, headerRight: canOrder ? () => <CartButton /> : undefined }}
      />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : !p ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : (
          <>
            <Gallery product={p} />
            <View className="gap-1">
              <Text variant="title" selectable>
                {p.name}
              </Text>
              <Text variant="callout" tone="muted" className="font-mono" selectable>
                {p.sku}
              </Text>
            </View>
            <List>
              <ListRow title={t('category')} trailing={<Text tone="muted">{p.category.name}</Text>} />
              <ListRow title={t('unit')} trailing={<Text tone="muted">{t(`unit_type.${p.unit_type}`)}</Text>} />
              {p.warranty_duration_months ? (
                <ListRow
                  title={t('warranty')}
                  trailing={<Text tone="muted">{t('warranty_months', { count: p.warranty_duration_months })}</Text>}
                />
              ) : null}
              {p.micron_thickness ? (
                <ListRow title={t('thickness')} trailing={<Text tone="muted">{`${p.micron_thickness} µm`}</Text>} />
              ) : null}
              <ListRow
                title={t('barcode')}
                trailing={<Text tone="muted">{p.uses_fixed_barcode ? t('barcode_fixed') : t('barcode_serial')}</Text>}
                last
              />
            </List>
            {p.description_md.trim() ? (
              <>
                <SectionHeader title={t('description')} />
                <Text variant="callout" selectable className="px-1">
                  {p.description_md.replace(/[#*_`>]/g, '').trim()}
                </Text>
              </>
            ) : null}
            {canOrder ? (
              <Button
                title={inCart ? t('in_cart_open') : t('add_to_cart')}
                icon={inCart ? 'cart' : 'add'}
                size="lg"
                variant={inCart ? 'secondary' : 'primary'}
                onPress={() => {
                  if (inCart) return router.push(NEW_ORDER as never);
                  add(p);
                  void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
                }}
              />
            ) : null}
          </>
        )}
      </Screen>
    </>
  );
}
