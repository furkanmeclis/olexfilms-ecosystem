"use client";

import { useMemo } from "react";
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";

import { useFormatter, useLocale } from "@/providers/locale-provider";

import {
  ChartFrame,
  type ChartTable,
  type LegendItem,
  CHART_INITIAL_DIMENSION,
} from "./chart-frame";
import { formatValue, type ValueFormat } from "./format";
import { OTHER_KEY, categoricalColor, foldCategories } from "./palette";
import type { CategoryDatum, ChartChromeProps } from "./types";
import { useDurationLabels } from "./use-duration-labels";
import { VizTooltipCard } from "./viz-tooltip";

export type DonutChartProps = ChartChromeProps & {
  items: CategoryDatum[];
  format?: ValueFormat;
  /** Centre label under the total (default chart.total). */
  centerLabel?: string;
};

/**
 * Part-to-whole donut: at most 8 slices + "Other", 2px surface gaps, total
 * in the middle, legend with shares (always, slices ≥ 2).
 */
export function DonutChart({
  items,
  format,
  centerLabel,
  height = 240,
  ...chrome
}: DonutChartProps) {
  const fmt = useFormatter();
  const { t } = useLocale();
  const durations = useDurationLabels();
  const slices = useMemo(
    () =>
      foldCategories(
        items.filter((i) => i.value > 0),
        t("chart.other"),
      ) as CategoryDatum[],
    [items, t],
  );
  const total = slices.reduce((acc, s) => acc + s.value, 0);
  const color = (s: CategoryDatum, i: number) =>
    s.key === OTHER_KEY
      ? "var(--viz-other)"
      : categoricalColor(s.colorSlot ?? i);
  const share = (v: number) =>
    total > 0
      ? fmt.number(v / total, { style: "percent", maximumFractionDigits: 1 })
      : "";

  const legend: LegendItem[] =
    slices.length >= 2
      ? slices.map((s, i) => ({
          key: s.key,
          label: `${s.label} · ${share(s.value)}`,
          color: color(s, i),
        }))
      : [];
  const table: ChartTable = {
    columns: [
      { key: "label", label: t("chart.category") },
      { key: "value", label: t("chart.value"), numeric: true },
      { key: "share", label: t("chart.share"), numeric: true },
    ],
    rows: slices.map((s) => ({
      key: s.key,
      cells: [
        s.label,
        formatValue(s.value, format, fmt, durations),
        share(s.value),
      ],
    })),
  };

  return (
    <ChartFrame
      {...chrome}
      height={height}
      legend={legend}
      table={table}
      isEmpty={slices.length === 0}
    >
      <div className="relative h-full w-full">
        <ResponsiveContainer
          width="100%"
          height="100%"
          minWidth={0}
          initialDimension={CHART_INITIAL_DIMENSION}
        >
          <PieChart>
            <Tooltip
              content={(props) => {
                const p = props.active ? props.payload?.[0] : undefined;
                const datum = p?.payload as CategoryDatum | undefined;
                if (!datum) return null;
                const i = slices.findIndex((s) => s.key === datum.key);
                return (
                  <VizTooltipCard
                    title={datum.label}
                    rows={[
                      {
                        key: datum.key,
                        label: share(datum.value),
                        color: color(datum, i),
                        value: datum.value,
                        format,
                      },
                    ]}
                  />
                );
              }}
            />
            <Pie
              data={slices}
              dataKey="value"
              nameKey="label"
              innerRadius="62%"
              outerRadius="92%"
              stroke="var(--viz-surface)"
              strokeWidth={2}
              isAnimationActive={false}
            >
              {slices.map((s, i) => (
                <Cell key={s.key} fill={color(s, i)} />
              ))}
            </Pie>
          </PieChart>
        </ResponsiveContainer>
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <span className="text-foreground text-lg font-semibold tabular-nums">
            {formatValue(total, format, fmt, durations)}
          </span>
          <span className="text-muted-foreground text-xs">
            {centerLabel ?? t("chart.total")}
          </span>
        </div>
      </div>
    </ChartFrame>
  );
}
