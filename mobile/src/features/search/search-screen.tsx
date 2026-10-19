import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { type Href, router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Alert, Pressable, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';
import { type SpaceKind, useResolvedSpace } from '@/core/spaces';
import { readJSON, writeJSON } from '@/core/storage';
import { compactPlate } from '@/features/plate-scan/lib';
import { PlateScanButton } from '@/features/plate-scan/plate-scan-button';
import { useDebounced } from '@/features/services/lib';
import { errorText, SearchField } from '@/features/services/ui';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';
import type { IconName } from '@/ui/icons';

type Hit = components['schemas']['SearchHit'];

const RECENT_KEY = 'search.recent';
const MAX_RECENT = 8;

const RESULT_ICONS: Record<string, IconName> = {
  customers: 'customers',
  vehicles: 'vehicle',
  services: 'service',
  warranties: 'warranty',
  orders: 'cart',
  organizations: 'building',
  stock_units: 'barcode',
  leads: 'target',
  products: 'package',
};

/** Detail route of a hit inside the space the user is in; null when the app has no screen for it. */
function hrefFor(space: SpaceKind | null, spec: string, id: string): Href | null {
  if (space === 'bayi') {
    if (spec === 'services') return `/bayi/services/${id}` as Href;
    if (spec === 'customers') return `/bayi/menu/customers/${id}` as Href;
    if (spec === 'warranties') return `/bayi/menu/warranties/${id}` as Href;
    if (spec === 'orders') return `/bayi/menu/orders/${id}` as Href;
  }
  if (space === 'admin') {
    if (spec === 'services') return `/admin/menu/customers/services/${id}` as Href;
    if (spec === 'customers') return `/admin/menu/customers/${id}` as Href;
    if (spec === 'warranties') return `/admin/menu/customers/warranties/${id}` as Href;
    if (spec === 'orders') return `/admin/menu/orders/${id}` as Href;
  }
  return null;
}

function remember(q: string) {
  const list = (readJSON<string[]>(RECENT_KEY) ?? []).filter((x) => x.toLowerCase() !== q.toLowerCase());
  writeJSON(RECENT_KEY, [q, ...list].slice(0, MAX_RECENT));
}

/**
 * Global search: plate, phone, name, service no, warranty code in one box.
 * Hits are grouped like the web's Cmd+K and open the detail screens of the
 * current space (the modal closes first).
 */
export function SearchScreen() {
  const { t } = useTranslation('search');
  const space = useResolvedSpace();
  const [q, setQ] = useState('');
  const [recent, setRecent] = useState<string[]>(() => readJSON<string[]>(RECENT_KEY) ?? []);
  const [opening, setOpening] = useState<string | null>(null);
  const query = useDebounced(q.trim(), 350);
  const active = query.length >= 2;

  const res = useQuery({
    queryKey: ['search', 'global', query],
    queryFn: () => unwrap(api.GET('/v1/search/global', { params: { query: { q: query, limit: 5 } } })),
    enabled: active,
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });

  const go = (href: Href) => {
    remember(query);
    setRecent(readJSON<string[]>(RECENT_KEY) ?? []);
    router.back();
    router.navigate(href);
  };

  const open = async (hit: Hit) => {
    if (hit.spec === 'vehicles') {
      // A vehicle has no screen of its own: open its owner.
      setOpening(hit.id);
      try {
        const v = await unwrap(api.GET('/v1/vehicles/{uuid}', { params: { path: { uuid: hit.id } } }));
        const href = hrefFor(space, 'customers', v.customer_uuid);
        if (href) go(href);
      } catch (e) {
        Alert.alert(t('open_failed'), errorText(e));
      } finally {
        setOpening(null);
      }
      return;
    }
    const href = hrefFor(space, hit.spec, hit.id);
    if (href) go(href);
  };

  const groups = (res.data?.groups ?? []).filter((g) => g.items.length > 0);
  const openable = (spec: string) => spec === 'vehicles' ? !!hrefFor(space, 'customers', 'x') : !!hrefFor(space, spec, 'x');

  return (
    <>
      <Stack.Screen options={{ title: t('title') }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-3">
        <View className="flex-row items-center gap-2">
          <View className="flex-1">
            <SearchField value={q} onChange={setQ} placeholder={t('placeholder')} autoFocus />
          </View>
          <PlateScanButton compact onResult={(plate) => setQ(compactPlate(plate))} />
        </View>

        {!active ? (
          recent.length > 0 ? (
            <View className="gap-1.5">
              <View className="flex-row items-center justify-between px-1">
                <Text variant="caption" tone="muted">
                  {t('recent')}
                </Text>
                <Pressable
                  accessibilityRole="button"
                  hitSlop={8}
                  onPress={() => {
                    writeJSON(RECENT_KEY, []);
                    setRecent([]);
                  }}>
                  <Text variant="footnote" tone="primary">
                    {t('clear')}
                  </Text>
                </Pressable>
              </View>
              <View className="overflow-hidden rounded-lg border border-border bg-card">
                {recent.map((r, i) => (
                  <Pressable
                    key={r}
                    accessibilityRole="button"
                    onPress={() => setQ(r)}
                    className={`min-h-11 flex-row items-center gap-3 px-4 py-2.5 active:bg-muted ${i < recent.length - 1 ? 'border-b border-border' : ''}`}>
                    <Icon name="clock" tone="muted-foreground" size={16} />
                    <Text variant="body" numberOfLines={1} className="flex-1">
                      {r}
                    </Text>
                  </Pressable>
                ))}
              </View>
            </View>
          ) : (
            <EmptyState icon="search" title={t('hint_title')} body={t('hint_body')} />
          )
        ) : res.isPending ? (
          <ActivityIndicator className="py-8" />
        ) : res.error ? (
          <EmptyState icon="search" title={t('common:error_generic')} body={errorText(res.error)} />
        ) : res.data && !res.data.enabled ? (
          <EmptyState icon="search" title={t('disabled')} />
        ) : groups.length === 0 ? (
          <EmptyState icon="search" title={t('no_results')} body={t('no_results_body')} />
        ) : (
          groups.map((g) => {
            const canOpen = openable(g.spec);
            return (
              <View key={g.spec} className="gap-1.5">
                <Text variant="caption" tone="muted" className="px-1">
                  {t(`group.${g.spec}`, { defaultValue: g.spec })}
                </Text>
                <View className="overflow-hidden rounded-lg border border-border bg-card">
                  {g.items.map((h, i) => (
                    <Pressable
                      key={h.id}
                      accessibilityRole="button"
                      disabled={!canOpen || opening !== null}
                      onPress={() => void open(h)}
                      className={`min-h-12 flex-row items-center gap-3 px-4 py-2.5 active:bg-muted ${i < g.items.length - 1 ? 'border-b border-border' : ''}`}>
                      <Icon name={RESULT_ICONS[g.spec] ?? 'search'} tone={canOpen ? 'primary' : 'muted-foreground'} size={18} />
                      <View className="flex-1 gap-0.5">
                        <Text variant="body" numberOfLines={1}>
                          {h.title}
                        </Text>
                        {h.subtitle ? (
                          <Text variant="footnote" tone="muted" numberOfLines={1}>
                            {h.subtitle}
                          </Text>
                        ) : null}
                      </View>
                      {opening === h.id ? <ActivityIndicator /> : null}
                    </Pressable>
                  ))}
                </View>
              </View>
            );
          })
        )}
      </ScrollView>
    </>
  );
}
