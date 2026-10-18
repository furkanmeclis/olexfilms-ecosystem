import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Alert, Pressable, ScrollView, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import type { components } from '@/core/api/schema';
import { errorMessage } from '@/core/i18n';
import { vehicleLabel } from '@/features/services/lib';
import { ErrorState, Section } from '@/features/services/ui';
import { Button } from '@/ui/button';
import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';

import {
  addDaysIso,
  BOOKING_MAX_AHEAD_DAYS,
  BOOKING_WINDOW_DAYS,
  dayState,
  formatDayLabel,
  formatSlotTime,
  isoDate,
  type DayState,
} from './booking-lib';
import { type PortalVehicle, retryPortal, useFleetRoles } from './lib';

type Slot = components['schemas']['AppointmentSlot'];
type Dealer = { uuid: string; name: string };

const VEHICLE_PICKER_LIMIT = 100;

/** Message of a failed booking: the capacity / closed-day cases get their own text. */
function bookingError(e: unknown, t: (k: string) => string) {
  if (isApiError(e, 'APPOINTMENT_CAPACITY_FULL')) return t('booking_error_capacity');
  if (isApiError(e, 'APPOINTMENT_DAY_CLOSED')) return t('booking_error_day_closed');
  return errorMessage(isApiError(e) ? e.code : undefined);
}

/**
 * Book an appointment (web: portal > appointments > new): own vehicle, a
 * dealer that takes portal bookings, a free start time from the dealer's
 * availability, an optional note, then POST /v1/portal/appointments.
 * `?dealer=&dealer_name=` and `?vehicle=` preselect. A fleet session cannot book.
 */
export function CustomerBookingScreen() {
  const { t } = useTranslation('customer');
  const qc = useQueryClient();
  const { readOnly } = useFleetRoles();
  const params = useLocalSearchParams<{ dealer?: string; dealer_name?: string; vehicle?: string }>();

  const [dealer, setDealer] = useState<Dealer | null>(
    params.dealer ? { uuid: params.dealer, name: params.dealer_name ?? '' } : null,
  );
  const [vehicle, setVehicle] = useState<string | null>(params.vehicle ?? null);
  const [day, setDay] = useState<string | null>(null);
  const [slot, setSlot] = useState<Slot | null>(null);
  const [zone, setZone] = useState<string | undefined>();
  const [note, setNote] = useState('');

  const chooseDealer = (d: Dealer | null) => {
    setDealer(d);
    setDay(null);
    setSlot(null);
  };

  const create = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST('/v1/portal/appointments', {
          body: {
            dealer_uuid: dealer!.uuid,
            vehicle_uuid: vehicle!,
            starts_at: slot!.start,
            ...(note.trim() ? { note: note.trim() } : {}),
          },
        }),
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['portal', 'appointments'] });
      Alert.alert(t('booking_done'), t('booking_done_body'));
      if (router.canGoBack()) router.back();
      else router.replace('/musteri/appointments');
    },
    onError: (e) => {
      if (isApiError(e, 'APPOINTMENT_CAPACITY_FULL') || isApiError(e, 'APPOINTMENT_DAY_CLOSED')) {
        setSlot(null);
        void qc.invalidateQueries({ queryKey: ['portal', 'appointments', 'availability'] });
      }
      Alert.alert(t('booking_failed'), bookingError(e, t));
    },
  });

  const ready = Boolean(dealer && vehicle && slot);

  return (
    <>
      <Stack.Screen options={{ title: t('booking_title') }} />
      {readOnly ? (
        <EmptyState icon="appointment" title={t('booking_read_only')} />
      ) : (
        <ScrollView
          contentInsetAdjustmentBehavior="automatic"
          keyboardShouldPersistTaps="handled"
          className="flex-1 bg-background"
          contentContainerClassName="gap-5 px-4 pb-12 pt-2">
          <Section title={t('booking_step_vehicle')}>
            <VehiclePicker value={vehicle} onChange={setVehicle} />
          </Section>

          <Section
            title={t('booking_step_dealer')}
            right={
              dealer ? (
                <Pressable accessibilityRole="button" hitSlop={8} onPress={() => chooseDealer(null)}>
                  <Text variant="footnote" tone="primary" className="font-semibold">
                    {t('booking_change')}
                  </Text>
                </Pressable>
              ) : undefined
            }>
            {dealer ? (
              <View className="min-h-14 flex-row items-center gap-3 px-4 py-3">
                <Icon name="location" tone="primary" size={20} />
                <Text variant="headline" className="flex-1" numberOfLines={2}>
                  {dealer.name || t('booking_dealer_selected')}
                </Text>
              </View>
            ) : (
              <DealerPicker onPick={chooseDealer} />
            )}
          </Section>

          <Section title={t('booking_step_time')}>
            {dealer ? (
              <SlotPicker
                dealerUuid={dealer.uuid}
                day={day}
                slot={slot}
                zone={zone}
                onZone={setZone}
                onDay={(d) => {
                  setDay(d);
                  setSlot(null);
                }}
                onSlot={setSlot}
              />
            ) : (
              <Text variant="callout" tone="muted" className="px-4 py-4">
                {t('booking_pick_dealer_first')}
              </Text>
            )}
          </Section>

          <Input
            label={t('booking_note')}
            placeholder={t('booking_note_hint')}
            value={note}
            onChangeText={setNote}
            maxLength={2000}
            multiline
            className="min-h-24 py-3"
            textAlignVertical="top"
          />

          <Button
            size="lg"
            title={create.isPending ? t('booking_booking') : t('booking_confirm')}
            disabled={!ready || create.isPending}
            onPress={() => create.mutate()}
          />
        </ScrollView>
      )}
    </>
  );
}

