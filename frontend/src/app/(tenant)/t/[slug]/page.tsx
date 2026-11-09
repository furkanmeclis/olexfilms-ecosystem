"use client";

import { useParams } from "next/navigation";

import { PageHeader } from "@/components/layout";
import { RecentAnnouncementsWidget } from "@/features/announcements";
import { NetworkOccupancyWidget } from "@/features/appointments";
import { CenterOverviewSection } from "@/features/center-overview";
import { HomeOverviewWidget } from "@/features/services";
import { StockSummaryWidget } from "@/features/stock";
import { StockForecastWidget } from "@/features/stock-forecast";
import { useLocale } from "@/providers/locale-provider";

export default function TenantHomePage() {
  const { t } = useLocale();
  const params = useParams<{ slug: string }>();
  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t("dashboard.tenant.welcome_title")}
        description={t("dashboard.tenant.welcome_description")}
      />
      {/* TEC-1072: center overview (cards, top vehicles / products, latest
          services) under one period; renders nothing outside the center. */}
      <CenterOverviewSection slug={params.slug} />
      <div className="grid gap-6 xl:grid-cols-2">
        <HomeOverviewWidget slug={params.slug} />
        <RecentAnnouncementsWidget slug={params.slug} />
        <StockSummaryWidget slug={params.slug} />
        <StockForecastWidget slug={params.slug} />
        <NetworkOccupancyWidget slug={params.slug} />
      </div>
    </div>
  );
}
