"use client";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { measureParts, type StockMeasures } from "../lib/measures";

const TONE: Record<string, string> = {
  boxes: "text-foreground",
  open: "text-foreground",
  cut: "text-muted-foreground",
  remnant: "text-amber-700 dark:text-amber-400",
};

/**
 * TEC-1243 (K14): closed boxes, open rolls, cut pieces and remnants of a
 * product on one line, readable at a glance. Remnants are tinted: they are
 * leftovers under the minimum cut and are not offered.
 */
export function StockMeasureLine({
  measures,
  className,
}: {
  measures: StockMeasures;
  className?: string;
}) {
  const { t, format } = useLocale();
  const parts = measureParts(measures, t, format.number);
  return (
    <p
      className={cn("flex flex-wrap gap-x-1.5 text-xs", className)}
      data-testid="stock-measure-line"
    >
      {parts.map((p, i) => (
        <span key={p.kind} className={cn("whitespace-nowrap", TONE[p.kind])}>
          {i > 0 ? (
            <span className="text-muted-foreground me-1.5">·</span>
          ) : null}
          {p.text}
        </span>
      ))}
    </p>
  );
}