function RadioRow({
  selected,
  onPress,
  last,
  children,
}: {
  selected: boolean;
  onPress: () => void;
  last?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Pressable
      accessibilityRole="radio"
      accessibilityState={{ selected }}
      onPress={onPress}
      className={cn(
        'min-h-14 flex-row items-center gap-3 px-4 py-3 active:opacity-80',
        !last && 'border-b border-border',
        selected && 'bg-secondary',
      )}>
      {children}
      {selected ? <Icon name="check" tone="primary" size={18} /> : null}
    </Pressable>
  );
}

function RowsSkeleton() {
  return (
    <View className="gap-2 p-3">
      <Skeleton className="h-12 rounded-md" />
      <Skeleton className="h-12 rounded-md" />
    </View>
  );
}

function VehiclePicker({ value, onChange }: { value: string | null; onChange: (uuid: string) => void }) {
  const { t } = useTranslation('customer');
  const q = useQuery({
    queryKey: ['portal', 'vehicles', 'picker'],
    retry: retryPortal,
    queryFn: () =>
      unwrap(api.GET('/v1/portal/vehicles', { params: { query: { limit: VEHICLE_PICKER_LIMIT, offset: 0 } } })),
  });
  const vehicles: PortalVehicle[] = useMemo(() => q.data?.items ?? [], [q.data]);

  // One vehicle: nothing to choose.
  useEffect(() => {
    if (!value && vehicles.length === 1) onChange(vehicles[0].uuid);
  }, [value, vehicles, onChange]);

  if (q.isPending) return <RowsSkeleton />;
  if (q.error) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (vehicles.length === 0) {
    return (
      <Text variant="callout" tone="muted" className="px-4 py-4">
        {t('booking_no_vehicles')}
      </Text>
    );
  }
  return (
    <>
      {vehicles.map((v, i) => {
        const label = vehicleLabel(v.car_brand?.name, v.car_model?.name, v.model_year);
        return (
          <RadioRow key={v.uuid} selected={v.uuid === value} last={i === vehicles.length - 1} onPress={() => onChange(v.uuid)}>
            <Icon name="vehicle" tone="muted-foreground" size={20} />
            <View className="flex-1">
              <Text variant="headline" numberOfLines={1}>
                {v.plate || label || t('vehicle')}
              </Text>
              {v.plate && label ? (
                <Text variant="footnote" tone="muted" numberOfLines={1}>
                  {label}
                </Text>
              ) : null}
            </View>
          </RadioRow>
        );
      })}
    </>
  );
}

