"use client";

import { ChevronRight } from "lucide-react";
import Link from "next/link";
import { useCallback, useMemo, type ReactNode } from "react";

import {
  categoriesFromSeries,
  DataGrid,
  DonutChart,
  envelopeGranularity,
  gridRowsFromReport,
  KpiGrid,
  RankBar,
  timeSeriesFromReport,
  TrendChart,
  unitFormat,
  type KpiTileProps,
  type ReportEnvelopeV2,
} from "@/components/analytics";
import { ErrorState } from "@/components/common/error-state";
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { isUnavailable, useReport } from "@/features/reports/hooks/use-report";
import {
  asV2,
  rankingPointHref,
  reportGridColumns,
  reportHref,
  reportKpiTiles,
  splitSeries,
} from "@/features/reports/lib/envelope";
import {
  reportsService,
  type ReportQuery,
} from "@/features/reports/services/reports.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/lib/toast";

/** Query string of the period (kept when opening the report detail). */
export function periodSearch(query: ReportQuery): string {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === null || v === undefined || v === "" || k === "limit") continue;
    params.set(k, String(v));
  }
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

/** "Open report" header action: drill-down to the report detail page. */
export function OpenReportLink({
  slug,
  reportKey,
  query,
}: {
  slug: string;
  reportKey: string;
  query: ReportQuery;
}) {
  const { t } = useLocale();
  return (
    <Link
      href={`${routes.tenant.reports.detail(slug, reportKey)}${periodSearch(query)}`}
      className="text-primary inline-flex items-center gap-1 text-sm underline-offset-4 hover:underline"
      data-testid={`open-report-${reportKey}`}
    >
      {t("reports.panel.open")}
      <ChevronRight className="size-4 rtl:rotate-180" aria-hidden />
    </Link>
  );
}

type PanelProps = {
  slug: string;
  reportKey: string;
  query: ReportQuery;
  title?: string;
  description?: string;
  /** Extra query (limit ...). */
  extra?: ReportQuery;
  className?: string;
};

/** Error text of a failed panel (the reason, not a silent gap). */
function useErrorText() {
  const { t } = useLocale();
  return t("reports.panel.error");
}

/** KPI strip of one report (kpis[]); unavailable reports render nothing. */
export function ReportKpiStrip({
  slug,
  reportKey,
  query,
  extra,
  map,
  className,
}: PanelProps & { map?: (tiles: KpiTileProps[]) => KpiTileProps[] }) {
  const { t } = useLocale();
  const report = useReport(slug, reportKey, { ...query, ...extra });
  if (report.error && isUnavailable(report.error)) return null;
  if (report.isError && !report.data) {
    return (
      <ErrorState
        className="py-8"
        title={t("reports.panel.kpi_error")}
        onRetry={() => void report.refetch()}
        retryLabel={t("reports.retry")}
      />
    );
  }
  const tiles = report.data ? reportKpiTiles(asV2(report.data), slug) : [];
  return (
    <div data-testid={`kpis-${reportKey}`} className={className}>
      <KpiGrid items={map ? map(tiles) : tiles} loading={report.isPending} />
    </div>
  );
}

/** Trend chart of a report (date series, previous period dashed). */
export function ReportTrendPanel({
  slug,
  reportKey,
  query,
  title,
  description,
  extra,
  kind = "line",
  height = 260,
  seriesKeys,
  className,
}: PanelProps & {
  kind?: "line" | "area";
  height?: number;
  /** Only these series (default: every date series). */
  seriesKeys?: readonly string[];
}) {
  const errorText = useErrorText();
  const report = useReport(slug, reportKey, { ...query, ...extra });
  if (report.error && isUnavailable(report.error)) return null;
  const env = report.data ? asV2(report.data) : undefined;
  const trends = splitSeries(env?.series).trends.filter(
    (s) => !seriesKeys || seriesKeys.includes(s.key),
  );
  const ts = timeSeriesFromReport(trends);
  return (
    <div data-testid={`trend-${reportKey}`} className={className}>
      <TrendChart
        kind={kind}
        title={title ?? env?.title}
        description={description}
        actions={
          <OpenReportLink slug={slug} reportKey={reportKey} query={query} />
        }
        data={ts.data}
        series={ts.series}
        format={ts.format}
        granularity={env ? envelopeGranularity(env) : undefined}
        showPrevious={ts.hasPrevious}
        height={height}
        loading={report.isPending}
        error={report.isError && !report.data ? errorText : undefined}
        onRetry={() => void report.refetch()}
      />
    </div>
  );
}

