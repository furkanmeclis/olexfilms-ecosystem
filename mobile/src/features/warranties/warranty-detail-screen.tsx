import { useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import {
  formatDate,
  fullName,
  publicWarrantyUrl,
  useLinks,
  vehicleLabel,
  warrantyTone,
} from '@/features/services/lib';
import { QrImage } from '@/features/services/qr-sheet';
import { openWarranty, shareWarranty } from '@/features/services/share';
import { DetailSkeleton, ErrorState, InfoRow, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { ExternalLink, Share2 } from '@/ui/icons';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

/** One warranty: coverage, product, vehicle, holder and the public page link. */
export function WarrantyDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('dealer');
  const links = useLinks();
  const q = useQuery({
    queryKey: ['warranties', 'detail', uuid],
    queryFn: () => unwrap(api.GET('/v1/warranties/{uuid}', { params: { path: { uuid } } })),
  });
  const w = q.data;

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
              <Text variant="callout">{w.product.name}</Text>
              <View className="items-center gap-2 py-2">
                <View className="overflow-hidden rounded-lg">
                  <QrImage value={publicWarrantyUrl(w.public_code)} size={200} />
                </View>
                <Text variant="footnote" tone="muted" selectable numberOfLines={1}>
                  {publicWarrantyUrl(w.public_code)}
                </Text>
              </View>
              <View className="flex-row gap-2">
                <Button
                  title={t('warranty_open')}
                  icon={<Icon as={ExternalLink} tone="primary-foreground" size={16} />}
                  onPress={() => void openWarranty(w.public_code)}
                  className="grow"
                />
                <Button
                  title={t('share')}
                  variant="secondary"
                  icon={<Icon as={Share2} tone="primary" size={16} />}
                  onPress={() => shareWarranty(w.public_code, t('warranty_share_message', { code: w.public_code }))}
                  className="grow"
                />
              </View>
            </View>

            <Section title={t('coverage')}>
              <InfoRow label={t('start')} value={formatDate(w.start_at, i18n.language)} />
              <InfoRow label={t('end')} value={formatDate(w.end_at, i18n.language)} />
              <InfoRow label={t('item_kind')} value={t(`kind_${w.item_kind}`)} />
              <InfoRow label={t('voided_at')} value={formatDate(w.voided_at, i18n.language)} />
              <InfoRow label={t('void_reason')} value={w.void_reason} last />
            </Section>

            <Section title={t('vehicle')}>
              <InfoRow label={t('plate')} value={w.vehicle.plate} />
              <InfoRow
                label={t('vehicle')}
                value={vehicleLabel(w.vehicle.brand_name, w.vehicle.model_name, w.vehicle.model_year)}
              />
              <InfoRow label={t('sku')} value={w.product.sku} last />
            </Section>

            <Section title={t('service')}>
              <InfoRow
                label={t('service_no')}
                value={w.service.service_no}
                onPress={() => router.push(links.service(w.service.uuid))}
              />
              {w.holder ? (
                <InfoRow
                  label={t('customer')}
                  value={fullName(w.holder)}
                  onPress={w.holder.anonymized ? undefined : () => router.push(links.customer(w.holder!.uuid))}
                />
              ) : null}
              <InfoRow label={t('organization')} value={w.organization.name} last />
            </Section>
          </>
        )}
      </ScrollView>
    </>
  );
}
