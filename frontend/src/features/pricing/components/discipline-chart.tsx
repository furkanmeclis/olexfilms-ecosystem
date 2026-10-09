"use client";

import { useMemo } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  ReferenceLine,
  XAxis,
  YAxis,
} from "recharts";

import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { toNumber } from "@/features/pricing/lib/recommended";
import type { DisciplineCountry } from "@/features/pricing/services/recommended.service";
import { useLocale } from "@/providers/locale-provider";

const AXIS_WIDTH = 120;
const ROW_HEIGHT = 36;

/**
 * Average deviation from the recommended price per country × currency
 * (horizontal bars, one series) with the threshold on both sides. In RTL the
 * value axis is reversed and the labels move to the right edge.
 */
export function DisciplineChart({
  countries,
  threshold,
}: {
  countries: readonly DisciplineCountry[];
  threshold: number;
}) {
  const { t, dir, format, locale } = useLocale();
  const rtl = dir === "rtl";
  const bars = useMemo(
    () =>
      countries
        .map((c) => ({
          label: `${c.country_iso2 || t("catalog.recommended.scope_currency")} · ${c.currency}`,
          name: locale === "tr" ? c.country_name_tr : c.country_name_en,
          avg: toNumber(c.avg_deviation_pct),
        }))
        .filter(
          (b): b is { label: string; name: string; avg: number } =>
            b.avg !== null,
        ),
    [countries, locale, t],
  );
  const config = useMemo<ChartConfig>(
    () => ({
      avg: {
        label: t("catalog.discipline.avg_deviation"),
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
      data-testid="discipline-chart"
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
        />
        <ReferenceLine
          x={threshold}
          stroke="var(--muted-foreground)"
          strokeDasharray="4 4"
        />
        <ReferenceLine
          x={-threshold}
          stroke="var(--muted-foreground)"
          strokeDasharray="4 4"
        />
        <ChartTooltip
          content={
            <ChartTooltipContent
              labelFormatter={(_, payload) =>
                String(payload?.[0]?.payload?.name ?? "")
              }
              formatter={(value) => (
                <span className="flex w-full justify-between gap-4">
                  <span className="text-muted-foreground">
                    {t("catalog.discipline.avg_deviation")}
                  </span>
                  <span className="font-mono tabular-nums">
                    {format.percent(Number(value) / 100, 2)}
                  </span>
                </span>
              )}
            />
          }
        />
        <Bar dataKey="avg" fill="var(--color-avg)" radius={4} />
      </BarChart>
    </ChartContainer>
  );
}
