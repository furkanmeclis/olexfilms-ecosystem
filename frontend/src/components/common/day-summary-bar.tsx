"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { localToday, shiftDate } from "@/lib/utils/local-date";
import { useLocale } from "@/providers/locale-provider";

export type DaySummaryStat = {
  label: string;
  value: string;
  /** Highlight the headline figure (e.g. collected total). */
  accent?: boolean;
};

/**
 * Day picker (prev / date / next / today) + compact figure strip used by the
 * daily pages (operations, sales, purchases) so they share one layout.
 */
export function DaySummaryBar({
  date,
  onDateChange,
  stats,
  loading,
  live,
  className,
}: {
  date: string;
  onDateChange: (date: string) => void;
  stats: DaySummaryStat[];
  loading?: boolean;
  /** Show the "live" pill when viewing today (auto-refreshing pages). */
  live?: boolean;
  className?: string;
}) {
  const { t } = useLocale();
  const isToday = date === localToday();

  return (
    <div
      className={cn(
        "bg-card mb-4 flex flex-col gap-3 rounded-2xl border p-3 lg:flex-row lg:items-center",
        className,
      )}
    >
      <div className="flex items-center gap-1">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={t("common.day.prev")}
          onClick={() => onDateChange(shiftDate(date, -1))}
        >
          <ChevronLeft className="size-4" />
        </Button>
        <DatePicker
          value={date}
          onChange={onDateChange}
          className="w-[10.5rem]"
          aria-label={t("common.day.pick")}
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={t("common.day.next")}
          onClick={() => onDateChange(shiftDate(date, 1))}
        >
          <ChevronRight className="size-4" />
        </Button>
        {!isToday ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => onDateChange(localToday())}
          >
            {t("common.day.today")}
          </Button>
        ) : live ? (
          <span className="ml-1 flex items-center gap-1.5 rounded-full bg-emerald-500/10 px-2.5 py-1 text-xs font-medium text-emerald-700 dark:text-emerald-400">
            <span className="size-1.5 animate-pulse rounded-full bg-emerald-500" />
            {t("common.day.live")}
          </span>
        ) : null}
      </div>
      <dl
        className={cn(
          "grid flex-1 grid-cols-2 gap-x-4 gap-y-2 lg:border-l lg:pl-4",
          stats.length >= 5
            ? "sm:grid-cols-5"
            : stats.length === 4
              ? "sm:grid-cols-4"
              : "sm:grid-cols-3",
        )}
      >
        {stats.map((stat) => (
          <div key={stat.label} className="min-w-0">
            <dt className="text-muted-foreground truncate text-xs">
              {stat.label}
            </dt>
            <dd
              className={cn(
                "truncate text-base font-semibold tabular-nums",
                stat.accent && "text-primary",
              )}
            >
              {loading ? <Skeleton className="mt-1 h-5 w-16" /> : stat.value}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
