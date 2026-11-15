"use client";

import { PageHeader } from "@/components/layout";
import { RecentAnnouncementsWidget } from "@/features/announcements";
import { NetworkOccupancyWidget } from "@/features/appointments";
import { CenterOverviewSection } from "@/features/center-overview";
import { HomeOverviewWidget } from "@/features/services";
import { StockSummaryWidget } from "@/features/stock";
import { StockForecastWidget } from "@/features/stock-forecast";
import { useLocale } from "@/providers/locale-provider";

/** TEC-1212: distributor overview. */
export function DistributorOverview({ slug }: { slug: string }) {
  const { t } = useLocale();
  return (
    <div className="flex flex-col gap-6" data-testid="distributor-overview">
      <PageHeader
        title={t("dashboard.tenant.welcome_title")}
        description={t("dashboard.tenant.welcome_description")}
      />
      <CenterOverviewSection slug={slug} />
      <div className="grid gap-6 xl:grid-cols-2">
        <HomeOverviewWidget slug={slug} />
        <RecentAnnouncementsWidget slug={slug} />
        <StockSummaryWidget slug={slug} />
        <StockForecastWidget slug={slug} />
        <NetworkOccupancyWidget slug={slug} />
      </div>
    </div>
  );
}
