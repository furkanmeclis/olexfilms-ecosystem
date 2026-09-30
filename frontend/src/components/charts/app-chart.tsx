"use client";

import { useMemo, type ReactNode } from "react";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Line,
  LineChart,
  Pie,
  PieChart,
  PolarAngleAxis,
  PolarGrid,
  PolarRadiusAxis,
  Radar,
  RadarChart,
  RadialBar,
  RadialBarChart,
  XAxis,
  YAxis,
} from "recharts";

import type { AppChartProps, ChartSeries } from "@/components/charts/types";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  type ChartConfig,
} from "@/components/ui/chart";
import { Skeleton } from "@/components/ui/skeleton";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

function normalizeSeries(
  series: AppChartProps["series"],
  config: AppChartProps["config"],
  categoryKey: string,
  stacked: boolean | undefined,
): ChartSeries[] {
  if (series?.length) {
    return series.map((item) =>
      typeof item === "string"
        ? { key: item, stackId: stacked ? "stack" : undefined }
        : {
            ...item,
            stackId: item.stackId ?? (stacked ? "stack" : undefined),
          },
    );
  }

  return Object.keys(config)
    .filter((key) => key !== categoryKey)
    .map((key) => ({
      key,
      stackId: stacked ? "stack" : undefined,
    }));
}

function SeriesMarks({
  chartType,
  series,
  curved,
}: {
  chartType: AppChartProps["type"];
  series: ChartSeries[];
  curved?: boolean;
}) {
  const curve = curved ? "monotone" : "linear";
  const horizontal = chartType === "bar-horizontal";

  return (
    <>
      {series.map((s) => {
        const mark: ChartSeries["type"] =
          chartType === "composed"
            ? (s.type ?? "bar")
            : chartType === "area"
              ? "area"
              : chartType === "line"
                ? "line"
                : "bar";
        const color = `var(--color-${s.key})`;

        if (mark === "area") {
          return (
            <Area
              key={s.key}
              dataKey={s.key}
              type={curve}
              fill={color}
              stroke={color}
              strokeDasharray={s.strokeDasharray}
              fillOpacity={0.25}
              stackId={s.stackId}
              yAxisId={s.yAxisId ?? "left"}
            />
          );
        }

        if (mark === "line") {
          return (
            <Line
              key={s.key}
              dataKey={s.key}
              type={curve}
              stroke={color}
              strokeWidth={2}
              strokeDasharray={s.strokeDasharray}
              dot={false}
              activeDot={{ r: 4 }}
              yAxisId={s.yAxisId ?? "left"}
            />
          );
        }

        return (
          <Bar
            key={s.key}
            dataKey={s.key}
            fill={color}
            radius={horizontal ? [0, 4, 4, 0] : [4, 4, 0, 0]}
            stackId={s.stackId}
            yAxisId={s.yAxisId ?? "left"}
          />
        );
      })}
    </>
  );
}

function chartPaletteColor(index: number): string {
  return `var(--chart-${(index % 5) + 1})`;
}

/** Stable CSS-safe key for category slice colors (labels may contain spaces). */
function categorySliceKey(index: number): string {
  return `slice_${index}`;
}

function withCategoryColors(
  config: ChartConfig,
  data: Record<string, unknown>[],
  categoryKey: string,
): ChartConfig {
  const next: ChartConfig = { ...config };
  data.forEach((row, index) => {
    const name = String(row[categoryKey] ?? "").trim();
    if (!name) return;
    const key = categorySliceKey(index);
    next[key] = {
      label: name,
      color: chartPaletteColor(index),
    };
  });
  return next;
}

