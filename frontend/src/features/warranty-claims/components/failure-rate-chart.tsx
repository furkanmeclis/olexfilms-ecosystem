"use client";

import { useMemo } from "react";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";

import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import {
  toFailureRateBars,
  type FailureRateBar,
} from "@/features/warranty-claims/lib/claim-reports";
import type { FailureRateRow } from "@/features/warranty-claims/services/claim-reports.service";
import { useLocale } from "@/providers/locale-provider";

const AXIS_WIDTH = 168;
const ROW_HEIGHT = 40;
const LABEL_MAX = 24;

function truncate(label: string) {
  return label.length > LABEL_MAX ? `${label.slice(0, LABEL_MAX - 1)}…` : label;
}

/**
 * Horizontal bars of the highest claim / approved failure rates (in percent
 * points). Theme tokens keep light and dark readable; in RTL the value axis
 * is reversed and the labels move to the right edge.
 */
export function FailureRateChart({
  rows,
}: {
  rows: readonly FailureRateRow[];
}) {
  const { t, dir, format } = useLocale();
  const rtl = dir === "rtl";
  const bars = useMemo<FailureRateBar[]>(() => toFailureRateBars(rows), [rows]);
  const config = useMemo<ChartConfig>(
    () => ({
      claimPercent: {
        label: t("warranty.claim_reports.columns.claim_rate"),
        color: "var(--chart-2)",
      },
      approvedPercent: {
        label: t("warranty.claim_reports.columns.approved_rate"),
        color: "var(--chart-1)",
      },
    }),
    [t],
  );
  if (bars.length === 0) return null;
  const height = bars.length * ROW_HEIGHT + 48;

  return (
    <ChartContainer
      config={config}
      className="aspect-auto w-full"
      style={{ height }}
      initialDimension={{ width: 480, height }}
      data-testid="claim-report-chart"
    >
      <BarChart
        data={bars}
        layout="vertical"
        margin={{ top: 4, bottom: 4, left: 0, right: 16 }}
      >
        <CartesianGrid horizontal={false} />
        <XAxis
          type="number"
          reversed={rtl}
          tickFormatter={(v: number) => format.percent(v / 100)}
        />
        <YAxis
          type="category"
          dataKey="label"
          orientation={rtl ? "right" : "left"}
          width={AXIS_WIDTH}
          tickLine={false}
          axisLine={false}
          tickFormatter={truncate}
        />
        <ChartTooltip
          content={
            <ChartTooltipContent
              formatter={(value, name) => (
                <span className="flex w-full justify-between gap-4">
                  <span className="text-muted-foreground">
                    {config[String(name)]?.label}
                  </span>
                  <span className="font-mono font-medium tabular-nums">
                    {format.percent(Number(value) / 100, 1)}
                  </span>
                </span>
              )}
            />
          }
        />
        <ChartLegend content={<ChartLegendContent />} />
        <Bar
          dataKey="claimPercent"
          fill="var(--color-claimPercent)"
          radius={4}
        />
        <Bar
          dataKey="approvedPercent"
          fill="var(--color-approvedPercent)"
          radius={4}
        />
      </BarChart>
    </ChartContainer>
  );
}
