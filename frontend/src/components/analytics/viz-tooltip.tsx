"use client";

import type { ReactNode } from "react";

import { useFormatter, useLocale } from "@/providers/locale-provider";

import { DeltaText } from "./delta-badge";
import { deltaPct, formatValue, type ValueFormat } from "./format";
import { useDurationLabels } from "./use-duration-labels";

export type TooltipRow = {
  key: string;
  label: string;
  color: string;
  value: number | null | undefined;
  previous?: number | null;
  format?: ValueFormat;
  good?: "up" | "down";
};

/** Tooltip card: title + one line per series (swatch, label, value, delta). */
export function VizTooltipCard({
  title,
  rows,
  footer,
}: {
  title: ReactNode;
  rows: TooltipRow[];
  footer?: ReactNode;
}) {
  const fmt = useFormatter();
  const { t } = useLocale();
  const durations = useDurationLabels();
  const withPrevious = rows.some((r) => r.previous !== undefined);
  return (
    <div
      className="bg-popover text-popover-foreground min-w-40 rounded-md border px-3 py-2 text-xs shadow-md"
      data-slot="viz-tooltip"
    >
      <div className="mb-1 font-medium">{title}</div>
      <ul className="flex flex-col gap-1">
        {rows.map((r) => (
          <li key={r.key} className="flex items-center gap-2">
            <span
              aria-hidden
              className="inline-block size-2.5 shrink-0 rounded-sm"
              style={{ background: r.color }}
            />
            <span className="text-muted-foreground min-w-0 flex-1 truncate">
              {r.label}
            </span>
            <span className="text-foreground font-medium tabular-nums">
              {formatValue(r.value, r.format, fmt, durations)}
            </span>
            {withPrevious ? (
              <DeltaText
                deltaPct={deltaPct(r.value, r.previous)}
                good={r.good}
                className="w-14 text-end"
              />
            ) : null}
          </li>
        ))}
      </ul>
      {withPrevious ? (
        <div className="text-muted-foreground mt-1 border-t pt-1">
          {rows.map((r) =>
            r.previous !== undefined ? (
              <div key={r.key} className="flex justify-between gap-3">
                <span className="truncate">
                  {t("chart.previous_of", { label: r.label })}
                </span>
                <span className="tabular-nums">
                  {formatValue(r.previous, r.format, fmt, durations)}
                </span>
              </div>
            ) : null,
          )}
        </div>
      ) : null}
      {footer}
    </div>
  );
}
