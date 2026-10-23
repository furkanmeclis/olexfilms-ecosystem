import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type Href, router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { formatDate, formatNumber, vehicleLabel } from '@/features/services/lib';
import { DetailSkeleton, ErrorState, ListBody, ListFooter, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import type { IconName } from '@/ui/icons';
import { List, ListRow } from '@/ui/list';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  type FleetLink,
  fleetLinkTone,
  type FleetReport,
  type FleetService,
  type FleetVehicle,
  type FleetWarranty,
  fleetWarrantyTone,
  reportPeriod,
  sortLinks,
} from './fleet-lib';
import { PAGE_SIZE, portalServiceTone, retryPortal, usePortalPages } from './lib';
import { StatTile, useServiceStatusLabel } from './parts';

const linksKey = ['portal', 'fleet', 'links'] as const;

/**
 * Card row of the fleet lists. Shared by the customer fleet portal and the
 * dealer fleet management screens (TEC-959); `onPress` makes it a link and
 * `children` holds row actions.
 */
export function FleetRow({
  title,
  subtitle,
  meta,
  pill,
  onPress,
  children,
}: {
  title: string;
  subtitle?: string;
  meta?: string;
  pill?: React.ReactNode;
  onPress?: () => void;
  children?: React.ReactNode;
}) {
  const body = (
    <View className="gap-1 rounded-lg border border-border bg-card px-4 py-3">
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="headline" numberOfLines={1} className="flex-1">
          {title}
        </Text>
        {pill}
      </View>
      {subtitle ? (
        <Text variant="callout" numberOfLines={1}>
          {subtitle}
        </Text>
      ) : null}
      {meta ? (
        <Text variant="footnote" tone="muted" numberOfLines={1}>
          {meta}
        </Text>
      ) : null}
      {children}
    </View>
  );
  if (!onPress) return body;
  return (
    <Pressable accessibilityRole="button" onPress={onPress} className="active:opacity-80">
      {body}
    </Pressable>
  );
}

type VehicleLike = Pick<FleetVehicle, 'plate' | 'car_brand' | 'car_model' | 'model_year' | 'active_warranty_count'>;

/** Fleet vehicle row: plate (or model) title, model subtitle, active warranty pill. */
export function FleetVehicleRow({ vehicle: v, meta, children }: { vehicle: VehicleLike; meta?: string; children?: React.ReactNode }) {
  const { t } = useTranslation('customer');
  const model = vehicleLabel(v.car_brand?.name, v.car_model?.name, v.model_year);
  return (
    <FleetRow
      title={v.plate || model || t('vehicle')}
      subtitle={v.plate ? model : undefined}
      meta={meta}
      pill={
        v.active_warranty_count > 0 ? (
          <StatusPill label={t('active_warranties_short', { count: v.active_warranty_count })} tone="done" />
        ) : undefined
      }>
      {children}
    </FleetRow>
  );
}

/** Fleet report row: kind, period and creation date (plus an optional state pill). */
export function FleetReportRow({ report: r, pill }: { report: Pick<FleetReport, 'period_kind' | 'period_start' | 'period_end' | 'created_at'>; pill?: React.ReactNode }) {
  const { t, i18n } = useTranslation('customer');
  return (
    <FleetRow
      title={t(`fleet_report_kind.${r.period_kind}`)}
      subtitle={reportPeriod(r.period_start, r.period_end)}
      meta={t('fleet_report_created', { date: formatDate(r.created_at, i18n.language) })}
      pill={pill}
    />
  );
}

/**
 * Shared paged list of the fleet screens (infinite scroll, pull to refresh,
 * empty / error / loading). The portal and the dealer panel pass their own
 * endpoint in `fetchPage`; `header` and `headerRight` add filters and actions.
 */
