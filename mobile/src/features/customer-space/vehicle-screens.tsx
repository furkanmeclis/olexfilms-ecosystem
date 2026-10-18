import { useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { FlatList, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { formatDate, formatNumber, vehicleLabel } from '@/features/services/lib';
import { DetailSkeleton, ErrorState, InfoRow, ListBody, ListFooter, Section } from '@/features/services/ui';
import { Icon } from '@/ui/icon';
import { List, ListRow } from '@/ui/list';
import { Text } from '@/ui/text';

import { PAGE_SIZE, retryPortal, useCustomerLinks, useFleetRoles, usePortalPages } from './lib';
import { ActiveWarrantyRow, PortalServiceRow, VehicleCard } from './parts';
import { useTransferLink } from './transfer-screens';

/** The customer's vehicles (GET /v1/portal/vehicles), infinite scroll. */
export function CustomerVehiclesScreen() {
  const { t } = useTranslation('customer');
  const links = useCustomerLinks();
  const list = usePortalPages(['portal', 'vehicles', 'list'], (offset) =>
    unwrap(api.GET('/v1/portal/vehicles', { params: { query: { limit: PAGE_SIZE, offset } } })),
  );
  return (
    <>
      <Stack.Screen options={{ title: t('vehicles') }} />
      <FlatList
        data={list.items}
        keyExtractor={(v) => v.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10 pt-2"
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={{ icon: 'vehicle', title: t('vehicles_empty'), body: t('vehicles_empty_body') }}
          />
        }
        renderItem={({ item }) => <VehicleCard v={item} onPress={() => router.push(links.vehicle(item.uuid))} />}
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

/** One vehicle: identity, service summary, active warranties and service history. */
export function CustomerVehicleDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('customer');
  const links = useCustomerLinks();
  const transferLink = useTransferLink();
  const { readOnly } = useFleetRoles();
  const q = useQuery({
    queryKey: ['portal', 'vehicles', 'detail', uuid],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/vehicles/{uuid}', { params: { path: { uuid } } })),
  });
  const v = q.data;
  const label = v ? vehicleLabel(v.car_brand?.name, v.car_model?.name, v.model_year) : '';
  const services = v?.services ?? [];
  const warranties = v?.active_warranties ?? [];

  return (
    <>
      <Stack.Screen options={{ title: v?.plate || label || t('vehicle'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {!v ? (
          q.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          )
        ) : (
          <>
            <View className="flex-row items-center gap-3 rounded-lg border border-border bg-card p-4">
              <View className="size-12 items-center justify-center rounded-md bg-secondary">
                <Icon name="vehicle" tone="primary" size={24} />
              </View>
              <View className="flex-1 gap-0.5">
                <Text variant="title" selectable numberOfLines={1}>
                  {v.plate || label}
                </Text>
                {v.plate && label ? <Text variant="callout">{label}</Text> : null}
              </View>
            </View>

            <Section title={t('details')}>
              <InfoRow label={t('plate')} value={v.plate} />
              <InfoRow label={t('plate_country')} value={v.plate_country} />
              <InfoRow label={t('vin')} value={v.vin} mono />
              <InfoRow label={t('service_total')} value={formatNumber(v.service_summary?.total ?? v.service_count ?? 0, i18n.language)} />
              <InfoRow
                label={t('service_completed')}
                value={v.service_summary ? formatNumber(v.service_summary.completed ?? 0, i18n.language) : null}
              />
              <InfoRow
                label={t('last_service_at')}
                value={formatDate(v.service_summary?.last_service_at ?? v.last_service_at, i18n.language)}
                last
              />
            </Section>

            {readOnly ? null : (
              <List>
                <ListRow
                  icon="transfer"
                  title={t('transfer_button')}
                  subtitle={t('transfer_button_hint')}
                  onPress={() => router.push(transferLink(v.uuid))}
                  last
                />
              </List>
            )}

            <View className="gap-2">
              <Text variant="caption" tone="muted" className="px-1">
                {t('active_warranties')}
              </Text>
              {warranties.length > 0 ? (
                warranties.map((w) => (
                  <ActiveWarrantyRow key={w.uuid} w={w} onPress={() => router.push(links.warranty(w.uuid))} />
                ))
              ) : (
                <View className="flex-row items-center gap-3 rounded-lg border border-border bg-card p-4">
                  <Icon name="warranty" tone="muted-foreground" size={18} />
                  <Text variant="callout" tone="muted" className="flex-1">
                    {t('no_active_warranties')}
                  </Text>
                </View>
              )}
            </View>

            <View className="gap-2">
              <Text variant="caption" tone="muted" className="px-1">
                {t('service_history')}
              </Text>
              {services.length > 0 ? (
                services.map((s) => (
                  <PortalServiceRow key={s.uuid} s={s} onPress={() => router.push(links.service(s.uuid))} />
                ))
              ) : (
                <View className="flex-row items-center gap-3 rounded-lg border border-border bg-card p-4">
                  <Icon name="service" tone="muted-foreground" size={18} />
                  <Text variant="callout" tone="muted" className="flex-1">
                    {t('services_empty_body')}
                  </Text>
                </View>
              )}
            </View>
          </>
        )}
      </ScrollView>
    </>
  );
}
