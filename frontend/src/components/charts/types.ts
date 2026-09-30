import type { ReactNode } from "react";

import type { ChartConfig } from "@/components/ui/chart";

export type ChartType =
  | "line"
  | "area"
  | "bar"
  | "bar-horizontal"
  | "pie"
  | "donut"
  | "radar"
  | "radial"
  | "composed";

export type ChartSeriesType = "line" | "area" | "bar";

export type ChartSeries = {
  /** Data key in each row */
  key: string;
  /** Override mark type for composed charts */
  type?: ChartSeriesType;
  /** Stack group id (stacked bar/area) */
  stackId?: string;
  /** Dual axis support */
  yAxisId?: "left" | "right";
  /** Dashed stroke (e.g. previous-period compare series) */
  strokeDasharray?: string;
};

export type AppChartProps = {
  type: ChartType;
  data: Record<string, unknown>[];
  config: ChartConfig;

  /**
   * Category / X-axis / pie name key.
   * Default: `"label"`
   */
  categoryKey?: string;

  /**
   * Series data keys or rich series defs.
   * If omitted, inferred from `config` keys (excluding categoryKey).
   */
  series?: Array<string | ChartSeries>;

  stacked?: boolean;
  /** Smooth curves for line/area */
  curved?: boolean;

  showGrid?: boolean;
  showLegend?: boolean;
  showTooltip?: boolean;
  showXAxis?: boolean;
  showYAxis?: boolean;
  /** Second Y axis when any series uses yAxisId: "right" */
  showRightYAxis?: boolean;

  valueFormatter?: (value: number) => string;
  /** Axis tick formatter; falls back to valueFormatter when omitted */
  tickValueFormatter?: (value: number) => string;
  categoryFormatter?: (value: string) => string;

  /** Chart plot height (px). Default 280 */
  height?: number;
  className?: string;

  /** Optional chrome */
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  footer?: ReactNode;

  loading?: boolean;
  emptyTitle?: string;
  emptyDescription?: string;

  /** Pie / donut radii */
  innerRadius?: number;
  outerRadius?: number;

  /** Margin overrides */
  margin?: { top?: number; right?: number; bottom?: number; left?: number };
};
