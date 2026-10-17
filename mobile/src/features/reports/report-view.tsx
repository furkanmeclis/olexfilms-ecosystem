import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';
import Svg, { Rect } from 'react-native-svg';
import { useCSSVariable } from 'uniwind';

import { useFormat } from '@/features/admin/kit';
import { Text } from '@/ui/text';

import type { Report, ReportSeries } from './api';

const DATE_KEY = /^\d{4}-\d{2}-\d{2}/;

function isTimeSeries(s: ReportSeries) {
  return s.points.length > 1 && s.points.every((p) => DATE_KEY.test(p.key));
}

/** Stat tiles: the `cards` series of summary reports and the overview. */
export function StatTiles({ series }: { series: ReportSeries }) {
  const f = useFormat();
  return (
    <View className="flex-row flex-wrap gap-2.5">
      {series.points.map((p) => (
        <View key={p.key} className="min-h-20 grow basis-[46%] justify-between rounded-lg border border-border bg-card p-3.5">
          <Text variant="footnote" tone="muted" numberOfLines={2}>
            {p.label}
          </Text>
          <Text variant="title" className="tabular-nums">
            {f.number(p.value, 1)}
          </Text>
        </View>
      ))}
    </View>
  );
}

/** Bars over time, drawn with react-native-svg (works in Expo Go). */
export function TimeChart({ series, compact }: { series: ReportSeries; compact?: boolean }) {
  const f = useFormat();
  const { t } = useTranslation('admin');
  const [width, setWidth] = useState(0);
  const primary = useCSSVariable('--color-primary');
  const muted = useCSSVariable('--color-muted');
  const height = compact ? 56 : 120;
  const values = series.points.map((p) => p.value);
  const max = Math.max(1, ...values);
  const total = values.reduce((a, b) => a + b, 0);
  const gap = series.points.length > 40 ? 1 : 2;
  const barW = width > 0 ? Math.max(1, (width - gap * (series.points.length - 1)) / series.points.length) : 0;
  const first = series.points[0];
  const last = series.points[series.points.length - 1];

  return (
    <View className="gap-2 rounded-lg border border-border bg-card p-3.5">
      <View className="flex-row items-baseline justify-between gap-2">
        <Text variant="callout" className="flex-1 font-semibold" numberOfLines={1}>
          {series.label}
        </Text>
        <Text variant="footnote" tone="muted">
          {t('reports.total', { value: f.number(total, 1) })}
        </Text>
      </View>
      <View onLayout={(e) => setWidth(e.nativeEvent.layout.width)} style={{ height }}>
        {width > 0 ? (
          <Svg width={width} height={height}>
            {series.points.map((p, i) => {
              const h = p.value > 0 ? Math.max(2, (p.value / max) * height) : 1;
              return (
                <Rect
                  key={p.key}
                  x={i * (barW + gap)}
                  y={height - h}
                  width={barW}
                  height={h}
                  rx={Math.min(2, barW / 2)}
                  fill={(p.value > 0 ? primary : muted) as string}
                />
              );
            })}
          </Svg>
        ) : null}
      </View>
      {first && last ? (
        <View className="flex-row justify-between">
          <Text variant="footnote" tone="muted">
            {f.date(first.key)}
          </Text>
          <Text variant="footnote" tone="muted">
            {t('reports.peak', { value: f.number(max, 1) })}
          </Text>
          <Text variant="footnote" tone="muted">
            {f.date(last.key)}
          </Text>
        </View>
      ) : null}
    </View>
  );
}

/** Horizontal bars for distributions and rankings. */
export function BarList({ series }: { series: ReportSeries }) {
  const f = useFormat();
  const max = Math.max(1, ...series.points.map((p) => p.value));
  return (
    <View className="gap-3 rounded-lg border border-border bg-card p-3.5">
      <Text variant="callout" className="font-semibold">
        {series.label}
      </Text>
      {series.points.map((p) => (
        <View key={p.key} className="gap-1">
          <View className="flex-row justify-between gap-2">
            <Text variant="footnote" className="flex-1" numberOfLines={1}>
              {p.label}
            </Text>
            <Text variant="footnote" className="font-semibold tabular-nums">
              {f.number(p.value, 1)}
            </Text>
          </View>
          <View className="h-2 overflow-hidden rounded-full bg-muted">
            <View className="h-2 rounded-full bg-primary" style={{ width: `${(p.value / max) * 100}%` }} />
          </View>
        </View>
      ))}
    </View>
  );
}

function cell(value: unknown, type: string, f: ReturnType<typeof useFormat>) {
  if (value === null || value === undefined || value === '') return '–';
  if (type === 'datetime' && typeof value === 'string') return f.dateTime(value);
  if ((type === 'integer' || type === 'number') && (typeof value === 'number' || typeof value === 'string'))
    return f.number(value, 2);
  return String(value);
}

/** Table rows as cards: the first column is the title, the rest label / value. */
export function TableView({ report }: { report: Report }) {
  const f = useFormat();
  const [head, ...rest] = report.columns;
  if (!head) return null;
  return (
    <View className="gap-2.5">
      {report.rows.map((row, i) => (
        <View key={i} className="gap-1 rounded-lg border border-border bg-card px-4 py-3">
          <Text variant="headline" numberOfLines={1}>
            {cell(row[head.key], head.type, f)}
          </Text>
          {rest.map((c) => (
            <View key={c.key} className="flex-row justify-between gap-3">
              <Text variant="footnote" tone="muted">
                {c.label}
              </Text>
              <Text variant="footnote" className="shrink text-right" numberOfLines={1}>
                {cell(row[c.key], c.type, f)}
              </Text>
            </View>
          ))}
        </View>
      ))}
    </View>
  );
}

/** Renders any report by its kind and the shape of its series. */
export function ReportView({ report }: { report: Report }) {
  const { t } = useTranslation('admin');
  const series = report.series.filter((s) => s.points.length > 0);
  const hasTable = report.kind === 'table' && report.columns.length > 0;
  const empty = !hasTable && series.length === 0 && report.rows.length === 0;

  return (
    <View className="gap-3">
      {series.map((s) =>
        s.key === 'cards' ? (
          <StatTiles key={s.key} series={s} />
        ) : isTimeSeries(s) ? (
          <TimeChart key={s.key} series={s} />
        ) : report.kind === 'ranking' && report.columns.length > 0 ? null : (
          <BarList key={s.key} series={s} />
        ),
      )}
      {(hasTable || (report.kind === 'ranking' && report.columns.length > 0)) && report.rows.length > 0 ? (
        <TableView report={report} />
      ) : null}
      {empty || (hasTable && report.rows.length === 0) ? (
        <Text variant="callout" tone="muted" className="py-6 text-center">
          {t('reports.no_data')}
        </Text>
      ) : null}
    </View>
  );
}
