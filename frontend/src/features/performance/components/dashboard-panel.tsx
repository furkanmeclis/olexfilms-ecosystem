"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { AppChart } from "@/components/charts";
import { ErrorState } from "@/components/common/error-state";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import { MetricCard } from "@/features/performance/components/metric-card";
import {
  metricKind,
  metricModuleOff,
  numberValue,
  PERFORMANCE_METRICS,
  targetAchievement,
  TARGET_METRICS,
} from "@/features/performance/lib/performance";
import {
  performanceKeys,
  performanceService,
} from "@/features/performance/services/performance.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Performance panel: one KPI card per monthly metric (value, change against
 * the previous month, target achievement ring for target metrics) and the
 * 12-month trend of a selected metric. Metrics of disabled modules are
 * grey "module off" cards and are left out of the trend picker.
 */
export function DashboardPanel({
  slug,
  period,
}: {
  slug: string;
  period: string;
}) {
  const { t, format } = useLocale();
  const enabled = useEnabledFeatures(slug);
  const dashboard = useQuery({
    queryKey: performanceKeys.dashboard(period),
    queryFn: () => performanceService.dashboard(period),
  });
  const [trendMetric, setTrendMetric] = useState<string>("services_count");
  const trendMetrics = PERFORMANCE_METRICS.filter(
    (m) => !metricModuleOff(m, enabled),
  );
  const activeTrend = trendMetrics.includes(
    trendMetric as (typeof trendMetrics)[number],
  )
    ? trendMetric
    : "services_count";

  const trendData = useMemo(() => {
    const ratio = metricKind(activeTrend) === "ratio";
    return (dashboard.data?.trend ?? []).map((point) => {
      const n = numberValue(point.metrics?.[activeTrend]?.value);
      return {
        period: point.period ?? "",
        value: n == null ? null : ratio ? Math.round(n * 1000) / 10 : n,
      };
    });
  }, [activeTrend, dashboard.data?.trend]);

  if (dashboard.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void dashboard.refetch()}
      />
    );
  }
  if (dashboard.isLoading || !dashboard.data) {
    return (
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className="h-28" />
        ))}
      </div>
    );
  }

  const data = dashboard.data;
  const targets = data.targets ?? [];
  return (
    <div className="space-y-6">
      <div
        className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4"
        data-testid="performance-kpis"
      >
        {PERFORMANCE_METRICS.map((metric) => (
          <MetricCard
            key={metric}
            metric={metric}
            data={data.metrics?.[metric]}
            moduleOff={metricModuleOff(metric, enabled)}
            achievement={
              (TARGET_METRICS as readonly string[]).includes(metric)
                ? targetAchievement(targets, metric, period)
                : null
            }
          />
        ))}
      </div>
      <AppChart
        type="line"
        title={t("performance.trend.title")}
        description={t("performance.trend.description")}
        data={trendData}
        categoryKey="period"
        config={{ value: { label: t(`performance.metrics.${activeTrend}`) } }}
        series={["value"]}
        curved
        emptyTitle={t("performance.empty")}
        valueFormatter={(v) =>
          metricKind(activeTrend) === "ratio"
            ? `${format.number(v, { maximumFractionDigits: 1 })}%`
            : format.number(v, { maximumFractionDigits: 2 })
        }
        height={260}
        actions={
          <Select value={activeTrend} onValueChange={setTrendMetric}>
            <SelectTrigger
              className="w-56"
              aria-label={t("performance.trend.metric")}
              data-testid="performance-trend-metric"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {trendMetrics.map((m) => (
                <SelectItem key={m} value={m}>
                  {t(`performance.metrics.${m}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        }
      />
    </div>
  );
}
