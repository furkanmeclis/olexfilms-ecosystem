import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { ErrorState, ListSkeleton } from '@/features/admin/kit';
import { EmptyState } from '@/ui/empty-state';

import { api, unwrap } from '@/core/api/client';
import { CustomerStep, type PickedCustomer, VehicleStep } from '@/features/services/new-service-screen';
import { fullName, type Vehicle, vehicleLabel } from '@/features/services/lib';
import { cn } from '@/ui/cn';
import { Button } from '@/ui/button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import type { IconName } from '@/ui/icons';

import { useBookingError } from './appointment-detail-screen';
import { findAppointment, useAvailability } from './data';
import { type AvailabilityDay, canReschedule, parseDay, todayIn } from './lib';

type Step = 'customer' | 'vehicle' | 'time';
const STEPS: Step[] = ['customer', 'vehicle', 'time'];

/**
 * New booking (customer, vehicle, time) or, with `uuid`, moving an existing
 * one to another time. Times come only from the availability slots, so the
 * workshop's hours, closures and capacity are never guessed on the phone.
 */
export function AppointmentFormScreen({ edit = false }: { edit?: boolean }) {
  const { t } = useTranslation('appointments');
  const qc = useQueryClient();
  const bookingError = useBookingError();
  const canWrite = useCan('appointments.write');
  const { uuid, at } = useLocalSearchParams<{ uuid?: string; at?: string }>();
  const [step, setStep] = useState<Step>('customer');
  const [customer, setCustomer] = useState<PickedCustomer | null>(null);
  const [vehicle, setVehicle] = useState<Vehicle | null>(null);
  const [noVehicle, setNoVehicle] = useState(false);
  const [slot, setSlot] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);

  const existing = useQuery({
    queryKey: ['appointments', 'detail', uuid],
    queryFn: () => findAppointment(uuid!, at),
    enabled: edit && !!uuid,
  });
  const current = edit ? existing.data : null;

  const save = useMutation({
    mutationFn: () => {
      const body = {
        starts_at: slot!,
        note: (note ?? current?.note ?? '').trim(),
        source: current?.source ?? ('panel' as const),
        ...(current
          ? { estimated_minutes: current.estimated_minutes, customer_uuid: current.customer_uuid, vehicle_uuid: current.vehicle_uuid ?? null }
          : { customer_uuid: customer!.uuid, vehicle_uuid: vehicle?.uuid ?? null }),
      };
      return current
        ? unwrap(api.PATCH('/v1/appointments/{uuid}', { params: { path: { uuid: current.uuid } }, body }))
        : unwrap(api.POST('/v1/appointments', { body }));
    },
    onSuccess: async (saved) => {
      await qc.cancelQueries({ queryKey: ['appointments', 'detail', saved.uuid] });
      qc.setQueryData(['appointments', 'detail', saved.uuid], saved);
      void qc.invalidateQueries({ queryKey: ['appointments', 'list'] });
      void qc.invalidateQueries({ queryKey: ['appointments', 'availability'] });
      router.back();
    },
    onError: (e) => Alert.alert(bookingError(e)),
  });

  const loadingExisting = edit && !!uuid && existing.isPending
  const timeStep = edit || step === 'time';
  const canSave = !!slot && (edit ? !!current && canReschedule(current) : !!customer && (!!vehicle || noVehicle));

  return (
    <>
      <Stack.Screen options={{ title: t(edit ? 'action.reschedule' : 'new') }} />
      <Screen>
        {!canWrite ? (
          <EmptyState icon="blocked" title={t('no_permission')} />
        ) : loadingExisting ? (
          <ListSkeleton rows={3} inset={false} />
        ) : edit && !!uuid && existing.isError ? (
          <ErrorState error={existing.error} onRetry={() => void existing.refetch()} />
        ) : edit && !current ? (
          <EmptyState icon="appointment" title={t('not_found')} />
        ) : (
          <>
        {!edit ? (
          <View className="flex-row gap-2">
            {STEPS.map((s, i) => {
              const reached = STEPS.indexOf(step) >= i;
              const reachable = s === 'customer' || (s === 'vehicle' && !!customer) || (s === 'time' && !!customer && (!!vehicle || noVehicle));
              return (
                <Pressable key={s} accessibilityRole="button" disabled={!reachable} onPress={() => setStep(s)} className="flex-1 gap-1.5">
                  <View className={cn('h-1 rounded-full', reached ? 'bg-primary' : 'bg-muted')} />
                  <Text variant="footnote" tone={s === step ? 'default' : 'muted'} className="font-semibold">
                    {i + 1}. {t(`step.${s}`)}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        ) : null}

        {!edit && step === 'customer' ? (
          <CustomerStep
            selected={customer}
            onPick={(c) => {
              if (c.uuid !== customer?.uuid) {
                setVehicle(null);
                setNoVehicle(false);
              }
              setCustomer(c);
              setStep('vehicle');
            }}
          />
        ) : null}

        {!edit && step === 'vehicle' && customer ? (
          <View className="gap-3">
            <VehicleStep
              customer={customer}
              selected={vehicle}
              onPick={(v) => {
                setVehicle(v);
                setNoVehicle(false);
                setStep('time');
              }}
            />
            <Button
              title={t('skip_vehicle')}
              variant="ghost"
              onPress={() => {
                setVehicle(null);
                setNoVehicle(true);
                setStep('time');
              }}
            />
          </View>
        ) : null}

        {timeStep && (!edit || current) ? (
          <View className="gap-4">
            {edit && current ? (
              <Summary icon="customer" title={current.customer_name ?? '–'} subtitle={[current.vehicle_plate, current.vehicle_label].filter(Boolean).join(' · ')} />
            ) : customer ? (
              <View className="gap-2">
                <Summary icon="customer" title={fullName(customer)} subtitle={customer.phone ?? ''} onPress={() => setStep('customer')} />
                <Summary
                  icon="vehicle"
                  title={vehicle?.plate ?? t('no_vehicle')}
                  subtitle={vehicle ? vehicleLabel(vehicle.car_brand?.name, vehicle.car_model?.name, vehicle.model_year) : ''}
                  onPress={() => setStep('vehicle')}
                />
              </View>
            ) : null}

            <SlotPicker value={slot} onChange={setSlot} />

            <Input
              label={t('note_label')}
              placeholder={t('note_placeholder')}
              value={note ?? current?.note ?? ''}
              onChangeText={setNote}
              multiline
            />
            <Button
              size="lg"
              title={t(edit ? 'save_reschedule' : 'save')}
              disabled={!canSave}
              loading={save.isPending}
              onPress={() => save.mutate()}
            />
          </View>
        ) : null}
          </>
        )}
      </Screen>
    </>
  );
}

function Summary({ icon, title, subtitle, onPress }: { icon: IconName; title: string; subtitle?: string; onPress?: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      disabled={!onPress}
      onPress={onPress}
      className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 active:opacity-70">
      <Icon name={icon} tone="primary" size={20} />
      <View className="flex-1">
        <Text variant="body" numberOfLines={1}>
          {title}
        </Text>
        {subtitle ? (
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {subtitle}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
}

/** Day strip plus the free times of the chosen day. */
function SlotPicker({ value, onChange }: { value: string | null; onChange: (start: string | null) => void }) {
  const { t, i18n } = useTranslation('appointments');
  const q = useAvailability();
  const zone = q.data?.find((d) => d.timezone)?.timezone;
  const today = todayIn(zone);
  const [picked, setDay] = useState<string | null>(null);
  const day = picked ?? today;
  // Days before the organization's today (the request has a day of slack) are not offered.
  const days: AvailabilityDay[] = (q.data ?? []).filter((d) => d.date >= today);
  const chosen = days.find((d) => d.date === day);

  return (
    <View className="gap-3">
      <Text variant="headline">{t('pick_day')}</Text>
      <View className="flex-row flex-wrap gap-2">
        {days.map((d) => {
          const on = d.date === day;
          const off = d.closed || d.remaining_capacity <= 0;
          const date = parseDay(d.date);
          return (
            <Pressable
              key={d.date}
              accessibilityRole="button"
              accessibilityState={{ selected: on, disabled: off }}
              onPress={() => {
                setDay(d.date);
                onChange(null);
              }}
              className={cn(
                'w-[13%] min-w-12 items-center rounded-lg border py-2',
                on ? 'border-primary bg-primary' : 'border-border bg-card',
                off && !on && 'opacity-40',
              )}>
              <Text variant="caption" tone={on ? 'onPrimary' : 'muted'}>
                {new Intl.DateTimeFormat(i18n.language, { weekday: 'short' }).format(date)}
              </Text>
              <Text variant="headline" tone={on ? 'onPrimary' : 'default'}>
                {date.getDate()}
              </Text>
            </Pressable>
          );
        })}
      </View>

      <Text variant="headline">{t('pick_time')}</Text>
      {q.isPending ? (
        <Text variant="callout" tone="muted">
          …
        </Text>
      ) : q.isError ? (
        <Text variant="callout" tone="danger">
          {t('error_generic')}
        </Text>
      ) : chosen?.closed ? (
        <Text variant="callout" tone="muted">
          {t('day_closed')}
        </Text>
      ) : chosen && chosen.remaining_capacity <= 0 ? (
        <Text variant="callout" tone="muted">
          {t('day_full')}
        </Text>
      ) : chosen && chosen.slots.length > 0 ? (
        <>
          <Text variant="footnote" tone="muted">
            {t('slots_left', { count: chosen.remaining_capacity })}
          </Text>
          <View className="flex-row flex-wrap gap-2">
            {chosen.slots.map((s) => {
              const on = s.start === value;
              return (
                <Pressable
                  key={s.start}
                  accessibilityRole="button"
                  accessibilityState={{ selected: on }}
                  onPress={() => onChange(s.start)}
                  className={cn(
                    'min-h-11 min-w-[22%] items-center justify-center rounded-lg border px-3',
                    on ? 'border-primary bg-primary' : 'border-border bg-card active:bg-muted',
                  )}>
                  <Text variant="callout" tone={on ? 'onPrimary' : 'default'} className="font-semibold">
                    {new Intl.DateTimeFormat(i18n.language, { timeStyle: 'short', timeZone: chosen.timezone }).format(new Date(s.start))}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </>
      ) : (
        <Text variant="callout" tone="muted">
          {t('slots_empty')}
        </Text>
      )}
    </View>
  );
}