/**
 * Dealers the customer already has services with (the mobile portal mirror
 * has no public dealer search yet: it needs the product owner's approval).
 * A dealer that does not take portal bookings answers 404 in the time step.
 */
function DealerPicker({ onPick }: { onPick: (d: Dealer) => void }) {
  const { t } = useTranslation('customer');
  const q = useQuery({
    queryKey: ['portal', 'appointments', 'dealers'],
    retry: retryPortal,
    queryFn: () => unwrap(api.GET('/v1/portal/services', { params: { query: { limit: 100, offset: 0 } } })),
  });
  const dealers = useMemo(() => {
    const seen = new Map<string, Dealer>();
    for (const s of q.data?.items ?? []) {
      if (s.organization?.uuid && !seen.has(s.organization.uuid)) {
        seen.set(s.organization.uuid, { uuid: s.organization.uuid, name: s.organization.name });
      }
    }
    return [...seen.values()];
  }, [q.data]);

  if (q.isPending) return <RowsSkeleton />;
  if (q.error) return <ErrorState error={q.error} onRetry={() => void q.refetch()} />;
  if (dealers.length === 0) {
    return (
      <Text variant="callout" tone="muted" className="px-4 py-4">
        {t('booking_no_dealers')}
      </Text>
    );
  }
  return (
    <>
      {dealers.map((d, i) => (
        <Pressable
          key={d.uuid}
          accessibilityRole="button"
          onPress={() => onPick(d)}
          className={cn(
            'min-h-14 flex-row items-center gap-3 px-4 py-3 active:opacity-80',
            i < dealers.length - 1 && 'border-b border-border',
          )}>
          <Icon name="location" tone="muted-foreground" size={20} />
          <Text variant="headline" className="flex-1" numberOfLines={1}>
            {d.name}
          </Text>
          <Icon name="forward" tone="muted-foreground" size={16} />
        </Pressable>
      ))}
    </>
  );
}

