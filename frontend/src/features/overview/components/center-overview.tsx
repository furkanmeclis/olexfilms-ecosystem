"use client";

import { Building2 } from "lucide-react";

import { RecentAnnouncementsWidget } from "@/features/announcements";
import {
  OverviewSection,
  OverviewShell,
} from "@/features/overview/components/overview-shell";
import {
  ReportKpiStrip,
  ReportRankPanel,
  ReportTablePanel,
  ReportTrendPanel,
} from "@/features/reports/components/report-panels";
import { StockForecastWidget } from "@/features/stock-forecast";
import { useLocale } from "@/providers/locale-provider";

const TOP = { limit: 10 } as const;

/**
 * TEC-1212: center overview. KPI strip (delta + sparkline), services and
 * orders against the previous period, the dealer rankings (most orders,
 * no orders, most services), the warehouse / stock summary and the latest
 * activity. Panels the member cannot open drop out; failures show a retry.
 */
export function CenterOverview({ slug }: { slug: string }) {
  const { t } = useLocale();
  return (
    <OverviewShell
      testId="center-overview"
      title={t("dashboard.role.center.title")}
      description={t("dashboard.role.center.description")}
      icon={<Building2 className="size-6" />}
    >
      {(query) => (
        <>
          <ReportKpiStrip slug={slug} reportKey="overview" query={query} />
          <div className="grid gap-6 xl:grid-cols-2">
            <ReportTrendPanel
              slug={slug}
              reportKey="services.trend"
              query={query}
              title={t("dashboard.role.services_trend")}
            />
            <ReportTrendPanel
              slug={slug}
              reportKey="orders.trend"
              query={query}
              title={t("dashboard.role.orders_trend")}
            />
          </div>
          <OverviewSection
            title={t("dashboard.role.dealers_section")}
            testId="center-dealers"
          >
            <div className="grid gap-6 xl:grid-cols-3">
              <ReportRankPanel
                slug={slug}
                reportKey="orders.by_buyer"
                query={query}
                extra={TOP}
                title={t("dashboard.role.top_ordering")}
              />
              <ReportTablePanel
                slug={slug}
                reportKey="orders.inactive_buyers"
                query={query}
                extra={TOP}
                height={300}
                title={t("dashboard.role.no_orders")}
              />
              <ReportRankPanel
                slug={slug}
                reportKey="services.by_org"
                query={query}
                extra={TOP}
                title={t("dashboard.role.top_services")}
              />
            </div>
          </OverviewSection>
          <OverviewSection
            title={t("dashboard.role.stock_section")}
            testId="center-stock"
          >
            <ReportKpiStrip slug={slug} reportKey="stock.summary" query={query} />
            <ReportTrendPanel
              slug={slug}
              reportKey="stock.movement"
              query={query}
              title={t("dashboard.role.stock_movement")}
              kind="area"
            />
            <StockForecastWidget slug={slug} />
          </OverviewSection>
          <div className="grid gap-6 xl:grid-cols-3">
            <ReportTablePanel
              className="xl:col-span-2"
              slug={slug}
              reportKey="activities.recent"
              query={query}
              extra={TOP}
              title={t("dashboard.role.recent_activity")}
            />
            <RecentAnnouncementsWidget slug={slug} />
          </div>
        </>
      )}
    </OverviewShell>
  );
}
