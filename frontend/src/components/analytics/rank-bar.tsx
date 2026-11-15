"use client";

import { useMemo } from "react";
import Link from "next/link";

import { useFormatter, useLocale } from "@/providers/locale-provider";

import { ChartFrame, type ChartTable } from "./chart-frame";
import { formatValue, type ValueFormat } from "./format";
import { categoricalColor, foldCategories } from "./palette";
import type { CategoryDatum, ChartChromeProps } from "./types";
import { useDurationLabels } from "./use-duration-labels";

export type RankBarItem = CategoryDatum & { href?: string };

export type RankBarProps = Omit<ChartChromeProps, "height"> & {
  items: RankBarItem[];
  format?: ValueFormat;
  /** Rows shown; the rest fold into "Other" (default 10). */
  limit?: number;
  /** Sort descending (default true). */
  sort?: boolean;
  /** Bar colour (default categorical slot 1; one series = one colour). */
  color?: string;
};

/**
 * Horizontal ranking bars (HTML, not SVG: long labels wrap, rows are
 * links when `href` is given). Values are labelled at the bar end.
 */
export function RankBar({
  items,
  format,
  limit = 10,
  sort = true,
  color = categoricalColor(0),
  ...chrome
}: RankBarProps) {
  const fmt = useFormatter();
  const { t } = useLocale();
  const durations = useDurationLabels();
  const rows = useMemo(() => {
    const sorted = sort ? [...items].sort((a, b) => b.value - a.value) : items;
    return foldCategories(sorted, t("chart.other"), limit) as RankBarItem[];
  }, [items, sort, limit, t]);
  const max = Math.max(0, ...rows.map((r) => r.value));
  const table: ChartTable = {
    columns: [
      { key: "label", label: t("chart.category") },
      { key: "value", label: t("chart.value"), numeric: true },
    ],
    rows: rows.map((r) => ({
      key: r.key,
      cells: [r.label, formatValue(r.value, format, fmt, durations)],
    })),
  };

  return (
    <ChartFrame
      {...chrome}
      table={table}
      isEmpty={rows.length === 0}
      height={Math.max(120, rows.length * 32)}
    >
      <ol className="flex flex-col gap-1" data-slot="rank-bar">
        {rows.map((r) => {
          const pct = max > 0 ? Math.max(0, r.value / max) * 100 : 0;
          const valueText = formatValue(r.value, format, fmt, durations);
          const inner = (
            <>
              <span className="text-foreground w-32 shrink-0 truncate text-xs sm:w-40">
                {r.label}
              </span>
              <span className="relative h-5 min-w-0 flex-1">
                <span
                  className="absolute inset-y-0 start-0 rounded-e-[4px]"
                  style={{
                    width: `${pct}%`,
                    background:
                      r.key === "__other" ? "var(--viz-other)" : color,
                  }}
                />
              </span>
              <span className="text-foreground w-20 shrink-0 text-end text-xs font-medium tabular-nums">
                {valueText}
              </span>
            </>
          );
          return (
            <li key={r.key} title={`${r.label}: ${valueText}`}>
              {r.href ? (
                <Link
                  href={r.href}
                  className="hover:bg-muted/60 flex items-center gap-2 rounded-md px-1 py-1"
                >
                  {inner}
                </Link>
              ) : (
                <div className="hover:bg-muted/60 flex items-center gap-2 rounded-md px-1 py-1">
                  {inner}
                </div>
              )}
            </li>
          );
        })}
      </ol>
    </ChartFrame>
  );
}
