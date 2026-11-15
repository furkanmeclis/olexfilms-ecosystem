"use client";

import { Store } from "lucide-react";

import { RecentAnnouncementsWidget } from "@/features/announcements";
import { NetworkOccupancyWidget } from "@/features/appointments";
import { OverviewShell } from "@/features/overview/components/overview-shell";
import {
  ReportDonutPanel,
  ReportKpiStrip,
  ReportRankPanel,
  ReportTablePanel,
  ReportTrendPanel,
} from "@/features/reports/components/report-panels";
import { useLocale } from "@/providers/locale-provider";

const TOP = { limit: 10 } as const;

/**
 * TEC-1212: dealer overview. Services, warranties, measurements and orders
 * over time against the previous period, the most used products and the
 * customer growth.
 */
export function DealerOverview({ slug }: { slug: string }) {
  const { t } = useLocale();
  return (
    <OverviewShell
      testId="dealer-overview"
      title={t("dashboard.role.dealer.title")}
      description={t("dashboard.role.dealer.description")}
      icon={<Store className="size-6" />}
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
              reportKey="warranties.summary"
              query={query}
              title={t("dashboard.role.warranties_trend")}
            />
            <ReportTrendPanel
              slug={slug}
              reportKey="measurements.summary"
              query={query}
              title={t("dashboard.role.measurements_trend")}
            />
            <ReportTrendPanel
              slug={slug}
              reportKey="orders.trend"
              query={query}
              title={t("dashboard.role.orders_trend")}
            />
          </div>
          <div className="grid gap-6 xl:grid-cols-3">
            <ReportRankPanel
              slug={slug}
              reportKey="services.top_products"
              query={query}
              extra={TOP}
              title={t("dashboard.role.top_products")}
            />
            <ReportTrendPanel
              slug={slug}
              reportKey="customers.trend"
              query={query}
              kind="area"
              title={t("dashboard.role.customer_growth")}
            />
            <ReportDonutPanel
              slug={slug}
              reportKey="services.status_distribution"
              query={query}
              title={t("dashboard.role.services_status")}
            />
          </div>
          <div className="grid gap-6 xl:grid-cols-3">
            <ReportTablePanel
              className="xl:col-span-2"
              slug={slug}
              reportKey="activities.recent"
              query={query}
              extra={TOP}
              title={t("dashboard.role.recent_activity")}
            />
            <div className="flex flex-col gap-6">
              <RecentAnnouncementsWidget slug={slug} />
              <NetworkOccupancyWidget slug={slug} />
            </div>
          </div>
        </>
      )}
    </OverviewShell>
  );
}
