"use client";

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
  type ReportEnvelopeV2,
} from "@/components/analytics";
import {
  asV2,
  rankingPointHref,
  reportGridColumns,
  reportHref,
  reportKpiTiles,
  splitSeries,
} from "@/features/reports/lib/envelope";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-1212: compact body of one report envelope with the analytics kit
 * (corporate dashboard widgets): KPIs, the trend (previous period dashed),
 * the first breakdown and the table. Replaces the summary-flattening
 * renderer (series were drawn as cards).
 */
export function CompactReportBody({
  report,
  slug,
}: {
  report: unknown;
  slug: string;
}) {
  const { t } = useLocale();
  const env: ReportEnvelopeV2 = asV2(report);
  const kpis = reportKpiTiles(env, slug);
  const split = splitSeries(env.series);
  const ts = timeSeriesFromReport(split.trends);
  const category = split.categories[0];
  const empty =
    !kpis.length &&
    !split.trends.length &&
    !category &&
    !(env.rows?.length ?? 0);
  if (empty) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("reports.panel.empty")}
      </p>
    );
  }
  return (
    <div className="flex flex-col gap-4">
      {kpis.length ? <KpiGrid items={kpis} /> : null}
      {split.trends.length ? (
        <TrendChart
          bare
          data={ts.data}
          series={ts.series}
          format={ts.format}
          granularity={envelopeGranularity(env)}
          showPrevious={ts.hasPrevious}
          height={220}
        />
      ) : null}
      {category ? (
        env.kind === "distribution" ? (
          <DonutChart
            bare
            items={categoriesFromSeries(category)}
            format={unitFormat(category.unit)}
            height={220}
          />
        ) : (
          <RankBar
            bare
            items={categoriesFromSeries(category).map((p) => ({
              ...p,
              href: rankingPointHref(env.report, p.key, slug),
            }))}
            format={unitFormat(category.unit)}
          />
        )
      ) : null}
      {env.columns?.length ? (
        <DataGrid
          columns={reportGridColumns(env)}
          rows={gridRowsFromReport(env.rows)}
          slug={slug}
          rowHref={(row) =>
            reportHref(
              (row._link ?? null) as Parameters<typeof reportHref>[0],
              slug,
              env.report,
            )
          }
          persistKey={`tenant-report-${env.report}-v2`}
          exportName={env.report}
          height={320}
          emptyText={t("reports.panel.empty")}
        />
      ) : null}
    </div>
  );
}
