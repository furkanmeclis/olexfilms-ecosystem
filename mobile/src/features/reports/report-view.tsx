import { type Href, router, useSegments } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { ChartCard, Donut, RankBars, TrendChart } from '@/features/analytics-kit/charts';
import { DataTable } from '@/features/analytics-kit/data-table';
import { DeltaBadge, KpiGrid, KpiTile } from '@/features/analytics-kit/kpi-tile';
import { chartSeries, groupTimeSeries, isTimeSeries, type Kpi, kpisOf, type Series, seriesTotal } from '@/features/analytics-kit/envelope';
import { bucketLabel, formatValue, guessGranularity } from '@/features/analytics-kit/format';
import { linkPath } from '@/features/analytics-kit/links';
import type { Column, Row } from '@/features/analytics-kit/table';
import { useChartTheme } from '@/features/analytics-kit/theme';
import { ChartEmpty } from '@/features/analytics-kit/states';
import { kpiIcon } from '@/features/dealer-home/board';
import { Text } from '@/ui/text';

import type { Report } from './api';

/** Opens a KPI's link (TEC-1210 `kpis[].link`) in the current space, when it has a screen there. */
export function useKpiPress() {
  const space = (useSegments() as string[])[1] ?? 'bayi';
  return (k: Kpi) => {
    const path = k.link ? linkPath(space, k.link.kind, k.link.uuid, k.link.report) : null;
    return path ? () => router.push(path as Href) : undefined;
  };
}

/** KPI tiles of a report (`kpis[]`, else the legacy `cards` series). */
export function KpiTiles({ kpis, max }: { kpis: Kpi[]; max?: number }) {
  const press = useKpiPress();
  const shown = max ? kpis.slice(0, max) : kpis;
  if (!shown.length) return null;
  return (
    <KpiGrid>
      {shown.map((k) => (
        <KpiTile key={k.key} kpi={k} icon={kpiIcon(k.key)} onPress={press(k)} />
      ))}
    </KpiGrid>
  );
}

/** Time series as a table: one row per bucket, a column per series (and its previous period). */
export function seriesTable(series: Series[], gran: string, locale: string, previousLabel: string, dateLabel: string): { columns: Column[]; rows: Row[] } {
  const base = series[0];
  const withPrev = series.filter((s) => s.points.some((p) => p.previous !== null && p.previous !== undefined));
  const columns: Column[] = [
    { key: '__bucket', label: dateLabel, type: 'text' },
    ...series.map((s) => ({ key: s.key, label: s.label, type: 'number', aggregate: 'sum' as const })),
    ...withPrev.map((s) => ({ key: `${s.key}__prev`, label: `${s.label} (${previousLabel})`, type: 'number', aggregate: 'sum' as const })),
  ];
  const rows: Row[] = (base?.points ?? []).map((p, i) => {
    const row: Row = { __bucket: bucketLabel(p.key, gran, locale, true), __key: p.key };
    for (const s of series) row[s.key] = s.points[i]?.value ?? null;
    for (const s of withPrev) row[`${s.key}__prev`] = s.points[i]?.previous ?? null;
    return row;
  });
  return { columns, rows };
}

type Mode = 'line' | 'bar' | 'table';

