"use client";

/** Shared recharts styling: hairline grid, receding axes, muted ticks. */
export const AXIS_TICK = { fill: "var(--viz-muted)", fontSize: 11 } as const;

export const gridProps = {
  stroke: "var(--viz-grid)",
  strokeWidth: 1,
  vertical: false,
} as const;

export const xAxisProps = {
  tickLine: false,
  axisLine: { stroke: "var(--viz-axis)" },
  tick: AXIS_TICK,
  tickMargin: 8,
  minTickGap: 24,
} as const;

export const yAxisProps = {
  tickLine: false,
  axisLine: false,
  tick: AXIS_TICK,
  width: 56,
} as const;

/** Crosshair for line / area charts. */
export const crosshairCursor = {
  stroke: "var(--viz-axis)",
  strokeWidth: 1,
} as const;

/** Hover band for bar charts. */
export const bandCursor = {
  fill: "var(--viz-grid)",
  fillOpacity: 0.5,
} as const;

/** Series count from which a legend is shown / direct labels are drawn. */
export const LEGEND_MIN_SERIES = 2;
export const DIRECT_LABEL_MAX_SERIES = 4;
