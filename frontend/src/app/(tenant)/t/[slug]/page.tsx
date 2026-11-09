"use client";

import { useParams } from "next/navigation";

import { PageHeader } from "@/components/layout";
import {
  AccountingDashboard,
  isAccountingHomeRole,
} from "@/features/accounting";
import { RecentAnnouncementsWidget } from "@/features/announcements";
import { NetworkOccupancyWidget } from "@/features/appointments";
import { CenterOverviewSection } from "@/features/center-overview";
import { HomeOverviewWidget } from "@/features/services";
import { StockSummaryWidget } from "@/features/stock";
import { StockForecastWidget } from "@/features/stock-forecast";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export default function TenantHomePage() {
  const { t } = useLocale();
  const params = useParams<{ slug: string }>();
  const { can } = usePermission();
  const org = useActiveOrganization(params.slug);
  // TEC-1070: accounting accounts land on the accounting home.
  if (isAccountingHomeRole(can, org?.type)) {
    return <AccountingDashboard slug={params.slug} />;
  }
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
