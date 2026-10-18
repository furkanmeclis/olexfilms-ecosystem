import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useCan } from '@/core/spaces';
import { ErrorState, InfoRow, ListSkeleton } from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Input } from '@/ui/input';
import { List } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { useClock } from './appointments-screen';
import { findAppointment } from './data';
import {
  type Appointment,
  type AppointmentStatus,
  bookingErrorKey,
  canReschedule,
  canStartIntake,
  nextStatuses,
  statusTone,
} from './lib';

/** Booking error as text: our own wording for the known codes, else generic. */
export function useBookingError() {
  const { t } = useTranslation('appointments');
  return (e: unknown) => {
    const key = isApiError(e) ? bookingErrorKey(e.code) : null;
    return t(key ?? 'error_generic');
  };
}

export function AppointmentDetailScreen() {
  const { t } = useTranslation('appointments');
  const clock = useClock();
  const qc = useQueryClient();
  const bookingError = useBookingError();
  const canWrite = useCan('appointments.write');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const [cancelling, setCancelling] = useState(false);
  const [reason, setReason] = useState('');

  const q = useQuery({
    queryKey: ['appointments', 'detail', uuid],
    queryFn: () => {
      const cached = qc
        .getQueriesData<Appointment[]>({ queryKey: ['appointments', 'list'] })
        .flatMap(([, rows]) => rows ?? []);
      return findAppointment(uuid!, cached);
    },
    enabled: !!uuid,
  });
  const a = q.data;

  const saved = (next: Appointment) => {
    qc.setQueryData(['appointments', 'detail', uuid], next);
    void qc.invalidateQueries({ queryKey: ['appointments', 'list'] });
  };

  const move = useMutation({
    mutationFn: (v: { status: AppointmentStatus; reason?: string }) =>
      unwrap(
        api.POST('/v1/appointments/{uuid}/status', {
          params: { path: { uuid: uuid! } },
          body: { status: v.status, cancel_reason: v.reason || undefined },
        }),
      ),
    onSuccess: (next) => {
      setCancelling(false);
      saved(next);
    },
    onError: (e) => Alert.alert(bookingError(e)),
  });

  const intake = useMutation({
    mutationFn: () => unwrap(api.POST('/v1/appointments/{uuid}/start-intake', { params: { path: { uuid: uuid! } } })),
    onSuccess: (next) => {
      saved(next);
      if (next.service_uuid) router.push(`/bayi/services/${next.service_uuid}` as never);
    },
    onError: (e) => Alert.alert(bookingError(e)),
  });

  const confirmStatus = (status: AppointmentStatus) => {
    if (status === 'cancelled') {
      setCancelling(true);
      return;
    }
    if (status === 'no_show') {
      Alert.alert(t('no_show_title'), undefined, [
        { text: t('cancel_keep'), style: 'cancel' },
        { text: t('no_show_confirm'), style: 'destructive', onPress: () => move.mutate({ status }) },
      ]);
      return;
    }
    move.mutate({ status });
  };

  const busy = move.isPending || intake.isPending;
  const actions = a && canWrite ? nextStatuses(a.status) : [];
  const vehicle = a ? [a.vehicle_plate, a.vehicle_label].filter(Boolean).join(' · ') : '';

  return (
    <>
      <Stack.Screen options={{ title: t('detail_title') }} />
      <Screen refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        {q.isPending ? (
          <ListSkeleton rows={4} inset={false} />
        ) : q.isError ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : !a ? (
          <EmptyState icon="appointment" title={t('error_generic')} />
        ) : (
          <>
            <View className="gap-2">
              <Text variant="title">{a.customer_name || '–'}</Text>
              <StatusPill label={t(`status.${a.status}`)} tone={statusTone(a.status)} />
            </View>

            <List>
              <InfoRow label={t('when')} value={clock.dateTime(a.starts_at)} />
              <InfoRow label={t('duration')} value={t('minutes', { count: a.estimated_minutes })} />
              <InfoRow label={t('vehicle')} value={vehicle || t('no_vehicle')} />
              <InfoRow label={t('source')} value={t(`source_value.${a.source}`)} />
              {a.status === 'cancelled' && a.cancel_reason ? (
                <InfoRow label={t('cancel_reason')} value={a.cancel_reason} />
              ) : null}
              <InfoRow label={t('note')} value={a.note} last />
            </List>

            {canWrite ? (
              <View className="gap-2.5">
                {canStartIntake(a) ? (
                  <Button size="lg" title={t('action.start_intake')} loading={intake.isPending} onPress={() => intake.mutate()} />
                ) : null}
                {a.service_uuid ? (
                  <Button
                    title={t('action.open_service')}
                    variant="outline"
                    onPress={() => router.push(`/bayi/services/${a.service_uuid}` as never)}
                  />
                ) : null}
                {actions
                  .filter((s) => s !== 'cancelled')
                  .map((s, i) => (
                    <Button
                      key={s}
                      size={i === 0 && !canStartIntake(a) ? 'lg' : 'md'}
                      variant={i === 0 && !canStartIntake(a) ? 'primary' : s === 'no_show' ? 'destructive' : 'outline'}
                      title={t(`action.${s}`)}
                      disabled={busy}
                      loading={move.isPending && move.variables?.status === s}
                      onPress={() => confirmStatus(s)}
                    />
                  ))}
                {canReschedule(a) ? (
                  <Button
                    title={t('action.reschedule')}
                    variant="outline"
                    disabled={busy}
                    onPress={() => router.push(`/bayi/menu/appointments/${a.uuid}/edit` as never)}
                  />
                ) : null}
                {actions.includes('cancelled') ? (
                  cancelling ? (
                    <View className="gap-3 rounded-lg border border-border bg-card p-4">
                      <Text variant="headline">{t('cancel_title')}</Text>
                      <Input
                        label={t('cancel_reason_label')}
                        placeholder={t('cancel_reason_placeholder')}
                        value={reason}
                        onChangeText={setReason}
                        multiline
                      />
                      <Button
                        variant="destructive"
                        title={t('cancel_confirm')}
                        loading={move.isPending}
                        onPress={() => move.mutate({ status: 'cancelled', reason: reason.trim() })}
                      />
                      <Button variant="ghost" title={t('cancel_keep')} onPress={() => setCancelling(false)} />
                    </View>
                  ) : (
                    <Button
                      variant="ghost"
                      title={t('action.cancelled')}
                      disabled={busy}
                      onPress={() => confirmStatus('cancelled')}
                    />
                  )
                ) : null}
              </View>
            ) : null}
          </>
        )}
      </Screen>
    </>
  );
}
