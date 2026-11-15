import type { ReactNode } from "react";

import type { ValueFormat, ValueType } from "./format";

/** Key suffix of a series' previous-period value in chart rows. */
export const PREVIOUS_SUFFIX = "__prev";

export function previousKey(key: string): string {
  return `${key}${PREVIOUS_SUFFIX}`;
}

/** One series drawn by a chart. */
export type ChartSeriesDef = {
  key: string;
  label: string;
  /**
   * Fixed categorical slot (0..7). When omitted the chart assigns slots by
   * identity (first seen keeps its slot, see assignColorSlots).
   */
  colorSlot?: number;
  format?: ValueFormat;
};

/** One row of a cartesian chart: `key` is the bucket / category. */
export type ChartDatum = {
  key: string;
  /** Category label (falls back to the formatted key). */
  label?: string;
  [series: string]: string | number | null | undefined;
};

/** One slice / bar of a single-series category chart. */
export type CategoryDatum = {
  key: string;
  label: string;
  value: number;
  /** Fixed colour slot for donut slices; default = position. */
  colorSlot?: number;
};

/** Shared loading / error / empty contract of every chart. */
export type ChartStateProps = {
  loading?: boolean;
  /** Error object or message; shows the error state with a retry button. */
  error?: unknown;
  onRetry?: () => void;
  /** Empty state text (default: chart.empty_title). */
  emptyText?: string;
};

export type ChartChromeProps = ChartStateProps & {
  title?: ReactNode;
  description?: ReactNode;
  /** Extra header actions (next to the table-view toggle). */
  actions?: ReactNode;
  height?: number;
  className?: string;
  /** Hide the card chrome (title, border); keeps legend + states. */
  bare?: boolean;
};

export type ReportLinkKind =
  | "service"
  | "order"
  | "customer"
  | "organization"
  | "warranty"
  | "measurement"
  | "product";

export type ReportLink = { kind: ReportLinkKind; uuid: string };

export type StatusTone = "neutral" | "progress" | "done" | "problem";

export type StatusCell = { label: string; tone: StatusTone };

/** A DataGrid row: plain record plus the optional envelope extras. */
export type GridRow = Record<string, unknown> & {
  _link?: ReportLink | null;
  _status?: Record<string, StatusCell> | null;
  currency?: string | null;
};

export type GridAggregate = "sum" | "avg" | "none";

export type GridFilterKind = "text" | "range" | "select" | "none";

export type GridColumn<R extends GridRow = GridRow> = {
  key: string;
  label: string;
  type?: ValueType;
  align?: "left" | "right";
  aggregate?: GridAggregate;
  /** Column-level currency for money (row `currency` wins). */
  currency?: string;
  /** Filter UI; default from type (number-like → range, status → select). */
  filter?: GridFilterKind;
  /** Options for select filters (default: distinct values in data). */
  options?: Array<{ value: string; label: string }>;
  /** Initial width in px. */
  width?: number;
  /** Custom raw value (sort, filter, export). Default: row[key]. */
  accessor?: (row: R) => unknown;
  /** Custom cell renderer (formatted text is still used for export). */
  render?: (row: R) => ReactNode;
  /** Allow "group by" on this column (default: text/status/org/date). */
  groupable?: boolean;
  /** Fraction digits for number/percent. */
  maximumFractionDigits?: number;
};
