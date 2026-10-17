import { useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Linking, Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDate, useDebounced, vehicleLabel, warrantyTone } from '@/features/services/lib';
import {
  DetailSkeleton,
  ErrorState,
  InfoRow,
  ListBody,
  ListFooter,
  SearchField,
  Section,
} from '@/features/services/ui';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { PAGE_SIZE, portalServiceTone, retryPortal, useCustomerLinks, usePortalPages, whatsappUrl } from './lib';
import { PortalServiceRow, useServiceStatusLabel } from './parts';

/** Every service of the customer across dealers (GET /v1/portal/services). */
export function CustomerServicesScreen() {
  const { t } = useTranslation('customer');
  const links = useCustomerLinks();
  const [q, setQ] = useState('');
  const query = useDebounced(q.trim());
  const list = usePortalPages(['portal', 'services', 'list', { q: query }], (offset) =>
    unwrap(api.GET('/v1/portal/services', { params: { query: { q: query || undefined, limit: PAGE_SIZE, offset } } })),
  );
  return (
    <>
      <Stack.Screen options={{ title: t('services') }} />
      <FlatList
        data={list.items}
        keyExtractor={(s) => s.uuid}
        contentInsetAdjustmentBehavior="automatic"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10"
        ListHeaderComponent={
          <View className="pb-1 pt-2">
            <SearchField value={q} onChange={setQ} placeholder={t('services_search')} />
          </View>
        }
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{
              icon: 'service',
              title: query ? t('services_empty_filtered') : t('services_empty'),
              body: query ? undefined : t('services_empty_body'),
            }}
          />
        }
        renderItem={({ item }) => <PortalServiceRow s={item} onPress={() => router.push(links.service(item.uuid))} />}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage();
        }}
        ListFooterComponent={<ListFooter loading={list.isFetchingNextPage} />}
        refreshControl={
          <RefreshControl refreshing={list.isRefetching && !list.isFetchingNextPage} onRefresh={() => void list.refetch()} />
        }
      />
    </>
  );
}

/**
 * One service: status, vehicle, dealer, applied products and the warranties
 * it issued. The service PDF is not offered: its download needs the bearer
 * token, and Expo Go has no file module to save it (see the report).
 */
export function CustomerServiceDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('customer');
  const links = useCustomerLinks();
  const statusLabel = useServiceStatusLabel();
  const q = useQuery({
    queryKey: ['portal', 'services', 'detail', uuid],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/services/{uuid}', { params: { path: { uuid } } })),
  });
  const s = q.data;
  const dealer = s?.dealer;
  const products = s?.products ?? [];
  const warranties = s?.warranties ?? [];

  return (
    <>
      <Stack.Screen options={{ title: s?.service_no ?? t('service'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!s ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <View className="gap-2 rounded-lg border border-border bg-card p-4">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="title" selectable className="flex-1" numberOfLines={1}>
                  {s.service_no}
                </Text>
                <StatusPill label={statusLabel(s.status, s.status_label)} tone={portalServiceTone(s.status)} />
              </View>
              <Text variant="footnote" tone="muted">
                {[
                  t('created_on', { date: formatDate(s.created_at, i18n.language) }),
                  s.completed_at ? t('completed_on', { date: formatDate(s.completed_at, i18n.language) }) : null,
                ]
                  .filter(Boolean)
                  .join(' · ')}
              </Text>
            </View>

            <Section title={t('vehicle')}>
              <InfoRow
                label={t('plate')}
                value={s.plate || '—'}
                onPress={s.vehicle_uuid ? () => router.push(links.vehicle(s.vehicle_uuid)) : undefined}
              />
              <InfoRow
                label={t('vehicle')}
                value={vehicleLabel(s.car_brand?.name, s.car_model?.name, s.model_year)}
                last
              />
            </Section>

            {dealer ? (
              <Section title={t('dealer')}>
                <InfoRow label={t('dealer')} value={dealer.name} />
                <InfoRow label={t('city')} value={[dealer.district, dealer.city].filter(Boolean).join(', ')} />
                <InfoRow label={t('address')} value={dealer.address} />
                <InfoRow
                  label={t('whatsapp')}
                  value={dealer.whatsapp}
                  onPress={dealer.whatsapp ? () => void Linking.openURL(whatsappUrl(dealer.whatsapp!)) : undefined}
                  last
                />
              </Section>
            ) : null}

            {products.length > 0 ? (
              <Section title={t('products')}>
                {products.map((p, i) => (
                  <View
                    key={p.service_item_uuid ?? i}
                    className={`gap-0.5 px-4 py-3 ${i < products.length - 1 ? 'border-b border-border' : ''}`}>
                    <Text variant="callout" className="font-semibold">
                      {p.name}
                    </Text>
                    <Text variant="footnote" tone="muted">
                      {[p.category, (p.applied_parts ?? []).join(', ')].filter(Boolean).join(' · ')}
                    </Text>
                  </View>
                ))}
              </Section>
            ) : (s.applied_parts?.length ?? 0) > 0 ? (
              <Section title={t('applied_parts')}>
                <View className="px-4 py-3">
                  <Text variant="callout">{s.applied_parts.join(', ')}</Text>
                </View>
              </Section>
            ) : null}

            {warranties.length > 0 ? (
              <Section title={t('warranties')}>
                {warranties.map((w, i) => (
                  <Pressable
                    key={w.uuid}
                    accessibilityRole="button"
                    onPress={() => router.push(links.warranty(w.uuid))}
                    className={`flex-row items-center gap-3 px-4 py-3 active:bg-muted ${
                      i < warranties.length - 1 ? 'border-b border-border' : ''
                    }`}>
                    <View className="flex-1 gap-0.5">
                      <Text variant="callout" className="font-semibold" numberOfLines={1}>
                        {w.product_name || w.public_code}
                      </Text>
                      <Text variant="footnote" tone="muted" numberOfLines={1}>
                        {[w.public_code, t('until', { date: formatDate(w.end_at, i18n.language) })].join(' · ')}
                      </Text>
                    </View>
                    <StatusPill label={t(`warranty_status.${w.status}`)} tone={warrantyTone(w.status)} />
                    <Icon name="forward" tone="muted-foreground" size={18} />
                  </Pressable>
                ))}
              </Section>
            ) : null}
          </>
        )}
      </ScrollView>
    </>
  );
}
