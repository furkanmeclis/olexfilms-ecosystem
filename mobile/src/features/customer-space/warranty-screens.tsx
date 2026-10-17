import { useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import {
  formatDate,
  publicWarrantyUrl,
  vehicleLabel,
  WARRANTY_STATUSES,
  type WarrantyStatus,
  warrantyTone,
} from '@/features/services/lib';
import { Chips, DetailSkeleton, ErrorState, InfoRow, ListBody, ListFooter, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  daysLeft,
  openPublicWarranty,
  PAGE_SIZE,
  retryPortal,
  sharePublicWarranty,
  useCustomerLinks,
  usePortalPages,
} from './lib';
import { WarrantyRow } from './parts';

type Filter = 'all' | WarrantyStatus;

/** The customer's warranties (GET /v1/portal/warranties) with status chips. */
export function CustomerWarrantiesScreen() {
  const { t } = useTranslation('customer');
  const links = useCustomerLinks();
  const [filter, setFilter] = useState<Filter>('all');
  const list = usePortalPages(['portal', 'warranties', 'list', { filter }], (offset) =>
    unwrap(
      api.GET('/v1/portal/warranties', {
        params: { query: { status: filter === 'all' ? undefined : filter, limit: PAGE_SIZE, offset } },
      }),
    ),
  );
  const chips = [
    { value: 'all' as Filter, label: t('filter_all') },
    ...WARRANTY_STATUSES.map((s) => ({ value: s as Filter, label: t(`warranty_status.${s}`) })),
  ];
  return (
    <>
      <Stack.Screen options={{ title: t('warranties') }} />
      <FlatList
        data={list.items}
        keyExtractor={(w) => w.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10"
        ListHeaderComponent={
          <View className="pb-1 pt-2">
            <Chips options={chips} value={filter} onChange={setFilter} />
          </View>
        }
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{
              icon: 'warranty',
              title: filter === 'all' ? t('warranties_empty') : t('warranties_empty_filtered'),
              body: filter === 'all' ? t('warranties_empty_body') : undefined,
            }}
          />
        }
        renderItem={({ item }) => <WarrantyRow w={item} onPress={() => router.push(links.warranty(item.uuid))} />}
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

/** One warranty: coverage, vehicle, service and the public certificate page (open / share). */
export function CustomerWarrantyDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('customer');
  const links = useCustomerLinks();
  const q = useQuery({
    queryKey: ['portal', 'warranties', 'detail', uuid],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/warranties/{uuid}', { params: { path: { uuid } } })),
  });
  const w = q.data;
  const left = w?.status === 'active' ? daysLeft(w.end_at) : null;

  return (
    <>
      <Stack.Screen options={{ title: w?.public_code ?? t('warranty'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!w ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <View className="gap-3 rounded-lg border border-border bg-card p-4">
              <View className="flex-row items-center justify-between gap-3">
                <Text variant="title" selectable className="flex-1">
                  {w.public_code}
                </Text>
                <StatusPill label={t(`warranty_status.${w.status}`)} tone={warrantyTone(w.status)} />
              </View>
              {w.product?.name ? <Text variant="callout">{w.product.name}</Text> : null}
              {left !== null && left >= 0 ? (
                <Text variant="footnote" tone="muted">
                  {t('days_left', { count: left })}
                </Text>
              ) : null}
              <Text variant="footnote" tone="muted" selectable numberOfLines={1}>
                {publicWarrantyUrl(w.public_code)}
              </Text>
              <View className="flex-row gap-2">
                <Button
                  title={t('warranty_open')}
                  icon={<Icon name="externalLink" tone="primary-foreground" size={16} />}
                  onPress={() => void openPublicWarranty(w.public_code)}
                  className="grow"
                />
                <Button
                  title={t('share')}
                  variant="secondary"
                  icon={<Icon name="share" tone="primary" size={16} />}
                  onPress={() => sharePublicWarranty(w.public_code, t('warranty_share_message', { code: w.public_code }))}
                  className="grow"
                />
              </View>
            </View>

            <Section title={t('coverage')}>
              <InfoRow label={t('start')} value={formatDate(w.start_at, i18n.language)} />
              <InfoRow label={t('end')} value={formatDate(w.end_at, i18n.language)} />
              <InfoRow label={t('item_kind')} value={w.item_kind ? t(`kind_${w.item_kind}`) : null} />
              <InfoRow label={t('voided_at')} value={formatDate(w.voided_at, i18n.language)} />
              <InfoRow label={t('void_reason')} value={w.void_reason} last />
            </Section>

            {w.vehicle ? (
              <Section title={t('vehicle')}>
                <InfoRow
                  label={t('plate')}
                  value={w.vehicle.plate || '—'}
                  onPress={w.vehicle.uuid ? () => router.push(links.vehicle(w.vehicle.uuid)) : undefined}
                />
                <InfoRow
                  label={t('vehicle')}
                  value={vehicleLabel(w.vehicle.brand_name, w.vehicle.model_name, w.vehicle.model_year)}
                  last
                />
              </Section>
            ) : null}

            <Section title={t('service')}>
              <InfoRow
                label={t('service_no')}
                value={w.service?.service_no}
                onPress={w.service?.uuid ? () => router.push(links.service(w.service.uuid)) : undefined}
              />
              <InfoRow label={t('dealer')} value={w.organization?.name} />
              <InfoRow label={t('sku')} value={w.product?.sku} last />
            </Section>

            {(w.claims?.length ?? 0) > 0 ? (
              <Section title={t('claims')}>
                {w.claims!.map((c, i) => (
                  <InfoRow
                    key={c.uuid}
                    label={formatDate(c.created_at, i18n.language)}
                    value={t(`claim_status.${c.status}`, { defaultValue: String(c.status) })}
                    last={i === w.claims!.length - 1}
                  />
                ))}
              </Section>
            ) : null}
          </>
        )}
      </ScrollView>
    </>
  );
}
