import { useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, Linking, Pressable, ScrollView, View } from 'react-native';
import { useModule } from '@/core/modules';

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
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { PullRefresh } from '@/ui/pull-refresh';
import { usePartLabel, VehiclePartPicker } from '@/features/vehicle-parts';

import { portalFiles, useExportDownload } from './files';
import { useTimeLeft } from './home-parts';
import {
  daysLeft,
  openPublicWarranty,
  PAGE_SIZE,
  type PortalService,
  portalServiceTone,
  retryPortal,
  sharePublicWarranty,
  useCustomerLinks,
  useFleetRoles,
  usePortalPages,
  whatsappUrl,
} from './lib';
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
          <PullRefresh busy={list.isRefetching && !list.isFetchingNextPage} onRefresh={() => void list.refetch()} />
        }
      />
    </>
  );
}

/**
 * One service: status, vehicle, dealer, applied products and the warranties
 * it issued. The service PDF and warranty certificate are downloaded with the
 * bearer token and shared; a completed service links to its review.
 */
export function CustomerServiceDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t, i18n } = useTranslation('customer');
  const reviewsOn = useModule('reviews');
  const fleetReadOnly = useFleetRoles().readOnly;
  const links = useCustomerLinks();
  const statusLabel = useServiceStatusLabel();
  const q = useQuery({
    queryKey: ['portal', 'services', 'detail', uuid],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/services/{uuid}', { params: { path: { uuid } } })),
  });
  const s = q.data;
  const fileJob = useExportDownload();
  const fileBusy = fileJob.isPending ? fileJob.variables?.tag : undefined;
  const dealer = s?.dealer;
  const products = s?.products ?? [];
  const warranties = s?.warranties ?? [];
  const partLabel = usePartLabel();
  const [openProduct, setOpenProduct] = useState<string | null>(null);
  // The merged list comes from the backend; when empty, the parts of the product rows.
  const appliedParts = s?.applied_parts?.length
    ? s.applied_parts
    : products.flatMap((p) => p.applied_parts ?? []);

  return (
    <>
      <Stack.Screen options={{ title: s?.service_no ?? t('service'), headerLargeTitle: false, headerBlurEffect: 'systemChromeMaterial' }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={<PullRefresh busy={q.isRefetching} onRefresh={() => void q.refetch()} />}>
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

            <View className="gap-2">
              <Button
                title={t('service_pdf')}
                variant="secondary"
                loading={fileBusy === 'pdf'}
                disabled={fileJob.isPending}
                icon={<Icon name="download" tone="primary" size={18} />}
                onPress={() => fileJob.mutate({ start: portalFiles.servicePdf(s.uuid), fallbackName: `${s.service_no}.pdf`, tag: 'pdf' })}
              />
              {warranties.some((w) => w.status === 'active') ? (
                <Button
                  title={t('warranty_certificate')}
                  variant="secondary"
                  loading={fileBusy === 'cert'}
                  disabled={fileJob.isPending}
                  icon={<Icon name="warranty" tone="primary" size={18} />}
                  onPress={() =>
                    fileJob.mutate({
                      start: portalFiles.warrantyCertificate(s.uuid),
                      fallbackName: `${s.service_no}-warranty.pdf`,
                      tag: 'cert',
                    })
                  }
                />
              ) : null}
              {/* A fleet-only account cannot send a review (the form refuses it): no dead end. */}
              {s.status === 'completed' && reviewsOn && !fleetReadOnly ? (
                <Button
                  title={t('review_service')}
                  variant="outline"
                  icon={<Icon name="star" tone="primary" size={18} />}
                  onPress={() => router.push(links.review(s.uuid))}
                />
              ) : null}
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
                {products.map((p, i) => {
                  const key = p.service_item_uuid ?? String(i);
                  const parts = p.applied_parts ?? [];
                  const open = openProduct === key;
                  return (
                    <View key={key} className={i < products.length - 1 ? 'border-b border-border' : ''}>
                      {/* A product row opens the diagram of its own parts. */}
                      <Pressable
                        accessibilityRole="button"
                        accessibilityState={{ expanded: open }}
                        accessibilityHint={parts.map(partLabel).join(', ') || undefined}
                        disabled={!parts.length}
                        onPress={() => setOpenProduct(open ? null : key)}
                        className="flex-row items-center gap-3 px-4 py-3 active:bg-muted">
                        <View className="flex-1 gap-0.5">
                          <Text variant="callout" className="font-semibold">
                            {p.name}
                          </Text>
                          <Text variant="footnote" tone="muted">
                            {[p.category, parts.length ? t('dealer:parts_count', { count: parts.length }) : null]
                              .filter(Boolean)
                              .join(' · ')}
                          </Text>
                        </View>
                        {parts.length ? <Icon name={open ? 'chevronUp' : 'chevronDown'} tone="muted-foreground" size={16} /> : null}
                      </Pressable>
                      {open ? <VehiclePartPicker value={parts} readOnly framed={false} className="px-3 pb-3" /> : null}
                    </View>
                  );
                })}
              </Section>
            ) : null}

            {appliedParts.length > 0 ? (
              <Section title={t('applied_parts')}>
                <VehiclePartPicker value={appliedParts} readOnly framed={false} className="p-3" />
              </Section>
            ) : null}

            {warranties.length > 0 ? (
              <Section title={t('warranties')}>
                {warranties.map((w, i) => (
                  <ServiceWarrantyRow key={w.uuid} w={w} last={i === warranties.length - 1} />
                ))}
              </Section>
            ) : null}
          </>
        )}
      </ScrollView>
    </>
  );
}

/**
 * One warranty of the service. The customer has no warranty screen, so the row carries what
 * that screen had: code, period, days left, kind, void date, and the public certificate page
 * (open / share). It does not navigate; the service detail is already the warranty's home.
 */
function ServiceWarrantyRow({ w, last }: { w: PortalService['warranties'][number]; last: boolean }) {
  const { t, i18n } = useTranslation('customer');
  const timeLeft = useTimeLeft();
  const left = w.status === 'active' ? daysLeft(w.end_at) : null;
  const period = [formatDate(w.start_at, i18n.language), formatDate(w.end_at, i18n.language)].filter(Boolean).join(' – ');
  const kind = w.item_kind === 'full' || w.item_kind === 'partial' ? t(`kind_${w.item_kind}`) : null;
  return (
    <View className={`gap-2 px-4 py-3 ${last ? '' : 'border-b border-border'}`}>
      <View className="flex-row items-center gap-3">
        <View className="flex-1 gap-0.5">
          <Text variant="callout" className="font-semibold" numberOfLines={1}>
            {w.product_name || w.public_code}
          </Text>
          <Text variant="footnote" tone="muted" selectable numberOfLines={1}>
            {[w.public_code, kind].filter(Boolean).join(' · ')}
          </Text>
        </View>
        <StatusPill label={t(`warranty_status.${w.status}`)} tone={warrantyTone(w.status)} />
      </View>
      <Text variant="footnote" tone="muted">
        {[
          period,
          left !== null && left >= 0 ? timeLeft(left) : null,
          w.voided_at ? `${t('voided_at')}: ${formatDate(w.voided_at, i18n.language)}` : null,
        ]
          .filter(Boolean)
          .join(' · ')}
      </Text>
      <View className="flex-row gap-2">
        <Button
          title={t('warranty_open')}
          variant="secondary"
          icon={<Icon name="externalLink" tone="primary" size={16} />}
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
  );
}
