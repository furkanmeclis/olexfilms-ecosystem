import { useQuery } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl, View } from 'react-native';

import { useCan } from '@/core/spaces';
import { ErrorState, ListSkeleton } from '@/features/admin/kit';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { loadRange, useOrgTimeZone } from './data';
import { type Appointment, addDays, startOfWeek, statusTone, ymd } from './lib';

/** Formatters in the organization's time zone (device zone until it is known). */
export function useClock() {
  const { i18n } = useTranslation();
  const tz = useOrgTimeZone();
  return {
    time: (iso: string) => new Intl.DateTimeFormat(i18n.language, { timeStyle: 'short', timeZone: tz }).format(new Date(iso)),
    day: (d: Date) => new Intl.DateTimeFormat(i18n.language, { weekday: 'long', day: 'numeric', month: 'long' }).format(d),
    short: (d: Date) => new Intl.DateTimeFormat(i18n.language, { day: 'numeric', month: 'short' }).format(d),
    dateTime: (iso: string) =>
      new Intl.DateTimeFormat(i18n.language, { dateStyle: 'full', timeStyle: 'short', timeZone: tz }).format(new Date(iso)),
  };
}

/** The week of bookings of the active dealer, grouped by day. */
export function AppointmentsScreen() {
  const { t } = useTranslation('appointments');
  const clock = useClock();
  const canWrite = useCan('appointments.write');
  const [weekStart, setWeekStart] = useState(() => startOfWeek(new Date()));
  const weekEnd = addDays(weekStart, 7);

  const q = useQuery({
    queryKey: ['appointments', 'list', ymd(weekStart)],
    queryFn: () => loadRange(weekStart, weekEnd),
  });

  const days = Array.from({ length: 7 }, (_, i) => addDays(weekStart, i));
  const byDay = new Map<string, Appointment[]>();
  for (const a of q.data ?? []) {
    const key = ymd(new Date(a.starts_at));
    byDay.set(key, [...(byDay.get(key) ?? []), a]);
  }
  const today = ymd(new Date());
  const isThisWeek = ymd(startOfWeek(new Date())) === ymd(weekStart);

  return (
    <>
      <Stack.Screen
        options={{
          title: t('title'),
          headerRight: canWrite
            ? () => (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('new')}
                  hitSlop={8}
                  onPress={() => router.push('/bayi/menu/appointments/new')}>
                  <View className="size-8 items-center justify-center">
                    <Icon name="add" tone="primary" />
                  </View>
                </Pressable>
              )
            : undefined,
        }}
      />
      <Screen
        refreshControl={<RefreshControl refreshing={q.isRefetching} onRefresh={() => void q.refetch()} />}>
        <View className="flex-row items-center gap-2">
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('prev_week')}
            onPress={() => setWeekStart(addDays(weekStart, -7))}
            className="size-11 items-center justify-center rounded-full bg-secondary active:opacity-70">
            <Icon name="back" tone="primary" size={18} />
          </Pressable>
          <View className="flex-1 items-center">
            <Text variant="headline">
              {clock.short(weekStart)} – {clock.short(addDays(weekStart, 6))}
            </Text>
            {q.data ? (
              <Text variant="footnote" tone="muted">
                {t('week_count', { count: q.data.length })}
              </Text>
            ) : null}
          </View>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('next_week')}
            onPress={() => setWeekStart(addDays(weekStart, 7))}
            className="size-11 items-center justify-center rounded-full bg-secondary active:opacity-70">
            <Icon name="forward" tone="primary" size={18} />
          </Pressable>
        </View>
        {!isThisWeek ? (
          <Button title={t('today')} variant="outline" onPress={() => setWeekStart(startOfWeek(new Date()))} />
        ) : null}

        {q.isPending ? (
          <ListSkeleton inset={false} />
        ) : q.isError ? (
          <ErrorState error={q.error} onRetry={() => void q.refetch()} />
        ) : q.data.length === 0 ? (
          <EmptyState
            icon="appointment"
            title={t('empty')}
            body={t('empty_body')}
            action={canWrite ? { title: t('new'), onPress: () => router.push('/bayi/menu/appointments/new') } : undefined}
          />
        ) : (
          days.map((d) => {
            const key = ymd(d);
            const rows = byDay.get(key);
            if (!rows) return null;
            return (
              <View key={key} className="gap-2">
                <SectionHeader title={key === today ? `${t('today')} · ${clock.day(d)}` : clock.day(d)} />
                {rows.map((a) => (
                  <AppointmentCard key={a.uuid} a={a} />
                ))}
              </View>
            );
          })
        )}
      </Screen>
    </>
  );
}

function AppointmentCard({ a }: { a: Appointment }) {
  const { t } = useTranslation('appointments');
  const clock = useClock();
  const vehicle = [a.vehicle_plate, a.vehicle_label].filter(Boolean).join(' · ');
  return (
    <Pressable
      accessibilityRole="button"
      onPress={() => router.push(`/bayi/menu/appointments/${a.uuid}?at=${encodeURIComponent(a.starts_at)}` as never)}
      className="flex-row items-center gap-3 rounded-lg border border-border bg-card px-4 py-3 active:opacity-70">
      <View className="w-14">
        <Text variant="headline">{clock.time(a.starts_at)}</Text>
        <Text variant="footnote" tone="muted">
          {t('minutes', { count: a.estimated_minutes })}
        </Text>
      </View>
      <View className="flex-1 gap-0.5">
        <Text variant="body" numberOfLines={1}>
          {a.customer_name || '–'}
        </Text>
        {vehicle ? (
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {vehicle}
          </Text>
        ) : null}
      </View>
      <StatusPill label={t(`status.${a.status}`)} tone={statusTone(a.status)} />
    </Pressable>
  );
}
