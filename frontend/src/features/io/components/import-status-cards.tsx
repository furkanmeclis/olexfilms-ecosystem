"use client";

import { useCallback } from "react";

import {
  importStatusCounts,
  importStatusTone,
  type ImportStatusTone,
} from "@/features/io/lib/import-summary";
import type { ImportPreviewSummary } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

const TONE_TEXT: Record<ImportStatusTone, string> = {
  success: "text-emerald-700 dark:text-emerald-300",
  warning: "text-amber-700 dark:text-amber-300",
  danger: "text-destructive",
  default: "text-foreground",
};

/** Localized row status: the backend label of a row, else imports.row_status.*. */
export function useImportStatusLabel() {
  const { t } = useLocale();
  return useCallback(
    (status: string, backendLabel?: string) => {
      const key = `imports.row_status.${status}`;
      const text = t(key);
      if (text !== key) return text;
      return backendLabel || status;
    },
    [t],
  );
}

type ImportStatusCardsProps = {
  summary: ImportPreviewSummary;
  /** Selected status filter; null = all rows. */
  selected?: string | null;
  onSelect?: (status: string | null) => void;
};

/** Row counts per status; a card click filters the row table. */
export function ImportStatusCards({
  summary,
  selected = null,
  onSelect,
}: ImportStatusCardsProps) {
  const { t } = useLocale();
  const label = useImportStatusLabel();
  const cards = [
    { status: null as string | null, count: summary.total ?? 0 },
    ...importStatusCounts(summary),
  ];

  return (
    <div
      className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5"
      data-testid="import-status-cards"
    >
      {cards.map(({ status, count }) => {
        const active = selected === status;
        const tone = status ? importStatusTone(status) : "default";
        return (
          <button
            key={status ?? "all"}
            type="button"
            aria-pressed={active}
            disabled={!onSelect}
            onClick={() => onSelect?.(active && status ? null : status)}
            data-testid={`import-status-card-${status ?? "all"}`}
            className={cn(
              "rounded-lg border p-3 text-start transition-colors",
              onSelect && "hover:bg-muted/60 cursor-pointer",
              active && "border-primary ring-primary/30 bg-primary/5 ring-2",
            )}
          >
            <div
              className={cn(
                "text-2xl font-semibold tabular-nums",
                TONE_TEXT[tone],
              )}
            >
              {count}
            </div>
            <div className="text-muted-foreground text-xs">
              {status ? label(status) : t("imports.preview_total")}
            </div>
          </button>
        );
      })}
    </div>
  );
}
