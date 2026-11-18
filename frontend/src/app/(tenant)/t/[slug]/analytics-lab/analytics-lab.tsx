"use client";

import { useMemo, useState, type ReactNode } from "react";
import { toast } from "@/lib/toast";

import {
  BarChart,
  CATEGORICAL_SLOTS,
  DataGrid,
  DonutChart,
  Heatmap,
  KpiGrid,
  KpiTile,
  PeriodBar,
  RankBar,
  SEQUENTIAL_STEPS,
  Sparkline,
  StatusBadge,
  TrendChart,
  categoricalColor,
  categoriesFromSeries,
  divergingColor,
  envelopeGranularity,
  gridColumnsFromReport,
  gridRowsFromReport,
  kpisToTiles,
  reportExportPath,
  sequentialColor,
  statusColor,
  timeSeriesFromReport,
  usePeriodState,
  type ChartSeriesDef,
} from "@/components/analytics";
import { StatsCard } from "@/components/common/stats-card";
import { Checkbox } from "@/components/ui/checkbox";
import { useAuth } from "@/providers/auth-provider";
import { useFormatter, useLocale } from "@/providers/locale-provider";

import {
  PRODUCT_SERIES,
  productMonths,
  sampleEnvelope,
  sampleHeatmap,
} from "./sample-data";

function Section({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children: ReactNode;
}) {
  return (
    <section
      aria-labelledby={`lab-${id}`}
      className="flex flex-col gap-3"
      data-lab={id}
    >
      <h2 id={`lab-${id}`} className="text-base font-semibold">
        {title}
      </h2>
      {children}
    </section>
  );
}

function Swatch({ color, label }: { color: string; label?: string }) {
  return (
    <span className="flex flex-col items-center gap-1">
      <span
        className="block h-8 w-10 rounded-md border"
        style={{ background: color }}
        aria-hidden
      />
      {label ? (
        <span className="text-muted-foreground text-[10px]">{label}</span>
      ) : null}
    </span>
  );
}

