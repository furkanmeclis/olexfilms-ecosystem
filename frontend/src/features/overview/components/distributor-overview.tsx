"use client";

import { Network } from "lucide-react";

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
import { useLocale } from "@/providers/locale-provider";

const TOP = { limit: 10 } as const;

/**
 * TEC-1212: distributor overview. network.summary (KPIs, services /
 * orders / warranties of the subtree against the previous period, dealers
 * ranked) and the subtree rankings.
 */
export function DistributorOverview({ slug }: { slug: string }) {
  const { t } = useLocale();
  return (
    <OverviewShell
      testId="distributor-overview"
      title={t("dashboard.role.distributor.title")}
      description={t("dashboard.role.distributor.description")}
      icon={<Network className="size-6" />}
    >
      {(query) => (
        <>
          <ReportKpiStrip slug={slug} reportKey="network.summary" query={query} />
          <ReportTrendPanel
            slug={slug}
            reportKey="network.summary"
            query={query}
            title={t("dashboard.role.network_trend")}
          />
          <ReportTablePanel
            slug={slug}
            reportKey="network.summary"
            query={query}
            title={t("dashboard.role.network_dealers")}
          />
          <OverviewSection
            title={t("dashboard.role.dealers_section")}
            testId="distributor-dealers"
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
          <RecentAnnouncementsWidget slug={slug} />
        </>
      )}
    </OverviewShell>
  );
}
