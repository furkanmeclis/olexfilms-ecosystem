import { keepPreviousData, useQueries, useQueryClient } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { useCan } from '@/core/spaces';
import { addDays, parseDay, startOfWeek, todayIn, ymd } from '@/features/appointments/lib';
import { Chips, ErrorState, InfoRow, ListSkeleton, useFormat } from '@/features/admin/kit';
import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { List } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { aggregateOccupancy, type OccupancyLevel, occupancyPercent, summarizeOccupancy } from './lib';

type View_ = 'day' | 'week';

const BAR: Record<OccupancyLevel, string> = {
  none: 'bg-muted',
  low: 'bg-success',
  medium: 'bg-warning',
  high: 'bg-warning',
  full: 'bg-destructive',
};
const TONE: Record<OccupancyLevel, StatusTone> = {
  none: 'neutral',
  low: 'done',
  medium: 'progress',
  high: 'progress',
  full: 'problem',
};

function Bar({ level, percent }: { level: OccupancyLevel; percent: number | null }) {
  return (
    <View className="h-2 overflow-hidden rounded-full bg-secondary">
      <View className={cn('h-2 rounded-full', BAR[level])} style={{ width: `${Math.min(percent ?? 0, 100)}%` }} />
    </View>
  );
}

function Step({ icon, label, onPress }: { icon: 'back' | 'forward'; label: string; onPress: () => void }) {
  return (
    <Pressable accessibilityRole="button" accessibilityLabel={label} hitSlop={8} onPress={onPress} className="size-11 items-center justify-center">
      <Icon name={icon} tone="primary" />
    </Pressable>
  );
}

/**
 * Appointment occupancy of the network (center: every organization, distributor:
 * its dealers) for a day or a week, as on the web (GET /v1/appointments/occupancy).
 * Read only; the endpoint answers per day, so a week sums seven requests.
 */
export function OccupancyScreen() {
  const { t } = useTranslation('governance');
  const f = useFormat();
  const qc = useQueryClient();
  const canRead = useCan('appointments.read');
  const [view, setView] = useState<View_>('day');
  const [anchor, setAnchor] = useState(() => todayIn());

  const days = useMemo(() => {
    if (view === 'day') return [anchor];
    const monday = startOfWeek(parseDay(anchor));
    return Array.from({ length: 7 }, (_, i) => ymd(addDays(monday, i)));
  }, [view, anchor]);

  const queries = useQueries({
    queries: days.map((date) => ({
      queryKey: ['admin-occupancy', date],
      queryFn: () => unwrap(api.GET('/v1/appointments/occupancy', { params: { query: { date } } })),
      enabled: canRead,
      placeholderData: keepPreviousData,
    })),
  });
  const pending = queries.some((q) => q.isPending);
  const failed = queries.find((q) => q.isError && !q.data);
  const refreshing = queries.some((q) => q.isRefetching);
  const key = queries.map((q) => q.dataUpdatedAt).join(',');
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const rows = useMemo(() => aggregateOccupancy(queries.map((q) => q.data ?? [])), [key]);
  const total = summarizeOccupancy(rows);

  const shift = (dir: 1 | -1) => setAnchor(ymd(addDays(parseDay(anchor), dir * (view === 'week' ? 7 : 1))));
  const label =
    view === 'day'
      ? f.date(anchor)
      : `${f.date(days[0])} – ${f.date(days[days.length - 1])}`;
  const pct = (r: number | null) => {
    const p = occupancyPercent(r);
    return p === null ? '–' : `${f.number(p)}%`;
  };

  return (
    <>
      <Stack.Screen options={{ title: t('occupancy.title') }} />
      <Screen
        refreshControl={
          <RefreshControl refreshing={refreshing} onRefresh={() => void qc.invalidateQueries({ queryKey: ['admin-occupancy'] })} />
        }>
        {!canRead ? (
          <EmptyState icon="blocked" title={t('no_access')} />
        ) : (
          <>
            <Chips
              options={[
                { key: 'day', label: t('occupancy.day') },
                { key: 'week', label: t('occupancy.week') },
              ]}
              value={view}
              onChange={setView}
            />
            <View className="flex-row items-center justify-between px-2">
              <Step icon="back" label={t('occupancy.previous')} onPress={() => shift(-1)} />
              <Pressable accessibilityRole="button" onPress={() => setAnchor(todayIn())}>
                <Text variant="headline">{label}</Text>
              </Pressable>
              <Step icon="forward" label={t('occupancy.next')} onPress={() => shift(1)} />
            </View>
            {pending && !rows.length ? (
              <ListSkeleton rows={5} inset={false} />
            ) : failed ? (
              <ErrorState error={failed.error} onRetry={() => queries.forEach((q) => void q.refetch())} />
            ) : !rows.length ? (
              <EmptyState icon="calendar" title={t('occupancy.empty')} />
            ) : (
              <>
                <List>
                  <InfoRow label={t('occupancy.organizations')} value={f.number(total.organizations)} />
                  <InfoRow label={t('occupancy.booked')} value={`${f.number(total.occupied)} / ${f.number(total.capacity)}`} />
                  <InfoRow label={t('occupancy.rate')} value={pct(total.rate)} last />
                </List>
                <View className="gap-2.5">
                  {rows.map((r) => (
                    <View key={r.organization_uuid} className="gap-2 rounded-lg border border-border bg-card px-4 py-3">
                      <View className="flex-row items-center justify-between gap-3">
                        <Text variant="headline" numberOfLines={1} className="flex-1">
                          {r.organization_name}
                        </Text>
                        <StatusPill label={pct(r.rate)} tone={TONE[r.level]} />
                      </View>
                      <Bar level={r.level} percent={occupancyPercent(r.rate)} />
                      <Text variant="footnote" tone="muted">
                        {r.capacity > 0
                          ? t('occupancy.row', { occupied: f.number(r.occupied), capacity: f.number(r.capacity), remaining: f.number(r.remaining) })
                          : t('occupancy.no_capacity')}
                      </Text>
                    </View>
                  ))}
                </View>
              </>
            )}
          </>
        )}
      </Screen>
    </>
  );
}
