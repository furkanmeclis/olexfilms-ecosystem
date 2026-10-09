"use client";

import { useQuery } from "@tanstack/react-query";

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import { ErrorState } from "@/components/common/error-state";
import { Skeleton } from "@/components/ui/skeleton";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import {
  deltaTone,
  formatMetric,
  lowerIsBetter,
  metricModuleOff,
  metricNumber,
  PERFORMANCE_METRICS,
} from "@/features/performance/lib/performance";
import {
  performanceKeys,
  performanceService,
} from "@/features/performance/services/performance.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * "My position in the network" (dealer): every metric of the dealer next to
 * the network average of the dealers (GET /v1/performance/benchmark, no
 * peer names), with an above / below average marker. Shown to dealers in
 * place of the ranking table.
 */
export function PositionCard({
  slug,
  period,
}: {
  slug: string;
  period: string;
}) {
  const { t, format } = useLocale();
  const enabled = useEnabledFeatures(slug);
  const benchmark = useQuery({
    queryKey: performanceKeys.benchmark(period),
    queryFn: () => performanceService.benchmark(period),
  });

  return (
    <div data-testid="performance-position-card">
      <Card>
        <CardHeader>
          <CardTitle>{t("performance.position.title")}</CardTitle>
          <p className="text-muted-foreground text-sm">
            {t("performance.position.description")}
          </p>
        </CardHeader>
        <CardContent>
          {benchmark.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              onRetry={() => void benchmark.refetch()}
            />
          ) : benchmark.isLoading ? (
            <Skeleton className="h-48" />
          ) : (
            <dl className="divide-border grid divide-y">
              <div className="text-muted-foreground grid grid-cols-3 gap-2 pb-2 text-xs">
                <dt>{t("performance.position.metric")}</dt>
                <dd className="text-end">{t("performance.position.mine")}</dd>
                <dd className="text-end">
                  {t("performance.position.network")}
                </dd>
              </div>
              {PERFORMANCE_METRICS.map((metric) => {
                const off = metricModuleOff(metric, enabled);
                const own = benchmark.data?.own?.metrics?.[metric];
                const avg = benchmark.data?.network_average?.[metric];
                const ownN = metricNumber(own);
                const avgN = metricNumber(avg);
                const diff =
                  ownN != null && avgN != null && avgN !== 0
                    ? ((ownN - avgN) / Math.abs(avgN)) * 100
                    : null;
                const tone = deltaTone(metric, diff);
                return (
                  <div
                    key={metric}
                    className={cn(
                      "grid grid-cols-3 items-center gap-2 py-2 text-sm",
                      off && "text-muted-foreground",
                    )}
                    data-testid={`position-${metric}`}
                  >
                    <dt>{t(`performance.metrics.${metric}`)}</dt>
                    {off ? (
                      <dd className="col-span-2 text-end text-xs">
                        {t("performance.module_off")}
                      </dd>
                    ) : (
                      <>
                        <dd className="text-end font-medium tabular-nums">
                          {formatMetric(
                            metric,
                            own,
                            format,
                            benchmark.data?.own?.currency,
                          )}
                          {diff != null ? (
                            <span
                              className={cn(
                                "ms-2 text-xs",
                                tone === "up" && "text-emerald-600",
                                tone === "down" && "text-destructive",
                              )}
                              title={
                                lowerIsBetter(metric)
                                  ? t("performance.position.lower_better")
                                  : undefined
                              }
                            >
                              {t(
                                tone === "up"
                                  ? "performance.position.better"
                                  : tone === "down"
                                    ? "performance.position.worse"
                                    : "performance.position.same",
                              )}
                            </span>
                          ) : null}
                        </dd>
                        <dd className="text-muted-foreground text-end tabular-nums">
                          {formatMetric(metric, avg, format)}
                        </dd>
                      </>
                    )}
                  </div>
                );
              })}
            </dl>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
