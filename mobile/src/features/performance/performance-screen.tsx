import { useQuery } from '@tanstack/react-query';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api } from '@/core/api/client';
import type { paths } from '@/core/api/schema';
import { useCan } from '@/core/spaces';
import { Chips, PagedList, PAGE_SIZE, useFormat, usePagedQuery } from '@/features/admin/kit';
import { EmptyState } from '@/ui/empty-state';
import { Text } from '@/ui/text';

import { unwrapPerformance } from './api';

import { formatMetric, type Metric, type MetricValue, recentMonths, ROW_METRICS } from './lib';

type Row = NonNullable<
  paths['/v1/performance/ranking']['get']['responses'][200]['content']['application/json']['data']
>['items'] extends (infer I)[] | undefined
  ? I
  : never;

type Kind = 'all' | 'dealer' | 'distributor';

/** Network totals of the month: the numbers the dashboard answers for the caller's organization. */
function Summary({ period }: { period: string }) {
  const { t } = useTranslation('governance');
  const f = useFormat();
  const q = useQuery({
    queryKey: ['performance', 'dashboard', period],
    queryFn: () => unwrapPerformance(api.GET('/v1/performance/dashboard', { params: { query: { period } } })),
  });
  const metrics = (q.data?.metrics ?? {}) as Record<string, MetricValue>;
  const shown = ROW_METRICS.filter((m) => metrics[m]?.value !== undefined && metrics[m]?.value !== '');
  if (!shown.length) return null;
  return (
    <View className="flex-row flex-wrap gap-2.5 px-4 pb-2">
      {shown.map((m) => (
        <View key={m} className="min-w-[46%] flex-1 gap-0.5 rounded-lg border border-border bg-card px-4 py-3">
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {t(`performance.metric.${m}`)}
          </Text>
          <Text variant="headline" className="tabular-nums">
            {formatMetric(m, metrics[m], f)}
          </Text>
        </View>
      ))}
    </View>
  );
}

function RankingRow({ row }: { row: Row }) {
  const { t } = useTranslation('governance');
  const f = useFormat();
  const metrics = (row.metrics ?? {}) as Record<string, MetricValue>;
  const shown: Metric[] = ROW_METRICS.filter((m) => metrics[m]?.value !== undefined && metrics[m]?.value !== '').slice(0, 4);
  return (
    <View className="mx-4 gap-2 rounded-lg border border-border bg-card px-4 py-3">
      <View className="flex-row items-center gap-3">
        <View className="size-8 items-center justify-center rounded-full bg-secondary">
          <Text variant="footnote" className="font-semibold tabular-nums">
            {row.rank ?? '–'}
          </Text>
        </View>
        <View className="flex-1 gap-0.5">
          <Text variant="headline" numberOfLines={1}>
            {row.name}
          </Text>
          <Text variant="footnote" tone="muted" numberOfLines={1}>
            {[t(`performance.type.${row.type ?? 'dealer'}`, { defaultValue: row.type ?? '' }), row.province_name, row.type === 'dealer' ? row.distributor?.name : null]
              .filter(Boolean)
              .join(' · ')}
          </Text>
        </View>
      </View>
      {shown.length ? (
        <View className="flex-row flex-wrap gap-y-1.5">
          {shown.map((m) => (
            <View key={m} className="w-1/2 gap-0.5">
              <Text variant="caption" tone="muted" numberOfLines={1}>
                {t(`performance.metric.${m}`)}
              </Text>
              <Text variant="callout" className="font-semibold tabular-nums">
                {formatMetric(m, metrics[m], f, row.currency)}
              </Text>
            </View>
          ))}
        </View>
      ) : (
        <Text variant="footnote" tone="muted">
          {t('performance.no_metrics')}
        </Text>
      )}
    </View>
  );
}

/**
 * Dealer performance of the network for a month: summary cards and the ranking
 * with each organization's key metrics. Read only (targets, bonuses and rules
 * stay on the web); simple cards, no chart.
 */
export function PerformanceScreen() {
  const { t } = useTranslation('governance');
  const canRead = useCan('performance.read');
  const months = recentMonths(new Date());
  const [period, setPeriod] = useState(months[0]!);
  const [kind, setKind] = useState<Kind>('all');

  const list = usePagedQuery<Row>(
    ['performance', 'ranking', period, kind],
    async (offset) => {
      const page = await unwrapPerformance(
        api.GET('/v1/performance/ranking', {
          params: { query: { period, limit: PAGE_SIZE, offset, type: kind === 'all' ? undefined : kind } },
        }),
      );
      return { items: page.items ?? [], total: page.total ?? 0, limit: page.limit ?? PAGE_SIZE, offset: page.offset ?? offset };
    },
    canRead,
  );

  const header = (
    <View>
      <Chips options={months.map((m) => ({ key: m, label: m }))} value={period} onChange={setPeriod} />
      <Chips
        options={[
          { key: 'all', label: t('performance.type.all') },
          { key: 'dealer', label: t('performance.type.dealer') },
          { key: 'distributor', label: t('performance.type.distributor') },
        ]}
        value={kind}
        onChange={setKind}
      />
      <Summary period={period} />
    </View>
  );

  return (
    <>
      <Stack.Screen options={{ title: t('performance.title') }} />
      {canRead ? (
        <PagedList
          query={list}
          keyOf={(r) => r.organization_uuid ?? String(r.rank)}
          header={header}
          empty={{ icon: 'chart', title: t('performance.empty') }}
          renderItem={({ item }) => <RankingRow row={item} />}
        />
      ) : (
        <EmptyState icon="blocked" title={t('no_access')} />
      )}
    </>
  );
}
