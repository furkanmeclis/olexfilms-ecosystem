"use client";

import { numberValue } from "@/features/performance/lib/performance";
import { achievementWidth } from "@/features/performance/lib/targets";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * Target achievement bar (capped at 100 %) with the percentage; the fill
 * grows from the inline start, so it follows RTL.
 */
export function AchievementBar({
  pct,
  className,
}: {
  pct: string | null | undefined;
  className?: string;
}) {
  const { format } = useLocale();
  const value = numberValue(pct);
  if (value == null) {
    return <span className="text-muted-foreground">—</span>;
  }
  const width = achievementWidth(value);
  return (
    <div
      className={cn("flex min-w-28 items-center gap-2", className)}
      data-testid="achievement-bar"
    >
      <div
        className="bg-muted h-2 flex-1 overflow-hidden rounded-full"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(width)}
      >
        <div
          className={cn(
            "h-full rounded-full",
            value >= 100
              ? "bg-emerald-500"
              : value >= 50
                ? "bg-amber-500"
                : "bg-red-500",
          )}
          style={{ inlineSize: `${width}%` }}
        />
      </div>
      <span className="text-xs tabular-nums">
        {format.number(value, { maximumFractionDigits: 1 })}%
      </span>
    </div>
  );
}