/** Ranking bars (first category series); rows open the record / list. */
export function ReportRankPanel({
  slug,
  reportKey,
  query,
  title,
  description,
  extra,
  limit = 10,
  className,
}: PanelProps & { limit?: number }) {
  const errorText = useErrorText();
  const report = useReport(slug, reportKey, { ...query, ...extra });
  if (report.error && isUnavailable(report.error)) return null;
  const env = report.data ? asV2(report.data) : undefined;
  const series = splitSeries(env?.series).categories[0];
  const items = categoriesFromSeries(series).map((item) => ({
    ...item,
    href: rankingPointHref(reportKey, item.key, slug),
  }));
  return (
    <div data-testid={`rank-${reportKey}`} className={className}>
      <RankBar
        title={title ?? env?.title}
        description={description}
        actions={
          <OpenReportLink slug={slug} reportKey={reportKey} query={query} />
        }
        items={items}
        format={unitFormat(series?.unit)}
        limit={limit}
        loading={report.isPending}
        error={report.isError && !report.data ? errorText : undefined}
        onRetry={() => void report.refetch()}
      />
    </div>
  );
}

/** Part-to-whole donut (first category series). */
export function ReportDonutPanel({
  slug,
  reportKey,
  query,
  title,
  description,
  extra,
  className,
}: PanelProps) {
  const errorText = useErrorText();
  const report = useReport(slug, reportKey, { ...query, ...extra });
  if (report.error && isUnavailable(report.error)) return null;
  const env = report.data ? asV2(report.data) : undefined;
  const series = splitSeries(env?.series).categories[0];
  return (
    <div data-testid={`donut-${reportKey}`} className={className}>
      <DonutChart
        title={title ?? env?.title}
        description={description}
        actions={
          <OpenReportLink slug={slug} reportKey={reportKey} query={query} />
        }
        items={categoriesFromSeries(series)}
        format={unitFormat(series?.unit)}
        loading={report.isPending}
        error={report.isError && !report.data ? errorText : undefined}
        onRetry={() => void report.refetch()}
      />
    </div>
  );
}

/** Server export of one report (xlsx / csv) with an error toast. */
export async function exportReport(
  reportKey: string,
  query: ReportQuery,
  format: "csv" | "xlsx",
  errorText: string,
) {
  try {
    await reportsService.exportReport(reportKey, query, format);
  } catch {
    appToast.error(errorText);
  }
}

/** DataGrid of an envelope (rows open their record). */
export function ReportGrid({
  env,
  slug,
  query,
  height,
  toolbar,
}: {
  env: ReportEnvelopeV2;
  slug: string;
  query: ReportQuery;
  height?: number;
  toolbar?: ReactNode;
}) {
  const { t } = useLocale();
  // Stable props: the grid recomputes its model when they change.
  const columns = useMemo(
    () => reportGridColumns({ columns: env.columns }),
    [env.columns],
  );
  const rows = useMemo(() => gridRowsFromReport(env.rows), [env.rows]);
  const report = env.report;
  const rowHref = useCallback(
    (row: { _link?: unknown }) => reportRowHref(row, slug, report),
    [slug, report],
  );
  return (
    <DataGrid
      columns={columns}
      rows={rows}
      slug={slug}
      rowHref={rowHref}
      persistKey={`reports.${env.report}`}
      exportName={env.report}
      caption={env.title}
      height={height}
      toolbar={toolbar}
      emptyText={t("reports.panel.empty")}
      onServerExport={(format) =>
        exportReport(env.report, query, format, t("reports.export_error"))
      }
    />
  );
}

/** Table card of a report (DataGrid). */
export function ReportTablePanel({
  slug,
  reportKey,
  query,
  title,
  extra,
  height = 360,
  className,
}: PanelProps & { height?: number }) {
  const { t } = useLocale();
  const report = useReport(slug, reportKey, { ...query, ...extra });
  if (report.error && isUnavailable(report.error)) return null;
  const env = report.data ? asV2(report.data) : undefined;
  return (
    <Card data-testid={`table-${reportKey}`} className={className}>
      <CardHeader>
        <CardTitle>{title ?? env?.title ?? ""}</CardTitle>
        <CardAction>
          <OpenReportLink slug={slug} reportKey={reportKey} query={query} />
        </CardAction>
      </CardHeader>
      <CardContent>
        {report.isError && !report.data ? (
          <ErrorState
            className="py-8"
            title={t("reports.panel.error")}
            onRetry={() => void report.refetch()}
            retryLabel={t("reports.retry")}
          />
        ) : env ? (
          <ReportGrid env={env} slug={slug} query={query} height={height} />
        ) : (
          <div className="bg-muted h-40 animate-pulse rounded-md" />
        )}
      </CardContent>
    </Card>
  );
}

function reportRowHref(
  row: { _link?: unknown },
  slug: string,
  reportKey: string,
): string | null {
  return reportHref(
    (row._link ?? null) as Parameters<typeof reportHref>[0],
    slug,
    reportKey,
  );
}
