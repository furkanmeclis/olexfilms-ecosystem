"use client";

import { quotaTone } from "@/features/ai-admin/lib/quota";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const TONE_CLASS = {
  default: "bg-primary",
  warning: "bg-amber-500",
  danger: "bg-destructive",
} as const;

/**
 * Usage percent bar; `percent` null means an unlimited quota (no bar).
 * Width grows from the inline start, so it follows RTL.
 */
export function QuotaBar({
  percent,
  className,
}: {
  percent: number | null | undefined;
  className?: string;
}) {
  const { t, format } = useLocale();
  if (percent == null) {
    return (
      <span className="text-muted-foreground text-sm">
        {t("ai_admin.quota.unlimited")}
      </span>
    );
  }
  const width = Math.min(Math.max(percent, 0), 100);
  const tone = quotaTone(percent);
  return (
    <div className={cn("flex min-w-28 items-center gap-2", className)}>
      <div
        className="bg-muted relative h-2 flex-1 overflow-hidden rounded-full"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(width)}
      >
        <div
          className={cn("h-full rounded-full", TONE_CLASS[tone])}
          style={{ width: `${width}%` }}
        />
      </div>
      <span
        className={cn(
          "w-12 text-end text-xs tabular-nums",
          tone === "danger" && "text-destructive font-medium",
        )}
      >
        {format.number(Math.round(percent * 10) / 10)}%
      </span>
    </div>
  );
}
