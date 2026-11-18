"use client";

import { useId, useState, type ReactNode } from "react";
import { AlertTriangle, BarChart3, RotateCw, Table2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { ChartChromeProps } from "./types";

export type LegendItem = {
  key: string;
  label: string;
  color: string;
  /** Previous-period entry: dashed swatch. */
  dashed?: boolean;
};

export type ChartTable = {
  columns: Array<{ key: string; label: string; numeric?: boolean }>;
  rows: Array<{ key: string; cells: string[] }>;
};

export function ChartLegend({ items }: { items: LegendItem[] }) {
  if (items.length === 0) return null;
  return (
    <ul
      className="text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-1 text-xs"
      data-slot="viz-legend"
    >
      {items.map((item) => (
        <li key={item.key} className="flex items-center gap-1.5">
          <svg width="14" height="8" aria-hidden className="shrink-0">
            <line
              x1="1"
              y1="4"
              x2="13"
              y2="4"
              stroke={item.color}
              strokeWidth={item.dashed ? 2 : 6}
              strokeDasharray={item.dashed ? "3 2" : undefined}
              strokeLinecap="round"
            />
          </svg>
          <span className="text-foreground">{item.label}</span>
        </li>
      ))}
    </ul>
  );
}

export function ChartDataTable({
  table,
  caption,
}: {
  table: ChartTable;
  caption?: ReactNode;
}) {
  return (
    <div className="max-h-80 overflow-auto rounded-md border">
      <table className="w-full text-sm" data-slot="viz-table">
        {caption ? <caption className="sr-only">{caption}</caption> : null}
        <thead className="bg-muted/60 sticky top-0">
          <tr>
            {table.columns.map((c) => (
              <th
                key={c.key}
                scope="col"
                className={cn(
                  "px-3 py-2 font-medium",
                  c.numeric ? "text-end" : "text-start",
                )}
              >
                {c.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {table.rows.map((r) => (
            <tr key={r.key} className="border-t">
              {r.cells.map((cell, i) => (
                <td
                  key={table.columns[i]?.key ?? i}
                  className={cn(
                    "px-3 py-1.5",
                    table.columns[i]?.numeric
                      ? "text-end tabular-nums"
                      : "text-start",
                  )}
                >
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function errorMessage(error: unknown): string | null {
  if (!error) return null;
  if (typeof error === "string") return error;
  if (error instanceof Error) return error.message;
  return null;
}

type ChartFrameProps = ChartChromeProps & {
  legend?: LegendItem[];
  table?: ChartTable;
  isEmpty?: boolean;
  /** Plot; rendered only when not loading / error / empty. */
  children: ReactNode;
  testId?: string;
};

/**
 * Card chrome shared by every chart: title, legend, table-view toggle
 * (accessibility) and the loading / error / empty states.
 */
export function ChartFrame({
  title,
  description,
  actions,
  height = 260,
  className,
  bare,
  loading,
  error,
  onRetry,
  emptyText,
  legend,
  table,
  isEmpty,
  children,
  testId,
}: ChartFrameProps) {
  const { t } = useLocale();
  const [asTable, setAsTable] = useState(false);
  const headingId = useId();
  const failed = Boolean(error);
  const showTable = asTable && table && !loading && !failed && !isEmpty;

  let body: ReactNode;
  if (loading) {
    body = (
      <div style={{ height }} aria-busy="true" aria-live="polite">
        <span className="sr-only">{t("chart.loading")}</span>
        <Skeleton className="h-full w-full rounded-md" />
      </div>
    );
  } else if (failed) {
    body = (
      <div
        role="alert"
        className="flex flex-col items-center justify-center gap-2 px-4 text-center"
        style={{ height }}
        data-state="error"
      >
        <AlertTriangle
          className="size-5"
          style={{ color: "var(--viz-critical)" }}
          aria-hidden
        />
        <p className="text-sm font-medium">{t("chart.error_title")}</p>
        {errorMessage(error) ? (
          <p className="text-muted-foreground max-w-sm text-xs">
            {errorMessage(error)}
          </p>
        ) : null}
        {onRetry ? (
          <Button size="sm" variant="outline" onClick={onRetry}>
            <RotateCw aria-hidden />
            {t("chart.retry")}
          </Button>
        ) : null}
      </div>
    );
  } else if (isEmpty) {
    body = (
      <div
        className="flex flex-col items-center justify-center gap-1 px-4 text-center"
        style={{ height }}
        data-state="empty"
      >
        <p className="text-sm font-medium">
          {emptyText ?? t("chart.empty_title")}
        </p>
        <p className="text-muted-foreground text-xs">
          {t("chart.empty_description")}
        </p>
      </div>
    );
  } else if (showTable) {
    body = <ChartDataTable table={table} caption={title} />;
  } else {
    body = (
      <div style={{ height }} className="w-full">
        {children}
      </div>
    );
  }

  const toggle =
    table && !loading && !failed && !isEmpty ? (
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-pressed={asTable}
        aria-label={asTable ? t("chart.show_chart") : t("chart.show_table")}
        title={asTable ? t("chart.show_chart") : t("chart.show_table")}
        onClick={() => setAsTable((v) => !v)}
      >
        {asTable ? <BarChart3 aria-hidden /> : <Table2 aria-hidden />}
      </Button>
    ) : null;

  const hasHeader = Boolean(title || description || actions || toggle);

  return (
    <section
      aria-labelledby={title ? headingId : undefined}
      data-slot="viz-chart"
      data-testid={testId}
      className={cn(
        "flex min-w-0 flex-col gap-3",
        !bare && "rounded-xl border p-4",
        className,
      )}
      style={bare ? undefined : { background: "var(--viz-surface)" }}
    >
      {hasHeader ? (
        <header className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            {title ? (
              <h3 id={headingId} className="text-sm font-semibold">
                {title}
              </h3>
            ) : null}
            {description ? (
              <p className="text-muted-foreground text-xs">{description}</p>
            ) : null}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {actions}
            {toggle}
          </div>
        </header>
      ) : null}
      {legend && legend.length > 0 && !loading && !failed && !isEmpty ? (
        <ChartLegend items={legend} />
      ) : null}
      {body}
    </section>
  );
}

/**
 * First-render size of a ResponsiveContainer, before its ResizeObserver
 * measures the frame. Recharts' default (-1 x -1) logs a "width(-1) and
 * height(-1) of chart should be greater than 0" warning on every mount.
 */
export const CHART_INITIAL_DIMENSION = { width: 320, height: 200 } as const;
