"use client";

import { BarChart3 } from "lucide-react";
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
  CardAction,
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
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { brandLogoUrl } from "@/features/vehicle-catalog/lib/images";
import {
  toTopVehicleBars,
  truncateLabel,
  type TopVehicleBar,
} from "@/features/vehicle-catalog/lib/top-vehicles";
import {
  STATS_GROUPS,
  STATS_PERIODS,
  type StatsGroup,
  type StatsPeriod,
  type TopVehicle,
} from "@/features/vehicle-catalog/services/vehicle-stats.service";
import { useLocale } from "@/providers/locale-provider";

const AXIS_WIDTH = 168;
const LOGO_SIZE = 20;
const ROW_HEIGHT = 36;

const GROUP_LABELS: Record<StatsGroup, string> = {
  model: "dashboard.top_vehicles.group.model",
  brand: "dashboard.top_vehicles.group.brand",
};
const PERIOD_LABELS: Record<StatsPeriod, string> = {
  "30d": "dashboard.top_vehicles.period.30d",
  "90d": "dashboard.top_vehicles.period.90d",
  "12m": "dashboard.top_vehicles.period.12m",
  all: "dashboard.top_vehicles.period.all",
};

export type TopVehicleModelsChartProps = {
  items: readonly TopVehicle[] | undefined;
  period: StatsPeriod;
  group: StatsGroup;
  onPeriodChange: (period: StatsPeriod) => void;
  onGroupChange: (group: StatsGroup) => void;
  isLoading?: boolean;
  isError?: boolean;
  onRetry?: () => void;
};

type TickProps = {
  x?: number | string;
  y?: number | string;
  payload?: { value?: string | number };
};

/**
 * TEC-151 center dashboard widget: horizontal bar chart of the top-10 car
 * brands / models by completed services, with brand logos on the category
 * axis. Colors come from the theme tokens (--chart-1, --muted-foreground,
 * --border), so light and dark both work. In RTL the value axis is
 * reversed and the category axis moves to the right edge, so bars grow
 * from right to left.
 */
export function TopVehicleModelsChart({
  items,
  period,
  group,
  onPeriodChange,
  onGroupChange,
  isLoading,
  isError,
  onRetry,
}: TopVehicleModelsChartProps) {
  const { t, dir } = useLocale();
  const rtl = dir === "rtl";
  const bars = useMemo(
    () => toTopVehicleBars(items ?? [], group),
    [items, group],
  );
  const byKey = useMemo(
    () => new Map(bars.map((bar) => [bar.key, bar] as const)),
    [bars],
  );
  const config = useMemo<ChartConfig>(
    () => ({
      count: {
        label: t("dashboard.top_vehicles.count"),
        color: "var(--chart-1)",
      },
    }),
    [t],
  );

  const renderTick = ({ x = 0, y = 0, payload }: TickProps) => {
    const bar = byKey.get(String(payload?.value ?? ""));
    if (!bar) return <g />;
    const nx = Number(x);
    const ny = Number(y);
    // The tick sits on the axis line; labels run away from the bars.
    const logoX = rtl ? AXIS_WIDTH - LOGO_SIZE - 4 : -AXIS_WIDTH + 4;
    return (
      <g transform={`translate(${nx},${ny})`} data-slot="top-vehicle-tick">
        <image
          href={brandLogoUrl(bar.brandUuid, bar.logoVersion)}
          x={logoX}
          y={-LOGO_SIZE / 2}
          width={LOGO_SIZE}
          height={LOGO_SIZE}
          preserveAspectRatio="xMidYMid meet"
          aria-hidden
        />
        <text
          x={rtl ? 8 : -8}
          y={0}
          dy="0.35em"
          textAnchor="end"
          direction={rtl ? "rtl" : "ltr"}
          className="fill-muted-foreground text-xs"
        >
          <title>{bar.label}</title>
          {truncateLabel(bar.label)}
        </text>
      </g>
    );
  };

  return (
    <Card data-slot="top-vehicle-models">
      <CardHeader>
        <CardTitle>{t("dashboard.top_vehicles.title")}</CardTitle>
        <CardDescription>
          {t("dashboard.top_vehicles.description")}
        </CardDescription>
        <CardAction className="flex flex-wrap items-center gap-2">
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={group}
            aria-label={t("dashboard.top_vehicles.group_label")}
            onValueChange={(v) => v && onGroupChange(v as StatsGroup)}
          >
            {STATS_GROUPS.map((g) => (
              <ToggleGroupItem key={g} value={g} data-group={g}>
                {t(GROUP_LABELS[g])}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={period}
            aria-label={t("dashboard.top_vehicles.period_label")}
            onValueChange={(v) => v && onPeriodChange(v as StatsPeriod)}
          >
            {STATS_PERIODS.map((p) => (
              <ToggleGroupItem key={p} value={p} data-period={p}>
                {t(PERIOD_LABELS[p])}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </CardAction>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div
            data-slot="top-vehicle-models-loading"
            aria-busy="true"
            className="flex flex-col gap-3"
          >
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
            data-slot="top-vehicle-models-error"
            className="text-muted-foreground flex flex-col items-center gap-2 py-10 text-center text-sm"
          >
            <p>{t("dashboard.top_vehicles.error")}</p>
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
            data-slot="top-vehicle-models-empty"
            className="text-muted-foreground flex flex-col items-center gap-2 py-10 text-center text-sm"
          >
            <BarChart3 className="size-8 opacity-50" aria-hidden />
            <p>{t("dashboard.top_vehicles.empty")}</p>
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
                        (payload?.[0]?.payload as TopVehicleBar | undefined)
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
            <table className="sr-only" data-slot="top-vehicle-models-table">
              <caption>{t("dashboard.top_vehicles.title")}</caption>
              <thead>
                <tr>
                  <th scope="col">{t("dashboard.top_vehicles.rank")}</th>
                  <th scope="col">{t("dashboard.top_vehicles.name")}</th>
                  <th scope="col">{t("dashboard.top_vehicles.count")}</th>
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