export function AnalyticsLab({ slug }: { slug: string }) {
  const { t } = useLocale();
  const fmt = useFormatter();
  const { user } = useAuth();
  const allowed =
    process.env.NODE_ENV !== "production" || Boolean(user?.isSuperAdmin);
  const [period, setPeriod, query] = usePeriodState();
  const env = useMemo(() => sampleEnvelope(), []);
  const trend = useMemo(() => timeSeriesFromReport(env.series), [env]);
  const kpis = useMemo(() => kpisToTiles(env.kpis, slug), [env, slug]);
  const columns = useMemo(() => gridColumnsFromReport(env.columns), [env]);
  const rows = useMemo(() => gridRowsFromReport(env.rows), [env]);
  const months = useMemo(() => productMonths(), []);
  const heat = useMemo(() => sampleHeatmap(), []);
  const [visible, setVisible] = useState<string[]>(
    PRODUCT_SERIES.map((s) => s.key),
  );
  const shownSeries: ChartSeriesDef[] = PRODUCT_SERIES.filter((s) =>
    visible.includes(s.key),
  );
  const ranking = useMemo(
    () =>
      categoriesFromSeries({
        key: "dealers",
        label: "",
        points: rows.slice(0, 14).map((r) => ({
          key: String(r.dealer),
          label: String(r.dealer),
          value: Number(r.services),
        })),
      }),
    [rows],
  );
  const donut = PRODUCT_SERIES.slice(0, 6).map((s, i) => ({
    key: s.key,
    label: s.label,
    value: Number(months[11][s.key]) + i,
  }));

  if (!allowed) {
    return (
      <div
        className="text-muted-foreground p-6 text-sm"
        data-screen="analytics-lab-denied"
      >
        {t("chart.lab.forbidden")}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-8 p-4 md:p-6" data-screen="analytics-lab">
      <header className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold">{t("chart.lab.title")}</h1>
        <p className="text-muted-foreground text-sm">
          {t("chart.lab.description")}
        </p>
      </header>

      <Section id="period" title={t("chart.lab.period")}>
        <PeriodBar value={period} onChange={setPeriod} />
        <code
          className="bg-muted w-fit rounded px-2 py-1 text-xs"
          data-testid="period-query"
        >
          {JSON.stringify(query)}
        </code>
      </Section>

      <Section id="kpi" title={t("chart.lab.kpi")}>
        <KpiGrid items={kpis} />
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <KpiTile label={kpis[0]?.label ?? ""} value={null} loading />
          <KpiTile
            label={kpis[1]?.label ?? ""}
            value={kpis[1]?.value}
            format={kpis[1]?.format}
          />
          <StatsCard
            title={kpis[2]?.label ?? ""}
            value={fmt.number(5.4)}
            deltaPct={-21.7}
            good="down"
            spark={kpis[2]?.spark}
          />
        </div>
      </Section>

      <Section id="palette" title={t("chart.lab.palette")}>
        <div className="flex flex-wrap gap-2">
          {Array.from({ length: CATEGORICAL_SLOTS }, (_, i) => (
            <Swatch key={i} color={categoricalColor(i)} label={String(i + 1)} />
          ))}
          <Swatch color={categoricalColor(-1)} label={t("chart.other")} />
        </div>
        <div className="flex flex-wrap gap-1">
          {Array.from({ length: SEQUENTIAL_STEPS }, (_, i) => (
            <Swatch
              key={i}
              color={sequentialColor(i / (SEQUENTIAL_STEPS - 1))}
            />
          ))}
        </div>
        <div className="flex flex-wrap gap-1">
          {Array.from({ length: 11 }, (_, i) => (
            <Swatch key={i} color={divergingColor(i / 5 - 1)} />
          ))}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {(["good", "warning", "serious", "critical"] as const).map((s) => (
            <Swatch key={s} color={statusColor(s)} label={s} />
          ))}
          {(["done", "progress", "neutral", "problem"] as const).map((tone) => (
            <StatusBadge
              key={tone}
              status={{ label: t(`chart.lab.tone_${tone}`), tone }}
            />
          ))}
        </div>
      </Section>

      <Section id="charts" title={t("chart.lab.charts")}>
        <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
          <TrendChart
            title={t("chart.lab.sample_trend")}
            description={t("chart.lab.sample_trend_hint")}
            data={trend.data}
            series={trend.series}
            granularity={envelopeGranularity(env)}
            showPrevious={trend.hasPrevious}
          />
          <TrendChart
            kind="area"
            title={trend.series[0]?.label}
            data={trend.data}
            series={trend.series.slice(0, 1)}
            granularity={envelopeGranularity(env)}
          />
          <div className="flex flex-col gap-2 xl:col-span-2">
            <div
              className="flex flex-nowrap items-center gap-3 overflow-x-auto text-xs"
              data-testid="series-filter"
            >
              {PRODUCT_SERIES.map((s) => (
                <label
                  key={s.key}
                  className="flex shrink-0 items-center gap-1.5"
                >
                  <Checkbox
                    checked={visible.includes(s.key)}
                    onCheckedChange={(c) =>
                      setVisible((v) =>
                        c ? [...v, s.key] : v.filter((k) => k !== s.key),
                      )
                    }
                  />
                  {s.label}
                </label>
              ))}
            </div>
            <BarChart
              title={t("chart.lab.sample_stacked")}
              description={t("chart.lab.sample_stacked_hint")}
              data={months}
              series={shownSeries}
              granularity="month"
              stacked
            />
          </div>
          <BarChart
            title={t("chart.lab.sample_grouped")}
            data={months.slice(-6)}
            series={PRODUCT_SERIES.slice(0, 3)}
            granularity="month"
          />
          <RankBar title={t("chart.lab.sample_rank")} items={ranking} />
          <DonutChart title={t("chart.lab.sample_donut")} items={donut} />
          <Heatmap title={t("chart.lab.sample_heatmap")} cells={heat} />
        </div>
        <div className="flex items-center gap-4">
          <Sparkline
            values={trend.data.map((d) => Number(d.services))}
            label={trend.series[0]?.label}
          />
          <Sparkline
            values={[3, 4, null, 6, 5, 8, 7]}
            color={categoricalColor(1)}
          />
        </div>
      </Section>

      <Section id="states" title={t("chart.lab.states")}>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
          <TrendChart
            title={t("chart.loading")}
            data={[]}
            series={[]}
            loading
            height={160}
          />
          <TrendChart
            title={t("chart.empty_title")}
            data={[]}
            series={trend.series}
            height={160}
          />
          <BarChart
            title={t("chart.error_title")}
            data={[]}
            series={[]}
            error={new Error("HTTP 503")}
            onRetry={() => toast.message(t("chart.retry"))}
            height={160}
          />
        </div>
      </Section>

      <Section id="grid" title={t("chart.lab.grid")}>
        <DataGrid
          caption={t("chart.lab.grid")}
          columns={columns}
          rows={rows}
          slug={slug}
          persistKey="analytics-lab.dealers"
          exportName="analytics-lab"
          defaultSorting={[{ id: "revenue", desc: true }]}
          onServerExport={(format) => {
            toast.message(reportExportPath(env.report, query, format));
          }}
        />
      </Section>
    </div>
  );
}
