"use client";

import {
  progressTone,
  warrantyProgress,
  type Warranty,
} from "@/features/warranty/lib/warranty-list";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const TONE_CLASS = {
  success: "bg-emerald-500",
  warning: "bg-amber-500",
  muted: "bg-muted-foreground/40",
} as const;

/**
 * Days left and the elapsed share of start → end (TEC-191). The bar grows
 * along the inline axis, so it fills right-to-left in RTL locales.
 */
export function WarrantyProgressBar({
  warranty,
  now,
  compact = false,
}: {
  warranty: Pick<Warranty, "status" | "start_at" | "end_at">;
  now?: Date;
  compact?: boolean;
}) {
  const { t } = useLocale();
  const p = warrantyProgress(warranty.start_at, warranty.end_at, now);
  const tone = progressTone(warranty.status, p);
  const label =
    warranty.status === "void"
      ? t("warranty.progress.void")
      : p.ended || warranty.status === "expired"
        ? t("warranty.progress.ended")
        : t("warranty.progress.days_left", { days: p.daysLeft });
  return (
    <div
      className={cn("space-y-1", compact ? "min-w-32" : "w-full")}
      data-testid="warranty-progress"
      data-percent={p.percent}
      data-days-left={p.daysLeft}
    >
      <div className="flex items-center justify-between gap-2 text-xs">
        <span
          className={cn(
            "font-medium",
            tone === "warning" && "text-amber-600 dark:text-amber-400",
            tone === "muted" && "text-muted-foreground",
          )}
        >
          {label}
        </span>
        <span className="text-muted-foreground tabular-nums">
          {t("warranty.progress.elapsed", { percent: p.percent })}
        </span>
      </div>
      <div
        className="bg-muted h-2 w-full overflow-hidden rounded-full"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={p.percent}
        aria-label={t("warranty.progress.aria")}
      >
        <div
          className={cn("h-full rounded-full", TONE_CLASS[tone])}
          style={{ inlineSize: `${p.percent}%` }}
        />
      </div>
    </div>
  );
}
