"use client";

import { useMemo } from "react";

import { useFormatter, useLocale } from "@/providers/locale-provider";

import { ChartFrame, type ChartTable } from "./chart-frame";
import { formatValue, isNumber, type ValueFormat } from "./format";
import { SEQUENTIAL_STEPS, sequentialColor } from "./palette";
import type { ChartChromeProps } from "./types";
import { useDurationLabels } from "./use-duration-labels";

/** One cell: ISO weekday (1 = Monday … 7 = Sunday) × hour (0..23). */
export type HeatmapCell = { day: number; hour: number; value: number };

export type HeatmapProps = Omit<ChartChromeProps, "height"> & {
  cells: HeatmapCell[];
  format?: ValueFormat;
  /** Hours shown (default 0..23). */
  hours?: number[];
  /** First weekday (1 Monday default, 7 Sunday). */
  weekStart?: 1 | 7;
};

/** day × hour matrix → value lookup + max (exported for tests). */
export function heatmapMatrix(cells: readonly HeatmapCell[]) {
  const map = new Map<string, number>();
  let max = 0;
  for (const c of cells) {
    if (!isNumber(c.value)) continue;
    const k = `${c.day}:${c.hour}`;
    const v = (map.get(k) ?? 0) + c.value;
    map.set(k, v);
    if (v > max) max = v;
  }
  return {
    get: (day: number, hour: number) => map.get(`${day}:${hour}`) ?? 0,
    max,
  };
}

const ALL_HOURS = Array.from({ length: 24 }, (_, i) => i);

/**
 * Day × hour heatmap on the sequential blue scale. Every cell has a hover
 * tooltip (title) and the table view lists the numbers.
 */
export function Heatmap({
  cells,
  format,
  hours = ALL_HOURS,
  weekStart = 1,
  ...chrome
}: HeatmapProps) {
  const fmt = useFormatter();
  const { t } = useLocale();
  const durations = useDurationLabels();
  const matrix = useMemo(() => heatmapMatrix(cells), [cells]);
  const days = weekStart === 7 ? [7, 1, 2, 3, 4, 5, 6] : [1, 2, 3, 4, 5, 6, 7];
  // 2024-01-01 is a Monday.
  const dayName = (d: number, style: "short" | "long" = "short") =>
    fmt.dateParts(new Date(2024, 0, d), { weekday: style });
  const hourName = (h: number) =>
    fmt.dateParts(new Date(2024, 0, 1, h), { hour: "2-digit" });

  const table: ChartTable = {
    columns: [
      { key: "day", label: t("chart.heatmap.day") },
      ...hours.map((h) => ({
        key: String(h),
        label: hourName(h),
        numeric: true,
      })),
    ],
    rows: days.map((d) => ({
      key: String(d),
      cells: [
        dayName(d, "long"),
        ...hours.map((h) =>
          formatValue(matrix.get(d, h), format, fmt, durations),
        ),
      ],
    })),
  };

  return (
    <ChartFrame
      {...chrome}
      table={table}
      isEmpty={matrix.max === 0}
      height={7 * 22 + 40}
    >
      <div className="overflow-x-auto" data-slot="heatmap">
        <div
          className="grid min-w-[560px] gap-[2px] text-[10px]"
          style={{
            gridTemplateColumns: `minmax(2.5rem,auto) repeat(${hours.length}, minmax(0,1fr))`,
          }}
        >
          <span />
          {hours.map((h) => (
            <span
              key={h}
              className="text-center"
              style={{ color: "var(--viz-muted)" }}
            >
              {h % 3 === 0 ? hourName(h) : ""}
            </span>
          ))}
          {days.map((d) => (
            <HeatmapRow
              key={d}
              label={dayName(d)}
              longLabel={dayName(d, "long")}
              values={hours.map((h) => matrix.get(d, h))}
              hours={hours}
              hourName={hourName}
              max={matrix.max}
              formatCell={(v) => formatValue(v, format, fmt, durations)}
            />
          ))}
        </div>
        <div className="text-muted-foreground mt-2 flex items-center justify-end gap-1 text-[10px]">
          <span>{t("chart.heatmap.less")}</span>
          {Array.from({ length: SEQUENTIAL_STEPS }, (_, i) => (
            <span
              key={i}
              aria-hidden
              className="inline-block h-2.5 w-3 rounded-[2px]"
              style={{
                background: sequentialColor(i / (SEQUENTIAL_STEPS - 1)),
              }}
            />
          ))}
          <span>{t("chart.heatmap.more")}</span>
        </div>
      </div>
    </ChartFrame>
  );
}

function HeatmapRow({
  label,
  longLabel,
  values,
  hours,
  hourName,
  max,
  formatCell,
}: {
  label: string;
  longLabel: string;
  values: number[];
  hours: number[];
  hourName: (h: number) => string;
  max: number;
  formatCell: (v: number) => string;
}) {
  return (
    <>
      <span className="text-foreground flex items-center pe-1">{label}</span>
      {values.map((v, i) => {
        const text = `${longLabel} ${hourName(hours[i])}: ${formatCell(v)}`;
        return (
          <span
            key={hours[i]}
            title={text}
            aria-hidden
            className="h-5 rounded-[3px] hover:outline-2 hover:outline-[var(--foreground)]"
            style={{
              background: v > 0 ? sequentialColor(v / max) : "var(--viz-grid)",
            }}
          />
        );
      })}
    </>
  );
}
