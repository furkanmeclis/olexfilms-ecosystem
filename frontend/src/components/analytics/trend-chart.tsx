"use client";

import { useMemo } from "react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { useFormatter, useLocale } from "@/providers/locale-provider";

import {
  DIRECT_LABEL_MAX_SERIES,
  LEGEND_MIN_SERIES,
  crosshairCursor,
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
import {
  type ChartChromeProps,
  type ChartDatum,
  type ChartSeriesDef,
  previousKey,
} from "./types";
import { useDurationLabels } from "./use-duration-labels";
import { useSeriesColors } from "./use-series-colors";
import { VizTooltipCard } from "./viz-tooltip";

export type TrendChartProps = ChartChromeProps & {
  /** line (default) or area. */
  kind?: "line" | "area";
  data: ChartDatum[];
  series: ChartSeriesDef[];
  /** Bucket keys are ISO dates of this granularity (axis + tooltip labels). */
  granularity?: Granularity;
  /** Keys are plain categories: use `label` as is. */
  categorical?: boolean;
  /** Value format of the (single) y axis. Default: first series' format. */
  format?: ValueFormat;
  /** Draw `previousKey(series.key)` values as dashed lines. */
  showPrevious?: boolean;
  /** Which direction is good (tooltip delta colour). */
  good?: "up" | "down";
  /** Stack areas (area kind only). */
  stacked?: boolean;
};

type LabelProps = { x?: number | string; y?: number | string; index?: number };

/**
 * Line / area over time. One y axis only; the previous period is the same
 * colour, dashed. Legend from 2 series, direct end labels up to 4.
 */
export function TrendChart({
  kind = "line",
  data,
  series: seriesProp,
  granularity,
  categorical,
  format,
  showPrevious,
  good = "up",
  stacked,
  ...chrome
}: TrendChartProps) {
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
  const previousOn =
    Boolean(showPrevious) &&
    rows.some((r) => series.some((s) => isNumber(r[previousKey(s.key)])));

  const label = (d: ChartDatum, long = false) =>
    categorical
      ? String(d.label ?? d.key)
      : formatBucket(d.key, granularity, fmt, long);

  const isEmpty =
    rows.length === 0 ||
    !rows.some((r) => series.some((s) => isNumber(r[s.key])));

  const legend: LegendItem[] =
    series.length >= LEGEND_MIN_SERIES || previousOn
      ? [
          ...series.map((s) => ({
            key: s.key,
            label: s.label,
            color: colors[s.key] ?? "var(--viz-other)",
          })),
          ...(previousOn
            ? [
                {
                  key: "__previous",
                  label: t("chart.previous_period"),
                  color: "var(--viz-muted)",
                  dashed: true,
                },
              ]
            : []),
        ]
      : [];

  const table: ChartTable = {
    columns: [
      { key: "__x", label: t("chart.category") },
      ...series.flatMap((s) => [
        { key: s.key, label: s.label, numeric: true },
        ...(previousOn
          ? [
              {
                key: previousKey(s.key),
                label: t("chart.previous_of", { label: s.label }),
                numeric: true,
              },
            ]
          : []),
      ]),
    ],
    rows: rows.map((r) => ({
      key: r.key,
      cells: [
        label(r, true),
        ...series.flatMap((s) => {
          const f = s.format ?? valueFormat;
          const cur = formatValue(r[s.key] as number, f, fmt, durations);
          return previousOn
            ? [
                cur,
                formatValue(r[previousKey(s.key)] as number, f, fmt, durations),
              ]
            : [cur];
        }),
      ],
    })),
  };

  const directLabels =
    series.length >= LEGEND_MIN_SERIES &&
    series.length <= DIRECT_LABEL_MAX_SERIES;
  const lastIndex = (key: string) => {
    for (let i = rows.length - 1; i >= 0; i--)
      if (isNumber(rows[i][key])) return i;
    return -1;
  };
  const longest = directLabels
    ? Math.max(...series.map((s) => s.label.length))
    : 0;
  const margin = {
    top: 8,
    right: directLabels ? Math.min(140, 12 + longest * 6.5) : 12,
    bottom: 0,
    left: 0,
  };

  const endLabel = (s: ChartSeriesDef) => {
    const at = lastIndex(s.key);
    const LabelMark = (p: LabelProps) =>
      p.index === at && p.x !== undefined && p.y !== undefined ? (
        <text
          x={Number(p.x) + 8}
          y={p.y}
          dy="0.35em"
          fontSize={11}
          fill="var(--foreground)"
        >
          {s.label}
        </text>
      ) : (
        <g />
      );
    return LabelMark;
  };

  const tooltip = (
    <Tooltip
      cursor={crosshairCursor}
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
              previous: previousOn
                ? ((datum[previousKey(s.key)] as number | null) ?? null)
                : undefined,
              format: s.format ?? valueFormat,
              good,
            }))}
          />
        );
      }}
    />
  );

  const axes = (
    <>
      <CartesianGrid {...gridProps} />
      <XAxis
        {...xAxisProps}
        dataKey="key"
        tickFormatter={(v) => {
          const d = rows.find((r) => r.key === String(v));
          return d ? label(d) : String(v);
        }}
      />
      <YAxis
        {...yAxisProps}
        tickFormatter={(v) =>
          formatAxisValue(Number(v), valueFormat, fmt, durations)
        }
      />
    </>
  );

  const previousMarks = previousOn
    ? series.map((s) => (
        <Line
          key={`${s.key}-prev`}
          dataKey={previousKey(s.key)}
          name={t("chart.previous_of", { label: s.label })}
          type="monotone"
          stroke={colors[s.key]}
          strokeOpacity={0.55}
          strokeWidth={2}
          strokeDasharray="5 4"
          dot={false}
          activeDot={false}
          isAnimationActive={false}
          connectNulls
        />
      ))
    : null;

  return (
    <ChartFrame
      {...chrome}
      legend={legend}
      table={table}
      isEmpty={isEmpty}
      testId={chrome.title ? undefined : "trend-chart"}
    >
      <ResponsiveContainer width="100%" height="100%">
        {kind === "area" ? (
          <AreaChart data={rows} margin={margin}>
            {axes}
            {tooltip}
            {series.map((s) => (
              <Area
                key={s.key}
                dataKey={s.key}
                name={s.label}
                type="monotone"
                stroke={colors[s.key]}
                fill={colors[s.key]}
                fillOpacity={stacked ? 0.35 : 0.14}
                strokeWidth={2}
                stackId={stacked ? "stack" : undefined}
                activeDot={{
                  r: 5,
                  strokeWidth: 2,
                  stroke: "var(--viz-surface)",
                }}
                label={directLabels && !stacked ? endLabel(s) : undefined}
                isAnimationActive={false}
              />
            ))}
            {stacked ? null : previousMarks}
          </AreaChart>
        ) : (
          <LineChart data={rows} margin={margin}>
            {axes}
            {tooltip}
            {previousMarks}
            {series.map((s) => (
              <Line
                key={s.key}
                dataKey={s.key}
                name={s.label}
                type="monotone"
                stroke={colors[s.key]}
                strokeWidth={2}
                dot={false}
                activeDot={{
                  r: 5,
                  strokeWidth: 2,
                  stroke: "var(--viz-surface)",
                }}
                label={directLabels ? endLabel(s) : undefined}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        )}
      </ResponsiveContainer>
    </ChartFrame>
  );
}
