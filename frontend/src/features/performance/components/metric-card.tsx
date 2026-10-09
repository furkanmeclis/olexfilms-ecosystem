"use client";

import { ArrowDownRight, ArrowUpRight, Minus } from "lucide-react";

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import {
  deltaTone,
  formatMetric,
  numberValue,
} from "@/features/performance/lib/performance";
import type { PerformanceMetricDelta } from "@/features/performance/services/performance.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Target achievement ring (0–100 % filled; the label shows the real %). */
function AchievementRing({ pct, label }: { pct: number; label: string }) {
  const radius = 16;
  const circumference = 2 * Math.PI * radius;
  const filled = Math.min(100, Math.max(0, pct)) / 100;
  return (
    <div
      className="relative size-12 shrink-0"
      role="img"
      aria-label={label}
      data-testid="metric-target-ring"
    >
      <svg viewBox="0 0 40 40" className="size-12 -rotate-90">
        <circle
          cx="20"
          cy="20"
          r={radius}
          fill="none"
          strokeWidth="4"
          className="stroke-muted"
        />
        <circle
          cx="20"
          cy="20"
          r={radius}
          fill="none"
          strokeWidth="4"
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={circumference * (1 - filled)}
          className={pct >= 100 ? "stroke-emerald-500" : "stroke-primary"}
        />
      </svg>
      <span className="absolute inset-0 flex items-center justify-center text-[10px] font-semibold tabular-nums">
        {Math.round(pct)}%
      </span>
    </div>
  );
}

/**
 * KPI card of one monthly metric: value, change against the previous month
 * and (for target metrics) the target achievement ring. A metric whose
 * module is off is grey with "module off" and no value.
 */
export function MetricCard({
  metric,
  data,
  moduleOff,
  achievement,
}: {
  metric: string;
  data: PerformanceMetricDelta | undefined;
  moduleOff: boolean;
  achievement?: number | null;
}) {
  const { t, format } = useLocale();
  const title = t(`performance.metrics.${metric}`);

  if (moduleOff) {
    return (
      <div data-testid={`metric-card-${metric}`} data-module-off="true">
        <Card className="bg-muted/40 text-muted-foreground h-full">
          <CardHeader>
            <CardTitle className="text-sm font-medium">{title}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm" data-testid="metric-module-off">
              {t("performance.module_off")}
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  const pct = numberValue(data?.delta_pct);
  const tone = deltaTone(metric, data?.delta_pct);
  const Icon =
    pct == null || pct === 0 ? Minus : pct > 0 ? ArrowUpRight : ArrowDownRight;
  return (
    <div data-testid={`metric-card-${metric}`}>
      <Card className="h-full">
        <CardHeader>
          <CardTitle className="text-muted-foreground text-sm font-medium">
            {title}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex items-end justify-between gap-3">
          <div className="min-w-0">
            <div
              className="text-2xl font-semibold tracking-tight tabular-nums"
              data-testid="metric-value"
            >
              {formatMetric(metric, data?.current, format)}
            </div>
            <p
              className={cn(
                "mt-1 flex items-center gap-1 text-xs",
                tone === "up" && "text-emerald-600",
                tone === "down" && "text-destructive",
                tone === "flat" && "text-muted-foreground",
              )}
            >
              <Icon className="size-3 rtl:-scale-x-100" aria-hidden />
              {pct == null
                ? t("performance.delta.none")
                : t("performance.delta.previous", {
                    value: format.percent(pct / 100, 1),
                  })}
            </p>
          </div>
          {achievement != null ? (
            <AchievementRing
              pct={achievement}
              label={t("performance.target_achievement", {
                value: format.percent(achievement / 100, 0),
              })}
            />
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
