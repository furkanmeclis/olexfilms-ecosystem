"use client";

import { FileDown } from "lucide-react";
import { useMemo } from "react";

import {
  categoriesFromSeries,
  DonutChart,
  envelopeGranularity,
  KpiGrid,
  PeriodBar,
  PERIOD_PRESETS,
  RankBar,
  timeSeriesFromReport,
  TrendChart,
  unitFormat,
  usePeriodState,
  type KpiTileProps,
  type PeriodPreset,
  type ReportSeriesV2,
} from "@/components/analytics";
import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import {
  exportReport,
  ReportGrid,
} from "@/features/reports/components/report-panels";
import {
  isUnavailable,
  useReport,
  useReportCatalog,
} from "@/features/reports/hooks/use-report";
import {
  asV2,
  rankingPointHref,
  reportKpiTiles,
  splitSeries,
} from "@/features/reports/lib/envelope";
import { useLocale } from "@/providers/locale-provider";

/** Series totals vs the previous window as tiles (reports without kpis). */
function seriesTiles(series: readonly ReportSeriesV2[]): KpiTileProps[] {
  return series
    .filter((s) => typeof s.total === "number")
    .map((s) => ({
      id: s.key,
      label: s.label,
      value: s.total ?? null,
      previous: s.previous_total ?? null,
      deltaPct: s.delta_pct ?? null,
      format: unitFormat(s.unit),
      spark: s.points.slice(-30).map((p) => p.value),
    }));
}

/**
 * TEC-1212: one report. One PeriodBar (URL state, compare defaults to the
 * previous period) drives the KPIs, the large trend chart with the previous
 * period dashed, the breakdowns (status donut / rankings), the DataGrid and
 * the server export (xlsx / csv).
 */
export function ReportDetailPage({
  slug,
  reportKey,
}: {
  slug: string;
  reportKey: string;
}) {
  const { t } = useLocale();
  const catalog = useReportCatalog(slug);
  const item = catalog.data?.items.find((i) => i.key === reportKey);
  const presets = useMemo(() => {
    const allowed = item?.periods?.length
      ? PERIOD_PRESETS.filter((p) => item.periods.includes(p))
      : PERIOD_PRESETS;
    return allowed.length ? allowed : PERIOD_PRESETS;
  }, [item]);
  const defaultPreset = (
    presets.includes(item?.default_period as PeriodPreset)
      ? item?.default_period
      : presets.includes("30d")
        ? "30d"
        : presets[0]
  ) as PeriodPreset;
  const periodic = item?.periodic ?? true;
  const [period, setPeriod, periodQuery] = usePeriodState({
    preset: defaultPreset,
    compare: "previous",
  });
  const query = periodic ? periodQuery : {};
  const report = useReport(slug, reportKey, query);
  const env = report.data ? asV2(report.data) : undefined;
  const title = env?.title ?? item?.title ?? reportKey;
  const split = splitSeries(env?.series);
  const ts = timeSeriesFromReport(split.trends);
  const kpis = env ? reportKpiTiles(env, slug) : [];
  const tiles = kpis.length ? kpis : seriesTiles(split.trends);
  const exportError = t("reports.export_error");

  const header = (
    <PageHeader
      title={title}
      breadcrumbs={[
        { label: t("reports.nav"), href: routes.tenant.reports.root(slug) },
        { label: title },
      ]}
      actions={
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" data-testid="report-export">
              <FileDown className="size-4" aria-hidden />
              {t("reports.export")}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              onSelect={() =>
                void exportReport(reportKey, query, "xlsx", exportError)
              }
            >
              {t("reports.export_xlsx")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() =>
                void exportReport(reportKey, query, "csv", exportError)
              }
            >
              {t("reports.export_csv")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      }
    />
  );

  if (report.error && isUnavailable(report.error)) {
    return (
      <div className="flex flex-col gap-6">
        {header}
        <EmptyState
          title={t("reports.detail.unavailable")}
          description={t("reports.detail.unavailable_description")}
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6" data-testid="report-detail">
      {header}
      {periodic ? (
        <PeriodBar
          value={period}
          onChange={setPeriod}
          presets={presets}
          showGranularity={(item?.granularities?.length ?? 0) > 0}
        />
      ) : null}
      {report.isError && !report.data ? (
        <ErrorState
          title={t("reports.panel.error")}
          onRetry={() => void report.refetch()}
          retryLabel={t("reports.retry")}
        />
      ) : report.isPending ? (
        <div className="flex flex-col gap-4">
          <KpiGrid items={[]} loading />
          <Skeleton className="h-[360px] rounded-xl" />
        </div>
      ) : env ? (
        <>
          {tiles.length ? (
            <div data-testid="report-kpis">
              <KpiGrid items={tiles} />
            </div>
          ) : null}
          {split.trends.length ? (
            <div data-testid="report-trend">
              <TrendChart
                title={t("reports.detail.trend")}
                description={t("reports.detail.trend_description")}
                data={ts.data}
                series={ts.series}
                format={ts.format}
                granularity={envelopeGranularity(env)}
                showPrevious={ts.hasPrevious}
                height={360}
              />
            </div>
          ) : null}
          {split.categories.length ? (
            <div
              className="grid gap-6 xl:grid-cols-2"
              data-testid="report-breakdowns"
            >
              {split.categories.map((s) =>
                env.kind === "distribution" || s.key === "status" ? (
                  <DonutChart
                    key={s.key}
                    title={s.label}
                    items={categoriesFromSeries(s)}
                    format={unitFormat(s.unit)}
                  />
                ) : (
                  <RankBar
                    key={s.key}
                    title={s.label}
                    items={categoriesFromSeries(s).map((p) => ({
                      ...p,
                      href: rankingPointHref(reportKey, p.key, slug),
                    }))}
                    format={unitFormat(s.unit)}
                    limit={15}
                  />
                ),
              )}
            </div>
          ) : null}
          {env.columns?.length ? (
            <div data-testid="report-grid">
              <ReportGrid env={env} slug={slug} query={query} height={520} />
            </div>
          ) : null}
          {!tiles.length &&
          !split.trends.length &&
          !split.categories.length &&
          !env.columns?.length ? (
            <EmptyState title={t("reports.panel.empty")} />
          ) : null}
        </>
      ) : null}
    </div>
  );
}
