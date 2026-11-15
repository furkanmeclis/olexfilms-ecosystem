"use client";

import Link from "next/link";
import { ChevronRight } from "lucide-react";

import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useFormatter, useLocale } from "@/providers/locale-provider";

import { DeltaBadge } from "./delta-badge";
import {
  deltaPct as computeDelta,
  formatValue,
  type ValueFormat,
} from "./format";
import { Sparkline } from "./sparkline";
import { useDurationLabels } from "./use-duration-labels";

export type KpiTileProps = {
  /** Stable id (KPI key). */
  id?: string;
  label: string;
  value: number | null | undefined;
  /** Previous-period value (shown under the delta). */
  previous?: number | null;
  /** Percent change; computed from value/previous when omitted. */
  deltaPct?: number | null;
  /** Which direction is good (default up). */
  good?: "up" | "down";
  format?: ValueFormat;
  /** Period buckets, at most ~30. */
  spark?: readonly number[];
  /** Detail page. */
  href?: string;
  /** Short hint under the value. */
  hint?: string;
  loading?: boolean;
  className?: string;
};

/**
 * A single number with its context: value, delta badge (icon + text, good
 * or bad by `good`), previous value, sparkline, optional link.
 */
export function KpiTile({
  id,
  label,
  value,
  previous,
  deltaPct,
  good = "up",
  format,
  spark,
  href,
  hint,
  loading,
  className,
}: KpiTileProps) {
  const fmt = useFormatter();
  const { t } = useLocale();
  const durations = useDurationLabels();
  const delta =
    deltaPct !== undefined
      ? deltaPct
      : previous !== undefined
        ? computeDelta(value, previous)
        : undefined;
  const hasPrevious = previous !== undefined;

  const body = (
    <>
      <div className="flex items-start justify-between gap-2">
        <span className="text-muted-foreground text-sm font-medium">
          {label}
        </span>
        {href ? (
          <ChevronRight
            className="text-muted-foreground size-4 shrink-0 rtl:rotate-180"
            aria-hidden
          />
        ) : null}
      </div>
      {loading ? (
        <Skeleton className="h-8 w-28" />
      ) : (
        <div className="flex items-end justify-between gap-3">
          <div className="min-w-0">
            <div className="text-foreground text-2xl font-semibold tracking-tight tabular-nums">
              {formatValue(value, format, fmt, durations)}
            </div>
            {hasPrevious || delta !== undefined ? (
              <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5">
                {delta !== undefined ? (
                  <DeltaBadge deltaPct={delta} good={good} />
                ) : null}
                {hasPrevious ? (
                  <span className="text-muted-foreground text-xs">
                    {t("chart.kpi.previous", {
                      value: formatValue(previous, format, fmt, durations),
                    })}
                  </span>
                ) : null}
              </div>
            ) : null}
            {hint ? (
              <p className="text-muted-foreground mt-1 text-xs">{hint}</p>
            ) : null}
          </div>
          {spark && spark.length > 1 ? (
            <Sparkline values={spark} className="shrink-0" />
          ) : null}
        </div>
      )}
    </>
  );

  const classes = cn(
    "bg-card text-card-foreground flex flex-col gap-2 rounded-xl border p-4",
    href &&
      "hover:bg-muted/40 focus-visible:ring-ring/50 outline-none transition-colors focus-visible:ring-[3px]",
    className,
  );

  return href ? (
    <Link href={href} className={classes} data-slot="kpi-tile" data-kpi={id}>
      {body}
      <span className="sr-only">{t("chart.kpi.open")}</span>
    </Link>
  ) : (
    <div className={classes} data-slot="kpi-tile" data-kpi={id}>
      {body}
    </div>
  );
}

/** Responsive row of KPI tiles. */
export function KpiGrid({
  items,
  loading,
  className,
}: {
  items: KpiTileProps[];
  loading?: boolean;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4",
        className,
      )}
      data-slot="kpi-grid"
    >
      {items.map((k, i) => (
        <KpiTile key={k.id ?? i} {...k} loading={loading || k.loading} />
      ))}
    </div>
  );
}