function ModeSwitch({ value, onChange }: { value: Mode; onChange: (m: Mode) => void }) {
  const { t } = useTranslation('analytics');
  const items: { key: Mode; icon: 'chart' | 'report' | 'stock' }[] = [
    { key: 'line', icon: 'chart' },
    { key: 'bar', icon: 'stock' },
    { key: 'table', icon: 'report' },
  ];
  return (
    <View className="flex-row rounded-md bg-muted p-0.5">
      {items.map((it) => {
        const on = it.key === value;
        return (
          <Pressable
            key={it.key}
            accessibilityRole="button"
            accessibilityLabel={t(`mode.${it.key}`)}
            accessibilityState={{ selected: on }}
            onPress={() => onChange(it.key)}
            className={`min-h-8 min-w-9 items-center justify-center rounded-sm px-2 ${on ? 'bg-card' : ''}`}>
            <Text variant="footnote" className={on ? 'font-semibold' : 'text-muted-foreground'}>
              {t(`mode.${it.key}`)}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

/**
 * Series over time that share buckets and unit, in one card: the total and its
 * delta, a line / bar / table switch, the chart with the previous period.
 */
export function TrendCard({
  series,
  title,
  granularity,
  height,
  defaultMode = 'line',
  switchable = true,
}: {
  series: Series[];
  title?: string;
  granularity?: string | null;
  height?: number;
  defaultMode?: Mode;
  switchable?: boolean;
}) {
  const th = useChartTheme();
  const { t } = useTranslation('analytics');
  const [mode, setMode] = useState<Mode>(defaultMode);
  const base = series[0];
  if (!base) return null;
  const gran = granularity ?? guessGranularity(base.points.map((p) => p.key));
  const single = series.length === 1;
  const total = single ? seriesTotal(base) : null;
  const delta = single ? base.delta_pct : null;
  const table = mode === 'table' ? seriesTable(series, gran, th.locale, t('previous_period'), t('bucket')) : null;

  return (
    <ChartCard title={title ?? (single ? base.label : undefined)} trailing={switchable ? <ModeSwitch value={mode} onChange={setMode} /> : undefined}>
      {total !== null ? (
        <View className="flex-row flex-wrap items-center gap-2">
          <Text variant="title" className="font-bold tabular-nums">
            {formatValue(total, base.unit, th.locale)}
          </Text>
          <DeltaBadge pct={delta} />
          {base.previous_total !== null && base.previous_total !== undefined ? (
            <Text variant="footnote" tone="muted" className="tabular-nums">
              {t('previous_value', { value: formatValue(base.previous_total, base.unit, th.locale) })}
            </Text>
          ) : null}
        </View>
      ) : null}
      {table ? (
        <DataTable columns={table.columns} rows={table.rows} searchable={false} />
      ) : (
        <TrendChart series={series} kind={mode === 'bar' ? 'bar' : 'line'} granularity={gran} height={height} />
      )}
    </ChartCard>
  );
}

/** Renders any report by its kind and the shape of its series (TEC-1213). */
export function ReportView({ report }: { report: Report }) {
  const { t } = useTranslation('analytics');
  const kpis = kpisOf(report);
  const series = chartSeries(report);
  const groups = groupTimeSeries(series);
  const others = series.filter((s) => !isTimeSeries(s));
  const hasTable = report.columns.length > 0 && report.rows.length > 0;
  const empty = !kpis.length && !series.length && !hasTable;

  return (
    <View className="gap-3">
      <KpiTiles kpis={kpis} />
      {groups.map((g) => (
        <TrendCard
          key={g.map((s) => s.key).join('+')}
          series={g}
          title={g.length > 1 ? (report.kind === 'timeseries' ? report.title : g.map((s) => s.label).join(' · ')) : undefined}
          granularity={report.granularity}
        />
      ))}
      {others.map((s) =>
        report.kind === 'distribution' || s.key.endsWith('status') || s.key.includes('distribution') ? (
          <ChartCard key={s.key} title={s.label}>
            <Donut items={s.points} unit={s.unit} />
          </ChartCard>
        ) : report.kind === 'ranking' && hasTable ? null : (
          <ChartCard key={s.key} title={s.label}>
            <RankBars items={s.points.map((p) => ({ key: p.key, label: p.label, value: p.value }))} unit={s.unit} />
          </ChartCard>
        ),
      )}
      {hasTable ? <DataTable columns={report.columns} rows={report.rows} /> : null}
      {empty ? (
        <ChartCard>
          <ChartEmpty text={t('empty')} />
        </ChartCard>
      ) : null}
    </View>
  );
}
