import { useQuery } from '@tanstack/react-query';
import { type Href, router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Pressable,
  RefreshControl,
  ScrollView,
  useWindowDimensions,
  View,
} from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { useModule } from '@/core/modules';
import { formatDateTime, formatNumber } from '@/features/services/lib';
import { NotificationBell } from '@/features/notifications/bell';
import { errorText } from '@/features/services/ui';
import { HeaderBrand, SpaceButton } from '@/features/shell/header-actions';
import { Button } from '@/ui/button';
import { Skeleton } from '@/ui/skeleton';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import {
  BrandMark,
  DateStub,
  NextStepCard,
  Rise,
  VehiclePage,
  WarrantyProgressCard,
} from './home-parts';
import { isHidden, nameOf, portalServiceTone, retryPortal, useCustomerLinks, useFleetRoles } from './lib';
import { StatTile, useServiceStatusLabel } from './parts';

const HOME_VEHICLES = 20;

function greetingKey(h: number) {
  if (h < 12) return 'home_hello_morning';
  if (h < 18) return 'home_hello_afternoon';
  return 'home_hello_evening';
}

function SectionTitle({ title, onSeeAll }: { title: string; onSeeAll?: () => void }) {
  const { t } = useTranslation('customer');
  return (
    <View className="flex-row items-end justify-between px-1">
      <Text variant="title">{title}</Text>
      {onSeeAll ? (
        <Pressable accessibilityRole="button" hitSlop={8} onPress={onSeeAll}>
          <Text variant="footnote" tone="primary" className="font-semibold">
            {t('see_all')}
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
}

function InlineError({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const { t } = useTranslation('common');
  return (
    <View className="items-center gap-2 rounded-2xl border border-border bg-card p-4">
      <Text variant="callout" tone="muted" className="text-center">
        {errorText(error)}
      </Text>
      <Button title={t('retry')} variant="secondary" onPress={onRetry} />
    </View>
  );
}

/**
 * Customer home (TEC-1019): greeting, the customer's cars as a carousel of
 * cut-out heroes, the warranties of the car on screen with the time left,
 * the one next step, shortcuts and the latest services. Fleet users also get
 * the fleet overview.
 */
export function CustomerHomeScreen() {
  const { t, i18n } = useTranslation('customer');
  const { width } = useWindowDimensions();
  const user = useSession((s) => s.me?.user);
  const links = useCustomerLinks();
  const { fleet } = useFleetRoles();
  const statusLabel = useServiceStatusLabel();
  // Booking is a module the admin can switch off (TEC-1023).
  const booking = useModule('appointments');
  const [page, setPage] = useState(0);

  const vehicles = useQuery({
    queryKey: ['portal', 'vehicles', 'home'],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/vehicles', { params: { query: { limit: HOME_VEHICLES } } })),
  });
  const services = useQuery({
    queryKey: ['portal', 'services', 'home'],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/services', { params: { query: { limit: 3, sort: '-created_at' } } })),
  });
  const next = useQuery({
    queryKey: ['portal', 'appointments', 'next'],
    enabled: booking,
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/appointments', { params: { query: { period: 'upcoming', limit: 1 } } })),
  });
  const fleetQ = useQuery({
    queryKey: ['portal', 'fleet', 'overview'],
    enabled: fleet,
    retry: retryPortal,
    staleTime: 5 * 60_000,
    queryFn: () => unwrap(api.GET('/v1/portal/fleet/overview')),
  });

  const vItems = vehicles.data?.items ?? [];
  const current = vItems[Math.min(page, Math.max(0, vItems.length - 1))];
  // The warranties of the car on screen, from its detail (cached per car).
  const detail = useQuery({
    queryKey: ['portal', 'vehicles', 'detail', current?.uuid],
    enabled: !!current,
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/vehicles/{uuid}', { params: { path: { uuid: current!.uuid } } })),
  });

  // Only a pull shows the spinner: background refetches toggling `refreshing`
  // made iOS push the content down without giving the inset back.
  const [pulling, setPulling] = useState(false);
  const refresh = async () => {
    setPulling(true);
    await Promise.allSettled([
      vehicles.refetch(),
      services.refetch(),
      booking ? next.refetch() : null,
      current ? detail.refetch() : null,
      fleet ? fleetQ.refetch() : null,
    ]);
    setPulling(false);
  };

  const appt = next.data?.items?.[0];
  const ov = fleetQ.data;
  const warranties = detail.data?.active_warranties ?? [];
  const brandOf = (vehicleUuid: string) => vItems.find((v) => v.uuid === vehicleUuid)?.car_brand;
  const hello = t(greetingKey(new Date().getHours()));

  return (
    <>
      <Stack.Screen options={{ title: t('home') }} />
      <ScrollView
        className="flex-1 bg-background"
        // No native header here (the stack hides it up front); iOS adds the status bar
        // and floating tab bar insets itself, so the padding stays small.
        contentInsetAdjustmentBehavior="automatic"
        contentContainerStyle={{ paddingTop: 8, paddingBottom: 32 }}
        refreshControl={
          <RefreshControl
            refreshing={pulling}
            onRefresh={() => void refresh()}
          />
        }>
        <View className="flex-row items-center justify-between px-5">
          <HeaderBrand />
          <View className="flex-row items-center gap-3">
            <NotificationBell />
            <SpaceButton />
          </View>
        </View>

        <View className="gap-0.5 px-5 pb-2 pt-5">
          <Text variant="callout" tone="muted" className="font-medium">
            {hello}
          </Text>
          <Text variant="largeTitle" className="text-[38px] leading-[42px] font-extrabold" numberOfLines={2}>
            {user?.name ?? ''}
            {user?.surname ? `\n${user.surname}` : ''}
          </Text>
        </View>

        {/* Cars */}
        {isHidden(vehicles.error) ? null : vehicles.isPending ? (
          <View className="px-5 pt-3">
            <Skeleton className="h-56 rounded-3xl" />
          </View>
        ) : vehicles.error ? (
          <View className="px-5">
            <InlineError error={vehicles.error} onRetry={() => void vehicles.refetch()} />
          </View>
        ) : vItems.length ? (
          <View className="gap-3">
            {vItems.length > 1 ? (
              // Several cars: pick one by plate; the hero itself never scrolls.
              <View className="flex-row flex-wrap gap-2 px-5">
                {vItems.map((v, i) => {
                  const on = v.uuid === current?.uuid;
                  return (
                    <Pressable
                      key={v.uuid}
                      accessibilityRole="button"
                      accessibilityState={{ selected: on }}
                      onPress={() => setPage(i)}
                      className={on ? 'rounded-full bg-primary px-3.5 py-1.5' : 'rounded-full border border-border bg-card px-3.5 py-1.5 active:opacity-70'}>
                      <Text variant="footnote" tone={on ? 'onPrimary' : 'default'} className="font-semibold tabular-nums">
                        {v.plate || [v.car_brand?.name, v.car_model?.name].filter(Boolean).join(' ')}
                      </Text>
                    </Pressable>
                  );
                })}
              </View>
            ) : null}
            {current ? (
              <VehiclePage key={current.uuid} v={current} width={width} onPress={() => router.push(links.vehicle(current.uuid))} />
            ) : null}
          </View>
        ) : (
          <View className="px-5 pt-2">
            <NextStepCard
              icon="location"
              eyebrow={t('home_no_car_eyebrow')}
              title={t('home_no_car_title')}
              body={t('home_no_car_body')}
            />
          </View>
        )}

        <View className="gap-7 px-5 pt-6">
          {/* Next step */}
          {!booking || isHidden(next.error) ? null : (
            <Rise index={0}>
              {appt ? (
                <NextStepCard
                  icon="appointment"
                  eyebrow={t('next_appointment')}
                  title={formatDateTime(appt.starts_at, i18n.language)}
                  body={[appt.dealer_name, appt.vehicle_plate].filter(Boolean).join(' · ')}
                  cta={t('home_view_appointment')}
                  onPress={() => router.push('/musteri/appointments')}
                />
              ) : !next.isPending && vItems.length ? (
                <NextStepCard
                  icon="appointment"
                  eyebrow={t('home_care_eyebrow')}
                  title={t('home_care_title')}
                  body={t('home_care_body')}
                  cta={t('home_book')}
                  onPress={() => router.push('/musteri/appointments/new' as Href)}
                />
              ) : null}
            </Rise>
          )}

          {/* Warranties of the car on screen */}
          {current && !isHidden(detail.error) ? (
            <View className="gap-3">
              <SectionTitle title={t('home_protection')} onSeeAll={() => router.push('/musteri/services' as Href)} />
              {detail.isPending ? (
                <Skeleton className="h-28 rounded-2xl" />
              ) : detail.error ? (
                <InlineError error={detail.error} onRetry={() => void detail.refetch()} />
              ) : warranties.length ? (
                warranties.slice(0, 3).map((w, i) => (
                  <Rise key={w.uuid} index={i + 2}>
                    <WarrantyProgressCard w={w} onPress={() => router.push(links.service(w.service.uuid))} />
                  </Rise>
                ))
              ) : (
                <Text variant="callout" tone="muted" className="px-1">
                  {t('home_no_warranty')}
                </Text>
              )}
            </View>
          ) : null}

          {/* Fleet */}
          {fleet && ov ? (
            <View className="gap-3">
              <SectionTitle title={t('fleet_title', { name: ov.fleet?.name ?? '' })} />
              <View className="flex-row flex-wrap gap-2.5">
                <StatTile icon="truck" label={t('fleet_vehicles')} value={formatNumber(ov.vehicle_count ?? 0, i18n.language)} />
                <StatTile icon="service" label={t('fleet_services')} value={formatNumber(ov.service_count ?? 0, i18n.language)} />
                <StatTile
                  icon="warranty"
                  label={t('active_warranties')}
                  value={formatNumber(ov.active_warranty_count ?? 0, i18n.language)}
                />
                <StatTile
                  icon="building"
                  label={t('fleet_dealers')}
                  value={formatNumber(ov.dealers?.filter((d) => d.link_status === 'active').length ?? 0, i18n.language)}
                />
              </View>
              {(ov.pending_link_count ?? 0) > 0 ? (
                <Text variant="footnote" tone="muted" className="px-1">
                  {t('fleet_pending_links', { count: ov.pending_link_count })}
                </Text>
              ) : null}
            </View>
          ) : null}

          {/* Latest services */}
          {isHidden(services.error) ? null : (
            <View className="gap-3">
              <SectionTitle title={t('latest_services')} onSeeAll={() => router.push('/musteri/services' as Href)} />
              {services.isPending ? (
                [0, 1].map((i) => <Skeleton key={i} className="h-20 rounded-2xl" />)
              ) : services.error ? (
                <InlineError error={services.error} onRetry={() => void services.refetch()} />
              ) : (services.data?.items?.length ?? 0) > 0 ? (
                services.data!.items.map((s, i) => {
                  const brand = brandOf(s.vehicle_uuid);
                  return (
                    <Rise key={s.uuid} index={i + 5}>
                      <Pressable
                        accessibilityRole="button"
                        onPress={() => router.push(links.service(s.uuid))}
                        className="flex-row items-center gap-3 rounded-2xl border border-border bg-card p-3.5 active:opacity-85"
                        style={{ borderCurve: 'continuous' }}>
                        <BrandMark uuid={brand?.uuid} name={s.car_brand_name} />
                        <View className="flex-1 gap-0.5">
                          <Text variant="headline" numberOfLines={1}>
                            {[s.car_brand_name, s.car_model_name].filter(Boolean).join(' ')}
                          </Text>
                          <Text variant="footnote" tone="muted" numberOfLines={1}>
                            {[nameOf(s.organization), s.plate].filter(Boolean).join(' · ')}
                          </Text>
                          <View className="flex-row pt-1">
                            <StatusPill label={statusLabel(s.status)} tone={portalServiceTone(s.status)} />
                          </View>
                        </View>
                        <DateStub iso={s.completed_at ?? s.created_at} />
                      </Pressable>
                    </Rise>
                  );
                })
              ) : (
                <Text variant="callout" tone="muted" className="px-1">
                  {t('services_empty_body')}
                </Text>
              )}
            </View>
          )}
        </View>
      </ScrollView>
    </>
  );
}

