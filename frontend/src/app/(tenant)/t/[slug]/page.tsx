"use client";

import { useParams } from "next/navigation";

import { PageHeader } from "@/components/layout";
import { RecentAnnouncementsWidget } from "@/features/announcements";
import { StockSummaryWidget } from "@/features/stock";
import { TopVehicleModelsWidget } from "@/features/vehicle-catalog";
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
      <div className="grid gap-6 xl:grid-cols-2">
        <RecentAnnouncementsWidget slug={params.slug} />
        <TopVehicleModelsWidget slug={params.slug} />
        <StockSummaryWidget slug={params.slug} />
      </div>
    </div>
  );
}