export function AppChart({
  type,
  data,
  config,
  categoryKey = "label",
  series: seriesProp,
  stacked,
  curved = true,
  showGrid = true,
  showLegend = true,
  showTooltip = true,
  showXAxis = true,
  showYAxis = true,
  showRightYAxis,
  valueFormatter,
  tickValueFormatter,
  categoryFormatter,
  height = 280,
  className,
  title,
  description,
  actions,
  footer,
  loading,
  emptyTitle,
  emptyDescription,
  innerRadius,
  outerRadius,
  margin,
}: AppChartProps) {
  const { t } = useLocale();
  const isCategoryChart =
    type === "pie" || type === "donut" || type === "radial";

  const axisValueFormatter = tickValueFormatter ?? valueFormatter;
  const axisWidth =
    type === "bar-horizontal" ? 120 : axisValueFormatter ? 72 : 48;

  const resolvedConfig = useMemo(
    () =>
      isCategoryChart ? withCategoryColors(config, data, categoryKey) : config,
    [isCategoryChart, config, data, categoryKey],
  );

  const series = useMemo(
    () => normalizeSeries(seriesProp, resolvedConfig, categoryKey, stacked),
    [seriesProp, resolvedConfig, categoryKey, stacked],
  );

  const hasRightAxis =
    showRightYAxis ?? series.some((s) => s.yAxisId === "right");

  const isEmpty = !loading && data.length === 0;

  const defaultMargin = margin ?? {
    top: 8,
    right: hasRightAxis ? 12 : 8,
    bottom: 0,
    left: 0,
  };

  const tooltip = showTooltip ? (
    <ChartTooltip
      cursor={
        type === "bar" || type === "bar-horizontal" || type === "composed"
      }
      content={
        <ChartTooltipContent
          nameKey={isCategoryChart ? categoryKey : undefined}
          formatter={
            valueFormatter
              ? (value) =>
                  valueFormatter(
                    typeof value === "number" ? value : Number(value),
                  )
              : undefined
          }
        />
      }
    />
  ) : null;

  const legend = showLegend ? (
    <ChartLegend
      content={
        <ChartLegendContent
          nameKey={isCategoryChart ? categoryKey : undefined}
        />
      }
    />
  ) : null;

  const grid = showGrid ? (
    <CartesianGrid vertical={false} strokeDasharray="3 3" />
  ) : null;

  const xAxis = showXAxis ? (
    <XAxis
      dataKey={type === "bar-horizontal" ? undefined : categoryKey}
      type={type === "bar-horizontal" ? "number" : "category"}
      tickLine={false}
      axisLine={false}
      tickMargin={8}
      minTickGap={24}
      tickFormatter={
        type === "bar-horizontal"
          ? (v) =>
              axisValueFormatter ? axisValueFormatter(Number(v)) : String(v)
          : categoryFormatter
            ? (v) => categoryFormatter(String(v))
            : undefined
      }
    />
  ) : null;

  const yAxis = showYAxis ? (
    <YAxis
      yAxisId="left"
      dataKey={type === "bar-horizontal" ? categoryKey : undefined}
      type={type === "bar-horizontal" ? "category" : "number"}
      tickLine={false}
      axisLine={false}
      width={axisWidth}
      tickFormatter={
        type === "bar-horizontal"
          ? categoryFormatter
            ? (v) => categoryFormatter(String(v))
            : undefined
          : axisValueFormatter
            ? (v) => axisValueFormatter(Number(v))
            : undefined
      }
    />
  ) : null;

  const rightAxis = hasRightAxis ? (
    <YAxis
      yAxisId="right"
      orientation="right"
      tickLine={false}
      axisLine={false}
      width={axisWidth}
      tickFormatter={
        axisValueFormatter ? (v) => axisValueFormatter(Number(v)) : undefined
      }
    />
  ) : null;

  let plot: ReactNode = null;

  if (!loading && !isEmpty) {
    if (type === "pie" || type === "donut") {
      const valueKey = series[0]?.key ?? "value";
      plot = (
        <PieChart margin={defaultMargin}>
          {tooltip}
          {legend}
          <Pie
            data={data}
            dataKey={valueKey}
            nameKey={categoryKey}
            innerRadius={
              type === "donut" ? (innerRadius ?? 55) : (innerRadius ?? 0)
            }
            outerRadius={outerRadius ?? 90}
            strokeWidth={2}
          >
            {data.map((row, index) => {
              const sliceKey = categorySliceKey(index);
              return (
                <Cell key={`cell-${index}`} fill={`var(--color-${sliceKey})`} />
              );
            })}
          </Pie>
        </PieChart>
      );
    } else if (type === "radar") {
      plot = (
        <RadarChart data={data} margin={defaultMargin}>
          <PolarGrid />
          <PolarAngleAxis dataKey={categoryKey} />
          <PolarRadiusAxis />
          {tooltip}
          {legend}
          {series.map((s) => (
            <Radar
              key={s.key}
              dataKey={s.key}
              stroke={`var(--color-${s.key})`}
              fill={`var(--color-${s.key})`}
              fillOpacity={0.25}
            />
          ))}
        </RadarChart>
      );
    } else if (type === "radial") {
      const valueKey = series[0]?.key ?? "value";
      const formatCategory = (raw: string) => {
        if (categoryFormatter) return categoryFormatter(raw);
        const label = config[raw]?.label;
        return typeof label === "string" ? label : raw;
      };
      plot = (
        <RadialBarChart
          data={data}
          innerRadius={innerRadius ?? 30}
          outerRadius={outerRadius ?? 110}
          margin={defaultMargin}
        >
          <PolarGrid gridType="circle" />
          <PolarAngleAxis
            dataKey={categoryKey}
            type="category"
            tickFormatter={formatCategory}
          />
          {tooltip}
          {legend}
          <RadialBar dataKey={valueKey} background>
            {data.map((row, index) => {
              const sliceKey = categorySliceKey(index);
              return (
                <Cell
                  key={`radial-${index}`}
                  fill={`var(--color-${sliceKey})`}
                />
              );
            })}
          </RadialBar>
        </RadialBarChart>
      );
    } else if (type === "line") {
      plot = (
        <LineChart data={data} margin={defaultMargin}>
          {grid}
          {xAxis}
          {yAxis}
          {rightAxis}
          {tooltip}
          {legend}
          <SeriesMarks chartType="line" series={series} curved={curved} />
        </LineChart>
      );
    } else if (type === "area") {
      plot = (
        <AreaChart data={data} margin={defaultMargin}>
          {grid}
          {xAxis}
          {yAxis}
          {rightAxis}
          {tooltip}
          {legend}
          <SeriesMarks chartType="area" series={series} curved={curved} />
        </AreaChart>
      );
    } else if (type === "bar" || type === "bar-horizontal") {
      plot = (
        <BarChart
          data={data}
          layout={type === "bar-horizontal" ? "vertical" : "horizontal"}
          margin={defaultMargin}
        >
          {grid}
          {xAxis}
          {yAxis}
          {rightAxis}
          {tooltip}
          {legend}
          <SeriesMarks chartType={type} series={series} curved={curved} />
        </BarChart>
      );
    } else {
      plot = (
        <ComposedChart data={data} margin={defaultMargin}>
          {grid}
          {xAxis}
          {yAxis}
          {rightAxis}
          {tooltip}
          {legend}
          <SeriesMarks chartType="composed" series={series} curved={curved} />
        </ComposedChart>
      );
    }
  }

  const chartBody =
    loading || isEmpty ? (
      <div
        className="flex w-full flex-col items-center justify-center gap-1 px-4 text-center"
        style={{ height }}
      >
        {loading ? (
          <Skeleton className="h-full w-full rounded-lg" />
        ) : (
          <>
            <p className="text-sm font-medium">
              {emptyTitle ?? t("chart.empty_title")}
            </p>
            <p className="text-muted-foreground text-xs">
              {emptyDescription ?? t("chart.empty_description")}
            </p>
          </>
        )}
      </div>
    ) : (
      <ChartContainer
        config={resolvedConfig}
        className="aspect-auto w-full"
        style={{ height }}
      >
        {plot}
      </ChartContainer>
    );

  if (!title && !description && !actions && !footer) {
    return <div className={className}>{chartBody}</div>;
  }

  return (
    <Card className={cn("shadow-none", className)}>
      {(title || description || actions) && (
        <CardHeader className="border-b [.border-b]:pb-4">
          {title ? <CardTitle>{title}</CardTitle> : null}
          {description ? (
            <CardDescription>{description}</CardDescription>
          ) : null}
          {actions ? <CardAction>{actions}</CardAction> : null}
        </CardHeader>
      )}
      <CardContent className={cn(!(title || description) && "pt-6")}>
        {chartBody}
      </CardContent>
      {footer ? <CardFooter>{footer}</CardFooter> : null}
    </Card>
  );
}