/** Two weeks of the dealer's days at a time; only bookable days offer start times. */
function SlotPicker({
  dealerUuid,
  day,
  slot,
  zone,
  onZone,
  onDay,
  onSlot,
}: {
  dealerUuid: string;
  day: string | null;
  slot: Slot | null;
  zone: string | undefined;
  onZone: (z: string | undefined) => void;
  onDay: (d: string | null) => void;
  onSlot: (s: Slot) => void;
}) {
  const { t, i18n } = useTranslation('customer');
  const [first] = useState(() => isoDate(new Date()));
  const last = addDaysIso(first, BOOKING_MAX_AHEAD_DAYS);
  const [from, setFrom] = useState(first);
  const to = addDaysIso(from, BOOKING_WINDOW_DAYS - 1);
  const lang = i18n.language;

  const q = useQuery({
    queryKey: ['portal', 'appointments', 'availability', dealerUuid, from, to],
    retry: retryPortal,
    queryFn: () =>
      unwrap(
        api.GET('/v1/portal/dealers/{uuid}/availability', {
          params: { path: { uuid: dealerUuid }, query: { from, to } },
        }),
      ),
  });
  const days = useMemo(() => q.data ?? [], [q.data]);
  const selected = days.find((d) => d.date === day);

  // The dealer's zone comes with the days; start times are shown in it.
  const tz = days.find((d) => d.timezone)?.timezone;
  useEffect(() => {
    if (tz !== zone) onZone(tz);
  }, [tz, zone, onZone]);

  // A refetch can fill the chosen day: drop the choice then.
  useEffect(() => {
    if (day && selected && dayState(selected) !== 'bookable') onDay(null);
  }, [day, selected, onDay]);

  const move = (delta: number) => {
    const next = addDaysIso(from, delta);
    setFrom(next < first ? first : next);
    onDay(null);
  };

  const notes: Record<DayState, string> = {
    bookable: '',
    closed: t('booking_day_closed'),
    full: t('booking_day_full'),
    none: t('booking_day_none'),
  };

  return (
    <View className="gap-3 p-3">
      <View className="flex-row items-center justify-between">
        <Button
          variant="ghost"
          title={t('booking_prev_days')}
          disabled={from <= first}
          onPress={() => move(-BOOKING_WINDOW_DAYS)}
        />
        <Text variant="footnote" tone="muted">
          {formatDayLabel(from, lang, { day: 'numeric', month: 'short' })} –{' '}
          {formatDayLabel(to, lang, { day: 'numeric', month: 'short' })}
        </Text>
        <Button
          variant="ghost"
          title={t('booking_next_days')}
          disabled={addDaysIso(from, BOOKING_WINDOW_DAYS) > last}
          onPress={() => move(BOOKING_WINDOW_DAYS)}
        />
      </View>

      {q.isPending ? (
        <ActivityIndicator />
      ) : q.error ? (
        <ErrorState error={q.error} onRetry={() => void q.refetch()} />
      ) : (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerClassName="gap-2">
          {days.map((d) => {
            const state = dayState(d);
            const bookable = state === 'bookable';
            const on = d.date === day;
            return (
              <Pressable
                key={d.date}
                accessibilityRole="button"
                accessibilityState={{ selected: on, disabled: !bookable }}
                disabled={!bookable}
                onPress={() => onDay(d.date)}
                className={cn(
                  'min-h-[76px] w-[84px] items-center justify-center gap-0.5 rounded-lg border px-2',
                  on ? 'border-primary bg-primary' : 'border-border bg-card',
                  !bookable && 'opacity-50',
                )}>
                <Text variant="caption" className={cn(on && 'text-primary-foreground')}>
                  {formatDayLabel(d.date, lang, { weekday: 'short' })}
                </Text>
                <Text variant="headline" className={cn(on && 'text-primary-foreground')}>
                  {formatDayLabel(d.date, lang, { day: 'numeric', month: 'short' })}
                </Text>
                <Text variant="caption" className={cn(on && 'text-primary-foreground')} numberOfLines={1}>
                  {bookable ? t('booking_day_free', { count: d.remaining_capacity }) : notes[state]}
                </Text>
              </Pressable>
            );
          })}
        </ScrollView>
      )}

      {selected && dayState(selected) === 'bookable' ? (
        <View className="gap-2">
          <Text variant="footnote" tone="muted" className="font-medium">
            {tz ? t('booking_pick_time_zone', { zone: tz }) : t('booking_pick_time')}
          </Text>
          <View className="flex-row flex-wrap gap-2">
            {selected.slots.map((s) => {
              const on = slot?.start === s.start;
              return (
                <Pressable
                  key={s.start}
                  accessibilityRole="button"
                  accessibilityState={{ selected: on }}
                  onPress={() => onSlot(s)}
                  className={cn(
                    'min-h-11 min-w-[72px] items-center justify-center rounded-md border px-3',
                    on ? 'border-primary bg-primary' : 'border-border bg-card',
                  )}>
                  <Text variant="callout" className={cn('font-semibold', on && 'text-primary-foreground')}>
                    {formatSlotTime(s.start, lang, tz)}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </View>
      ) : !q.isPending && !q.error ? (
        <Text variant="callout" tone="muted">
          {t('booking_pick_day')}
        </Text>
      ) : null}
    </View>
  );
}