export function FleetList<T extends { uuid: string }>({
  title,
  queryKey,
  fetchPage,
  renderItem,
  empty,
  header,
  headerRight,
}: {
  title: string;
  queryKey: readonly unknown[];
  fetchPage: (offset: number) => Promise<{ items: T[]; total: number }>;
  renderItem: (item: T) => React.ReactElement;
  empty: { icon: IconName; title: string; body?: string };
  header?: React.ReactElement;
  headerRight?: () => React.ReactNode;
}) {
  const list = usePortalPages(queryKey, fetchPage);
  return (
    <>
      <Stack.Screen options={{ title, headerRight }} />
      <FlatList
        data={list.items}
        keyExtractor={(i) => i.uuid}
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-2.5 px-4 pb-10 pt-2"
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        ListHeaderComponent={header}
        ListEmptyComponent={
          <ListBody
            isPending={list.isPending}
            error={list.error}
            count={list.items.length}
            onRetry={() => void list.refetch()}
            empty={empty}
          />
        }
        renderItem={({ item }) => renderItem(item)}
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

export function FleetVehiclesScreen() {
  const { t, i18n } = useTranslation('customer');
  return (
    <FleetList<FleetVehicle>
      title={t('fleet_vehicles')}
      queryKey={['portal', 'fleet', 'vehicles']}
      fetchPage={(offset) => unwrap(api.GET('/v1/portal/fleet/vehicles', { params: { query: { limit: PAGE_SIZE, offset } } }))}
      empty={{ icon: 'truck', title: t('fleet_vehicles_empty'), body: t('fleet_vehicles_empty_body') }}
      renderItem={(v) => (
        <FleetVehicleRow
          vehicle={v}
          meta={[
            t('service_count', { count: v.service_count }),
            v.warranty_until ? t('until', { date: formatDate(v.warranty_until, i18n.language) }) : null,
          ]
            .filter(Boolean)
            .join(' · ')}
        />
      )}
    />
  );
}

export function FleetServicesScreen() {
  const { t, i18n } = useTranslation('customer');
  const statusLabel = useServiceStatusLabel();
  return (
    <FleetList<FleetService>
      title={t('services')}
      queryKey={['portal', 'fleet', 'services']}
      fetchPage={(offset) => unwrap(api.GET('/v1/portal/fleet/services', { params: { query: { limit: PAGE_SIZE, offset } } }))}
      empty={{ icon: 'service', title: t('services_empty'), body: t('services_empty_body') }}
      renderItem={(s) => (
        <FleetRow
          title={s.plate || s.service_no}
          subtitle={[vehicleLabel(s.car_brand_name, s.car_model_name), s.package].filter(Boolean).join(' · ')}
          meta={[s.service_no, s.dealer?.name, formatDate(s.completed_at ?? s.created_at, i18n.language)]
            .filter(Boolean)
            .join(' · ')}
          pill={<StatusPill label={statusLabel(s.status)} tone={portalServiceTone(s.status)} />}
        />
      )}
    />
  );
}

export function FleetWarrantiesScreen() {
  const { t, i18n } = useTranslation('customer');
  return (
    <FleetList<FleetWarranty>
      title={t('warranties')}
      queryKey={['portal', 'fleet', 'warranties']}
      fetchPage={(offset) =>
        unwrap(api.GET('/v1/portal/fleet/warranties', { params: { query: { limit: PAGE_SIZE, offset } } }))
      }
      empty={{ icon: 'warranty', title: t('warranties_empty'), body: t('warranties_empty_body') }}
      renderItem={(w) => (
        <FleetRow
          title={w.product_name || w.public_code}
          subtitle={w.plate ?? undefined}
          meta={[
            w.public_code,
            t('until', { date: formatDate(w.end_at, i18n.language) }),
            w.state === 'active' && w.days_left >= 0 ? t('days_left', { count: w.days_left }) : null,
            w.dealer?.name,
          ]
            .filter(Boolean)
            .join(' · ')}
          pill={<StatusPill label={t(`warranty_status.${w.state}`)} tone={fleetWarrantyTone(w.state)} />}
        />
      )}
    />
  );
}

/** Generated fleet reports (read only: kind, period, creation date). */
export function FleetReportsScreen() {
  const { t } = useTranslation('customer');
  return (
    <FleetList<FleetReport>
      title={t('fleet_reports')}
      queryKey={['portal', 'fleet', 'reports']}
      fetchPage={(offset) => unwrap(api.GET('/v1/portal/fleet/reports', { params: { query: { limit: PAGE_SIZE, offset } } }))}
      empty={{ icon: 'report', title: t('fleet_reports_empty'), body: t('fleet_reports_empty_body') }}
      renderItem={(r) => <FleetReportRow report={r} />}
    />
  );
}

function LinkRow({ link, busy, onAccept, onReject }: { link: FleetLink; busy: boolean; onAccept: () => void; onReject: () => void }) {
  const { t, i18n } = useTranslation('customer');
  return (
    <View className="gap-2 border-b border-border px-4 py-3">
      <View className="flex-row items-center justify-between gap-3">
        <Text variant="body" numberOfLines={1} className="flex-1">
          {link.dealer_name}
        </Text>
        <StatusPill label={t(`fleet_link_status.${link.status}`)} tone={fleetLinkTone(link.status)} />
      </View>
      <Text variant="footnote" tone="muted">
        {formatDate(link.started_at ?? link.created_at, i18n.language)}
      </Text>
      {link.status === 'pending' ? (
        <View className="flex-row gap-2">
          <Button title={t('fleet_link_accept')} loading={busy} onPress={onAccept} className="flex-1" />
          <Button title={t('fleet_link_reject')} variant="outline" disabled={busy} onPress={onReject} className="flex-1" />
        </View>
      ) : null}
    </View>
  );
}

/**
 * Fleet space home: the month overview, dealer link requests (accept / reject)
 * and entries to the vehicle, service, warranty and report lists.
 */
export function FleetHomeScreen() {
  const { t, i18n } = useTranslation('customer');
  const qc = useQueryClient();
  const overview = useQuery({
    queryKey: ['portal', 'fleet', 'overview'],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/fleet/overview')),
  });
  const links = useQuery({
    queryKey: linksKey,
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/fleet/links')),
  });

  const answer = useMutation({
    mutationFn: ({ uuid, accept }: { uuid: string; accept: boolean }) =>
      unwrap(
        accept
          ? api.POST('/v1/portal/fleet/links/{uuid}/accept', { params: { path: { uuid } } })
          : api.POST('/v1/portal/fleet/links/{uuid}/reject', { params: { path: { uuid } } }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['portal', 'fleet'] });
    },
    onError: (e) => {
      Alert.alert(t('fleet_link_failed'), errorMessage(isApiError(e) ? e.code : undefined));
    },
  });

  const confirmReject = (l: FleetLink) =>
    Alert.alert(t('fleet_link_reject_title'), l.dealer_name, [
      { text: t('keep'), style: 'cancel' },
      { text: t('fleet_link_reject'), style: 'destructive', onPress: () => answer.mutate({ uuid: l.uuid, accept: false }) },
    ]);

  const ov = overview.data;
  const linkItems = sortLinks(links.data?.items ?? []);
  const entries: { key: string; icon: IconName; title: string; href: Href }[] = [
    { key: 'vehicles', icon: 'truck', title: t('fleet_vehicles'), href: '/musteri/fleet/vehicles' as Href },
    { key: 'services', icon: 'service', title: t('services'), href: '/musteri/fleet/services' as Href },
    { key: 'warranties', icon: 'warranty', title: t('warranties'), href: '/musteri/fleet/warranties' as Href },
    { key: 'reports', icon: 'report', title: t('fleet_reports'), href: '/musteri/fleet/reports' as Href },
  ];

  return (
    <>
      <Stack.Screen options={{ title: t('fleet') }} />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={
          <RefreshControl
            refreshing={overview.isRefetching || links.isRefetching}
            onRefresh={() => {
              void overview.refetch();
              void links.refetch();
            }}
          />
        }>
        {ov ? (
          <View className="gap-1.5">
            <Text variant="caption" tone="muted" className="px-1">
              {t('fleet_title', { name: ov.fleet?.name ?? '' })}
            </Text>
            <View className="flex-row flex-wrap gap-2.5">
              <StatTile icon="truck" label={t('fleet_vehicles')} value={formatNumber(ov.vehicle_count ?? 0, i18n.language)} />
              <StatTile icon="service" label={t('fleet_services')} value={formatNumber(ov.service_count ?? 0, i18n.language)} />
              <StatTile
                icon="warranty"
                label={t('active_warranties')}
                value={formatNumber(ov.active_warranty_count ?? 0, i18n.language)}
              />
              <StatTile
                icon="appointment"
                label={t('fleet_upcoming_appointments')}
                value={formatNumber(ov.upcoming_appointment_count ?? 0, i18n.language)}
              />
            </View>
          </View>
        ) : overview.isPending ? (
          <DetailSkeleton />
        ) : (
          <ErrorState error={overview.error} onRetry={() => void overview.refetch()} />
        )}

        <List>
          {entries.map((e, i) => (
            <ListRow
              key={e.key}
              icon={e.icon}
              title={e.title}
              onPress={() => router.push(e.href)}
              last={i === entries.length - 1}
            />
          ))}
        </List>

        {links.error ? null : (
          <Section title={t('fleet_links')}>
            {linkItems.length === 0 ? (
              <Text variant="callout" tone="muted" className="px-4 py-4">
                {links.isPending ? t('common:loading') : t('fleet_links_empty')}
              </Text>
            ) : (
              linkItems.map((l) => (
                <LinkRow
                  key={l.uuid}
                  link={l}
                  busy={answer.isPending && answer.variables?.uuid === l.uuid}
                  onAccept={() => answer.mutate({ uuid: l.uuid, accept: true })}
                  onReject={() => confirmReject(l)}
                />
              ))
            )}
          </Section>
        )}
      </ScrollView>
    </>
  );
}
