import { type Href, router, useSegments } from 'expo-router';
import { View } from 'react-native';

import { ChartCard, RankBars, type RankItem } from '@/features/analytics-kit/charts';
import { chartSeries, groupTimeSeries, kpisOf, rowLink } from '@/features/analytics-kit/envelope';
import { KpiTileSkeleton } from '@/features/analytics-kit/kpi-tile';
import { linkPath } from '@/features/analytics-kit/links';
import { ChartEmpty, ChartError, ChartSkeleton } from '@/features/analytics-kit/states';
import type { Row } from '@/features/analytics-kit/table';

import type { useCatalogReport } from './api';
import { KpiTiles, TrendCard } from './report-view';

type R = ReturnType<typeof useCatalogReport>;

/** KPI tiles of a home report (kpis[] or legacy cards); hidden when the report is not open to the caller. */
export function KpiSection({ r, max, skeleton = 4 }: { r: R; max?: number; skeleton?: number }) {
  if (!r.available && !r.loading) return null;
  if (r.loading && !r.query.data) return <KpiTileSkeleton count={skeleton} />;
  if (r.query.isError && !r.query.data) return <ChartError error={r.query.error} onRetry={() => void r.query.refetch()} />;
  const kpis = r.query.data ? kpisOf(r.query.data) : [];
  if (!kpis.length) return null;
  return <KpiTiles kpis={kpis} max={max} />;
}

/** The time series of a home report in one card (series compared, previous period dashed). */
export function TrendSection({
  r,
  title,
  keys,
  height = 160,
  defaultMode,
}: {
  r: R;
  title?: string;
  /** Series to show (all time series when omitted). */
  keys?: string[];
  height?: number;
  defaultMode?: 'line' | 'bar';
}) {
  if (!r.available && !r.loading) return null;
  const data = r.query.data;
  if (r.loading && !data) {
    return (
      <ChartCard title={title}>
        <ChartSkeleton height={height} />
      </ChartCard>
    );
  }
  if (r.query.isError && !data) {
    return (
      <ChartCard title={title}>
        <ChartError error={r.query.error} onRetry={() => void r.query.refetch()} />
      </ChartCard>
    );
  }
  const series = data ? chartSeries(data).filter((s) => !keys || keys.includes(s.key)) : [];
  const group = groupTimeSeries(series)[0];
  if (!group) {
    return (
      <ChartCard title={title}>
        <ChartEmpty />
      </ChartCard>
    );
  }
  return <TrendCard series={group} title={title ?? (group.length > 1 ? data?.title : undefined)} granularity={data?.granularity} height={height} defaultMode={defaultMode} />;
}

/** Opens a report row's record: `_link`, else `organization_uuid` as an organization. */
export function useRowPress() {
  const space = (useSegments() as string[])[1] ?? 'bayi';
  return (row: Row) => {
    const link = rowLink(row) ?? (typeof row.organization_uuid === 'string' ? { kind: 'organization', uuid: row.organization_uuid } : null);
    const path = link ? linkPath(space, link.kind, link.uuid) : null;
    return path ? () => router.push(path as Href) : undefined;
  };
}

/**
 * Ranking of a home report from its rows (dealers by orders, services by
 * dealer): label, value, an optional second line; rows open their record.
 */
export function RankSection({
  r,
  title,
  toItem,
  max = 5,
  footer,
}: {
  r: R;
  title: string;
  toItem: (row: Row, i: number) => Omit<RankItem, 'onPress'> | null;
  max?: number;
  footer?: React.ReactNode;
}) {
  const press = useRowPress();
  if (!r.available && !r.loading) return null;
  const data = r.query.data;
  let body: React.ReactNode;
  if (r.loading && !data) body = <ChartSkeleton height={120} />;
  else if (r.query.isError && !data) body = <ChartError error={r.query.error} onRetry={() => void r.query.refetch()} />;
  else {
    const items: RankItem[] = [];
    (data?.rows ?? []).slice(0, max).forEach((row, i) => {
      const it = toItem(row, i);
      if (it) items.push({ ...it, onPress: press(row) });
    });
    body = items.length ? <RankBars items={items} /> : <ChartEmpty height={80} />;
  }
  return (
    <ChartCard title={title}>
      <View>{body}</View>
      {footer}
    </ChartCard>
  );
}
