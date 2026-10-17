import { useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Linking, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useCan } from '@/core/spaces';
import { formatDate, fullName, useLinks, vehicleLabel } from '@/features/services/lib';
import { ServiceRow } from '@/features/services/services-list-screen';
import { DetailSkeleton, ErrorState, InfoRow, ListSkeleton, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { Mail, Phone } from '@/ui/icons';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

/**
 * Contact, vehicles and recent services of one customer. Vehicles and services
 * are separate list calls (the customer detail does not embed them).
 */
export function CustomerDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('dealer');
  const links = useLinks();
  const canVehicles = useCan('vehicles.read');
  const canServices = useCan('services.read');

  const customer = useQuery({
    queryKey: ['customers', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/customers/{uuid}', { params: { path: { uuid } } })),
  });
  const vehicles = useQuery({
    queryKey: ['vehicles', 'of-customer', uuid],
    queryFn: () => unwrap(api.GET('/v1/vehicles', { params: { query: { customer_uuid: uuid, limit: 50, offset: 0 } } })),
    enabled: canVehicles,
  });
  const services = useQuery({
    queryKey: ['services', 'list', { customer: uuid }],
    queryFn: () =>
      unwrap(api.GET('/v1/services', { params: { query: { customer_uuid: uuid, limit: 10, offset: 0 } } })),
    enabled: canServices,
  });

  const c = customer.data;
  const refresh = () => {
    void customer.refetch();
    if (canVehicles) void vehicles.refetch();
    if (canServices) void services.refetch();
  };

  return (
    <>
      <Stack.Screen options={{ title: c ? fullName(c) : t('customer'), headerLargeTitle: false }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<RefreshControl refreshing={customer.isRefetching} onRefresh={refresh} />}>
        {!c ? (
          customer.isPending ? (
            <DetailSkeleton />
          ) : (
            <ErrorState error={customer.error} onRetry={() => void customer.refetch()} />
          )
        ) : (
          <>
            <View className="gap-3 rounded-lg border border-border bg-card p-4">
              <View className="gap-0.5">
                <Text variant="title">{fullName(c)}</Text>
                {c.company_name ? (
                  <Text variant="callout" tone="muted">
                    {c.company_name}
                  </Text>
                ) : null}
                <Text variant="footnote" tone="muted">
                  {t(`customer_type.${c.type}`)} · {t(`customer_status.${c.status}`)}
                </Text>
              </View>
              {c.phone || c.email ? (
                <View className="flex-row gap-2">
                  {c.phone ? (
                    <Button
                      title={t('call')}
                      icon={<Icon as={Phone} tone="primary-foreground" size={16} />}
                      onPress={() => void Linking.openURL(`tel:${c.phone}`)}
                      className="grow"
                    />
                  ) : null}
                  {c.email ? (
                    <Button
                      title={t('email')}
                      variant="secondary"
                      icon={<Icon as={Mail} tone="primary" size={16} />}
                      onPress={() => void Linking.openURL(`mailto:${c.email}`)}
                      className="grow"
                    />
                  ) : null}
                </View>
              ) : null}
            </View>

            <Section title={t('contact')}>
              <InfoRow
                label={t('phone')}
                value={c.phone}
                onPress={c.phone ? () => void Linking.openURL(`tel:${c.phone}`) : undefined}
              />
              <InfoRow label={t('email')} value={c.email} />
              <InfoRow label={t('tax_office')} value={c.tax_office} />
              <InfoRow label={t('tax_no')} value={c.tax_no_last4 ? `•••• ${c.tax_no_last4}` : null} />
              <InfoRow label={t('national_id')} value={c.national_id_last4 ? `•••• ${c.national_id_last4}` : null} />
              <InfoRow label={t('first_service')} value={formatDate(c.first_service_at, i18n.language)} />
              <InfoRow label={t('customer_since')} value={formatDate(c.linked_at ?? c.created_at, i18n.language)} last />
            </Section>

            {canVehicles ? (
              <Section title={t('vehicles')}>
                {vehicles.isPending ? (
                  <ListSkeleton rows={1} />
                ) : (vehicles.data?.items.length ?? 0) > 0 ? (
                  vehicles.data!.items.map((v, i, arr) => (
                    <InfoRow
                      key={v.uuid}
                      label={v.plate || '—'}
                      value={vehicleLabel(v.car_brand?.name, v.car_model?.name, v.model_year) || v.vin || '—'}
                      last={i === arr.length - 1}
                    />
                  ))
                ) : (
                  <Text variant="callout" tone="muted" className="px-4 py-3">
                    {t('vehicles_empty')}
                  </Text>
                )}
              </Section>
            ) : null}

            {canServices ? (
              <View className="gap-2">
                <Text variant="caption" tone="muted" className="px-1">
                  {t('services_title')}
                </Text>
                {services.isPending ? (
                  <ListSkeleton rows={2} />
                ) : (services.data?.items.length ?? 0) > 0 ? (
                  services.data!.items.map((s) => (
                    <ServiceRow key={s.uuid} s={s} onPress={() => router.push(links.service(s.uuid))} />
                  ))
                ) : (
                  <Text variant="callout" tone="muted" className="px-1">
                    {t('services_empty')}
                  </Text>
                )}
              </View>
            ) : null}
          </>
        )}
      </ScrollView>
    </>
  );
}
