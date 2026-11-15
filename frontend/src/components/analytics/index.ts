/**
 * TEC-1211 analytics kit: palette + tokens, charts (recharts), KpiTile,
 * PeriodBar (URL state) and the Excel-like DataGrid. Adapters for the
 * report envelope v2 (TEC-1210) live in ./adapters.
 */
export * from "./palette";
export * from "./format";
export * from "./types";
export * from "./period";
export * from "./adapters";
export { usePeriodState } from "./use-period-state";
export { useSeriesColors } from "./use-series-colors";
export { PeriodBar, type PeriodBarProps } from "./period-bar";
export { KpiTile, KpiGrid, type KpiTileProps } from "./kpi-tile";
export { DeltaBadge, DeltaText } from "./delta-badge";
export { Sparkline, sparklinePath, type SparklineProps } from "./sparkline";
export {
  ChartFrame,
  ChartLegend,
  ChartDataTable,
  type ChartTable,
  type LegendItem,
} from "./chart-frame";
export { VizTooltipCard, type TooltipRow } from "./viz-tooltip";
export { TrendChart, type TrendChartProps } from "./trend-chart";
export { BarChart, type BarChartProps } from "./bar-chart";
export { RankBar, type RankBarProps, type RankBarItem } from "./rank-bar";
export { DonutChart, type DonutChartProps } from "./donut-chart";
export {
  Heatmap,
  heatmapMatrix,
  type HeatmapProps,
  type HeatmapCell,
} from "./heatmap";
export { DataGrid, StatusBadge, type DataGridProps } from "./data-grid";
