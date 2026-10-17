import { useQuery } from '@tanstack/react-query';
import { type Href, router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { errorText } from '@/features/services/ui';
import { HeaderBrand } from '@/features/shell/header-actions';
import { formatDateTime, formatNumber } from '@/features/services/lib';
import { Button } from '@/ui/button';
import {
  Building2,
  CalendarDays,
  CarFront,
  type LucideIcon,
  MapPin,
  ShieldCheck,
  Truck,
  Wrench,
} from '@/ui/icons';
import { Icon } from '@/ui/icon';
import { Skeleton } from '@/ui/skeleton';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { appointmentTone, isHidden, openDealerFinder, retryPortal, useCustomerLinks, useFleetRoles } from './lib';
import { PortalServiceRow, StatTile, VehicleCard } from './parts';

const HOME_VEHICLES = 20;

function greetingKey(h: number) {
  if (h < 12) return 'greeting_morning';
  if (h < 18) return 'greeting_afternoon';
  return 'greeting_evening';
}

function SectionTitle({ title, onSeeAll }: { title: string; onSeeAll?: () => void }) {
  const { t } = useTranslation('customer');
  return (
    <View className="flex-row items-center justify-between px-1">
      <Text variant="caption" tone="muted">
        {title}
      </Text>
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
    <View className="items-center gap-2 rounded-lg border border-border bg-card p-4">
      <Text variant="callout" tone="muted" className="text-center">
        {errorText(error)}
      </Text>
      <Button title={t('retry')} variant="secondary" onPress={onRetry} />
    </View>
  );
}

function InlineEmpty({ icon, text }: { icon: LucideIcon; text: string }) {
  return (
    <View className="items-center gap-2 rounded-lg border border-border bg-card p-5">
      <Icon as={icon} tone="muted-foreground" />
      <Text variant="callout" tone="muted" className="text-center">
        {text}
      </Text>
    </View>
  );
}

/**
 * Customer home: vehicles (with their warranty counts), latest services and
 * the next appointment, three small parallel requests. Fleet users also get
 * the fleet overview card.
 */
export function CustomerHomeScreen() {
  const { t, i18n } = useTranslation('customer');
  const name = useSession((s) => s.me?.user.name);
  const links = useCustomerLinks();
  const { fleet } = useFleetRoles();

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

  const refresh = () => {
    void vehicles.refetch();
    void services.refetch();
    void next.refetch();
    if (fleet) void fleetQ.refetch();
  };

  const vItems = vehicles.data?.items ?? [];
  const vTotal = vehicles.data?.total ?? vItems.length;
  const activeWarranties = vItems.reduce((sum, v) => sum + (v.active_warranty_count ?? 0), 0);
  const appt = next.data?.items?.[0];
  const ov = fleetQ.data;

  const actions: { key: string; icon: LucideIcon; onPress: () => void }[] = [
    { key: 'appointments', icon: CalendarDays, onPress: () => router.push('/musteri/appointments') },
    { key: 'warranties', icon: ShieldCheck, onPress: () => router.push('/musteri/warranties') },
    { key: 'services', icon: Wrench, onPress: () => router.push('/musteri/services' as Href) },
    { key: 'find_dealer', icon: MapPin, onPress: () => void openDealerFinder() },
  ];

  return (
    <>
      <Stack.Screen
        options={{
          title: name ? t(greetingKey(new Date().getHours()), { name }) : t('home'),
          headerLeft: () => <HeaderBrand />,
        }}
      />
      <ScrollView
        contentInsetAdjustmentBehavior="automatic"
        className="flex-1 bg-background"
        contentContainerClassName="gap-4 px-4 pb-12 pt-2"
        refreshControl={
          <RefreshControl
            refreshing={vehicles.isRefetching || services.isRefetching || next.isRefetching}
            onRefresh={refresh}
          />
        }>
        <View className="flex-row flex-wrap gap-2.5">
          {actions.map((a) => (
            <Pressable
              key={a.key}
              accessibilityRole="button"
              onPress={a.onPress}
              className="min-h-[76px] grow basis-[46%] justify-between rounded-lg border border-border bg-card p-3 active:opacity-80">
              <View className="size-8 items-center justify-center rounded-md bg-secondary">
                <Icon as={a.icon} tone="primary" size={18} />
              </View>
              <Text variant="callout" className="font-semibold" numberOfLines={1}>
                {t(`action.${a.key}`)}
              </Text>
            </Pressable>
          ))}
        </View>

        {fleet && ov ? (
          <View className="gap-1.5">
            <SectionTitle title={t('fleet_title', { name: ov.fleet?.name ?? '' })} />
            <View className="flex-row flex-wrap gap-2.5">
              <StatTile icon={Truck} label={t('fleet_vehicles')} value={formatNumber(ov.vehicle_count ?? 0, i18n.language)} />
              <StatTile icon={Wrench} label={t('fleet_services')} value={formatNumber(ov.service_count ?? 0, i18n.language)} />
              <StatTile
                icon={ShieldCheck}
                label={t('active_warranties')}
                value={formatNumber(ov.active_warranty_count ?? 0, i18n.language)}
              />
              <StatTile
                icon={Building2}
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

        {vehicles.isPending ? (
          <View className="flex-row gap-2.5">
            <Skeleton className="h-20 grow rounded-lg" />
            <Skeleton className="h-20 grow rounded-lg" />
          </View>
        ) : vehicles.data ? (
          <View className="flex-row flex-wrap gap-2.5">
            <StatTile
              icon={CarFront}
              label={t('vehicles')}
              value={formatNumber(vTotal, i18n.language)}
              onPress={() => router.push('/musteri/vehicles' as Href)}
            />
            <StatTile
              icon={ShieldCheck}
              label={t('active_warranties')}
              value={formatNumber(activeWarranties, i18n.language)}
              onPress={() => router.push('/musteri/warranties')}
            />
          </View>
        ) : null}

        {!isHidden(next.error) ? (
          <View className="gap-2">
            <SectionTitle title={t('next_appointment')} onSeeAll={() => router.push('/musteri/appointments')} />
            {next.isPending ? (
              <Skeleton className="h-[68px] rounded-lg" />
            ) : next.error ? (
              <InlineError error={next.error} onRetry={() => void next.refetch()} />
            ) : appt ? (
              <Pressable
                accessibilityRole="button"
                onPress={() => router.push('/musteri/appointments')}
                className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 active:opacity-80">
                <View className="size-10 items-center justify-center rounded-md bg-secondary">
                  <Icon as={CalendarDays} tone="primary" size={20} />
                </View>
                <View className="flex-1 gap-0.5">
                  <Text variant="headline" numberOfLines={1}>
                    {formatDateTime(appt.starts_at, i18n.language)}
                  </Text>
                  <Text variant="footnote" tone="muted" numberOfLines={1}>
                    {[appt.dealer_name, appt.vehicle_plate].filter(Boolean).join(' · ')}
                  </Text>
                </View>
                <StatusPill label={t(`appointment_status.${appt.status}`)} tone={appointmentTone(appt.status)} />
              </Pressable>
            ) : (
              <InlineEmpty icon={CalendarDays} text={t('next_appointment_none')} />
            )}
          </View>
        ) : null}

        {!isHidden(vehicles.error) ? (
          <View className="gap-2">
            <SectionTitle
              title={t('vehicles')}
              onSeeAll={vTotal > 3 ? () => router.push('/musteri/vehicles' as Href) : undefined}
            />
            {vehicles.isPending ? (
              [0, 1].map((i) => <Skeleton key={i} className="h-[76px] rounded-lg" />)
            ) : vehicles.error ? (
              <InlineError error={vehicles.error} onRetry={() => void vehicles.refetch()} />
            ) : vItems.length > 0 ? (
              vItems.slice(0, 3).map((v) => (
                <VehicleCard key={v.uuid} v={v} onPress={() => router.push(links.vehicle(v.uuid))} />
              ))
            ) : (
              <InlineEmpty icon={CarFront} text={t('vehicles_empty_body')} />
            )}
          </View>
        ) : null}

        {!isHidden(services.error) ? (
          <View className="gap-2">
            <SectionTitle title={t('latest_services')} onSeeAll={() => router.push('/musteri/services' as Href)} />
            {services.isPending ? (
              [0, 1].map((i) => <Skeleton key={i} className="h-[76px] rounded-lg" />)
            ) : services.error ? (
              <InlineError error={services.error} onRetry={() => void services.refetch()} />
            ) : (services.data?.items?.length ?? 0) > 0 ? (
              services.data!.items.map((s) => (
                <PortalServiceRow key={s.uuid} s={s} onPress={() => router.push(links.service(s.uuid))} />
              ))
            ) : (
              <InlineEmpty icon={Wrench} text={t('services_empty_body')} />
            )}
          </View>
        ) : null}
      </ScrollView>
    </>
  );
}
