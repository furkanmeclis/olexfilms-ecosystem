"use client";

import { useMemo } from "react";
import {
  Bar,
  BarChart as RBarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { useFormatter, useLocale } from "@/providers/locale-provider";

import {
  LEGEND_MIN_SERIES,
  bandCursor,
  gridProps,
  xAxisProps,
  yAxisProps,
} from "./chart-axes";
import { ChartFrame, type ChartTable, type LegendItem } from "./chart-frame";
import {
  formatAxisValue,
  formatBucket,
  formatValue,
  isNumber,
  type Granularity,
  type ValueFormat,
} from "./format";
import { foldSeries } from "./palette";
import type { ChartChromeProps, ChartDatum, ChartSeriesDef } from "./types";
import { useDurationLabels } from "./use-duration-labels";
import { useSeriesColors } from "./use-series-colors";
import { VizTooltipCard } from "./viz-tooltip";

export type BarChartProps = ChartChromeProps & {
  data: ChartDatum[];
  series: ChartSeriesDef[];
  /** Stack the series (part-to-whole per bucket). */
  stacked?: boolean;
  /** Keys are ISO dates of this granularity; omit for categories. */
  granularity?: Granularity;
  format?: ValueFormat;
};

const RADIUS_TOP: [number, number, number, number] = [4, 4, 0, 0];

/**
 * Vertical bars, grouped or stacked. Bars sit on the baseline with 4px
 * rounded tops; neighbouring fills keep a 2px surface gap.
 */
export function BarChart({
  data,
  series: seriesProp,
  stacked,
  granularity,
  format,
  ...chrome
}: BarChartProps) {
  const fmt = useFormatter();
  const { t } = useLocale();
  const durations = useDurationLabels();
  const folded = useMemo(
    () => foldSeries(data, seriesProp, t("chart.other")),
    [data, seriesProp, t],
  );
  const series = folded.series as ChartSeriesDef[];
  const rows = folded.rows as ChartDatum[];
  const colors = useSeriesColors(series);
  const valueFormat = format ?? series[0]?.format;

  const label = (d: ChartDatum, long = false) =>
    granularity
      ? formatBucket(d.key, granularity, fmt, long)
      : String(d.label ?? d.key);

  const isEmpty =
    rows.length === 0 ||
    !rows.some((r) => series.some((s) => isNumber(r[s.key])));

  const legend: LegendItem[] =
    series.length >= LEGEND_MIN_SERIES
      ? series.map((s) => ({
          key: s.key,
          label: s.label,
          color: colors[s.key] ?? "var(--viz-other)",
        }))
      : [];

  const table: ChartTable = {
    columns: [
      { key: "__x", label: t("chart.category") },
      ...series.map((s) => ({ key: s.key, label: s.label, numeric: true })),
      ...(stacked && series.length > 1
        ? [{ key: "__total", label: t("chart.total"), numeric: true }]
        : []),
    ],
    rows: rows.map((r) => {
      const values = series.map((s) => r[s.key] as number | null);
      const total = values.reduce<number>(
        (acc, v) => acc + (isNumber(v) ? v : 0),
        0,
      );
      return {
        key: r.key,
        cells: [
          label(r, true),
          ...series.map((s, i) =>
            formatValue(values[i], s.format ?? valueFormat, fmt, durations),
          ),
          ...(stacked && series.length > 1
            ? [formatValue(total, valueFormat, fmt, durations)]
            : []),
        ],
      };
    }),
  };

  return (
    <ChartFrame {...chrome} legend={legend} table={table} isEmpty={isEmpty}>
      <ResponsiveContainer width="100%" height="100%">
        <RBarChart
          data={rows}
          margin={{ top: 8, right: 8, bottom: 0, left: 0 }}
          barGap={2}
          barCategoryGap="20%"
        >
          <CartesianGrid {...gridProps} />
          <XAxis
            {...xAxisProps}
            dataKey="key"
            tickFormatter={(_v, i) => (rows[i] ? label(rows[i]) : "")}
          />
          <YAxis
            {...yAxisProps}
            tickFormatter={(v) =>
              formatAxisValue(Number(v), valueFormat, fmt, durations)
            }
          />
          <Tooltip
            cursor={bandCursor}
            content={(props) => {
              const datum = props.active
                ? (props.payload?.[0]?.payload as ChartDatum | undefined)
                : undefined;
              if (!datum) return null;
              return (
                <VizTooltipCard
                  title={label(datum, true)}
                  rows={series.map((s) => ({
                    key: s.key,
                    label: s.label,
                    color: colors[s.key] ?? "var(--viz-other)",
                    value: datum[s.key] as number | null,
                    format: s.format ?? valueFormat,
                  }))}
                />
              );
            }}
          />
          {series.map((s, i) => (
            <Bar
              key={s.key}
              dataKey={s.key}
              name={s.label}
              fill={colors[s.key]}
              stackId={stacked ? "stack" : undefined}
              radius={!stacked || i === series.length - 1 ? RADIUS_TOP : 0}
              stroke={stacked ? "var(--viz-surface)" : undefined}
              strokeWidth={stacked ? 2 : 0}
              maxBarSize={48}
              isAnimationActive={false}
            />
          ))}
        </RBarChart>
      </ResponsiveContainer>
    </ChartFrame>
  );
}
