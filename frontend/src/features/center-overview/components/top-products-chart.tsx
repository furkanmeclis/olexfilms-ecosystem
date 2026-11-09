"use client";

import { Package } from "lucide-react";
import { useMemo } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  LabelList,
  XAxis,
  YAxis,
} from "recharts";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { Skeleton } from "@/components/ui/skeleton";
import type { ReportPoint } from "@/features/center-overview/services/center-overview.service";
import { truncateLabel } from "@/features/vehicle-catalog/lib/top-vehicles";
import { useLocale } from "@/providers/locale-provider";

const AXIS_WIDTH = 168;
const ROW_HEIGHT = 36;

type ProductBar = { key: string; label: string; count: number };

type TickProps = {
  x?: number | string;
  y?: number | string;
  payload?: { value?: string | number };
};

/**
 * TEC-1072: "most used products" of the center home, in the same visual
 * language as the top vehicle models chart (TEC-151): a horizontal bar per
 * product by the number of completed services it was used in, theme
 * colors, RTL mirrored, and a screen-reader table.
 */
export function TopProductsChart({
  points,
  isLoading,
  isError,
  onRetry,
}: {
  points: readonly ReportPoint[] | undefined;
  isLoading?: boolean;
  isError?: boolean;
  onRetry?: () => void;
}) {
  const { t, dir } = useLocale();
  const rtl = dir === "rtl";
  const bars = useMemo<ProductBar[]>(
    () =>
      (points ?? []).map((p) => ({
        key: p.key,
        label: p.label,
        count: p.value,
      })),
    [points],
  );
  const byKey = useMemo(
    () => new Map(bars.map((bar) => [bar.key, bar] as const)),
    [bars],
  );
  const config = useMemo<ChartConfig>(
    () => ({
      count: {
        label: t("dashboard.center.top_products.count"),
        color: "var(--chart-1)",
      },
    }),
    [t],
  );

  const renderTick = ({ x = 0, y = 0, payload }: TickProps) => {
    const bar = byKey.get(String(payload?.value ?? ""));
    if (!bar) return <g />;
    return (
      <g transform={`translate(${Number(x)},${Number(y)})`}>
        <text
          x={rtl ? 8 : -8}
          y={0}
          dy="0.35em"
          textAnchor="end"
          direction={rtl ? "rtl" : "ltr"}
          className="fill-muted-foreground text-xs"
        >
          <title>{bar.label}</title>
          {truncateLabel(bar.label, 24)}
        </text>
      </g>
    );
  };

  return (
    <Card data-slot="top-products">
      <CardHeader>
        <CardTitle>{t("dashboard.center.top_products.title")}</CardTitle>
        <CardDescription>
          {t("dashboard.center.top_products.description")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div aria-busy="true" className="flex flex-col gap-3">
            {Array.from({ length: 5 }, (_, i) => (
              <Skeleton
                key={i}
                className="h-6"
                style={{ width: `${90 - i * 12}%` }}
              />
            ))}
          </div>
        ) : isError ? (
          <div
            role="alert"
            className="text-muted-foreground flex flex-col items-center gap-2 py-10 text-center text-sm"
          >
            <p>{t("dashboard.center.error")}</p>
            {onRetry ? (
              <button
                type="button"
                className="text-primary underline-offset-4 hover:underline"
                onClick={onRetry}
              >
                {t("common.retry")}
              </button>
            ) : null}
          </div>
        ) : bars.length === 0 ? (
          <div
            data-slot="top-products-empty"
            className="text-muted-foreground flex flex-col items-center gap-2 py-10 text-center text-sm"
          >
            <Package className="size-8 opacity-50" aria-hidden />
            <p>{t("dashboard.center.top_products.empty")}</p>
          </div>
        ) : (
          <>
            <ChartContainer
              config={config}
              className="aspect-auto w-full"
              style={{ height: bars.length * ROW_HEIGHT + 24 }}
              initialDimension={{
                width: 480,
                height: bars.length * ROW_HEIGHT + 24,
              }}
              aria-hidden
            >
              <BarChart
                data={bars}
                layout="vertical"
                margin={{
                  top: 4,
                  bottom: 4,
                  left: rtl ? 32 : 0,
                  right: rtl ? 0 : 32,
                }}
              >
                <CartesianGrid horizontal={false} />
                <XAxis
                  type="number"
                  dataKey="count"
                  hide
                  reversed={rtl}
                  allowDecimals={false}
                />
                <YAxis
                  type="category"
                  dataKey="key"
                  orientation={rtl ? "right" : "left"}
                  width={AXIS_WIDTH}
                  tickLine={false}
                  axisLine={false}
                  interval={0}
                  tick={renderTick}
                />
                <ChartTooltip
                  cursor={false}
                  content={
                    <ChartTooltipContent
                      labelFormatter={(_, payload) =>
                        (payload?.[0]?.payload as ProductBar | undefined)
                          ?.label ?? ""
                      }
                    />
                  }
                />
                <Bar
                  dataKey="count"
                  fill="var(--color-count)"
                  radius={rtl ? [4, 0, 0, 4] : [0, 4, 4, 0]}
                  isAnimationActive={false}
                >
                  <LabelList
                    dataKey="count"
                    position={rtl ? "left" : "right"}
                    className="fill-foreground"
                    fontSize={12}
                  />
                </Bar>
              </BarChart>
            </ChartContainer>
            <table className="sr-only" data-slot="top-products-table">
              <caption>{t("dashboard.center.top_products.title")}</caption>
              <thead>
                <tr>
                  <th scope="col">{t("dashboard.top_vehicles.rank")}</th>
                  <th scope="col">{t("dashboard.center.top_products.name")}</th>
                  <th scope="col">
                    {t("dashboard.center.top_products.count")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {bars.map((bar, i) => (
                  <tr key={bar.key}>
                    <td>{i + 1}</td>
                    <td>{bar.label}</td>
                    <td>{bar.count}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </CardContent>
    </Card>
  );
}
