"use client";

import { Gauge } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { DashboardPanel } from "@/features/performance/components/dashboard-panel";
import { PositionCard } from "@/features/performance/components/position-card";
import { RankingTable } from "@/features/performance/components/ranking-table";
import { RegionMap } from "@/features/performance/components/region-map";
import { ReportWidgets } from "@/features/performance/components/report-widgets";
import { recentMonths } from "@/features/performance/lib/performance";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type PerformanceTab = "panel" | "ranking" | "map" | "widgets";

/** Tabs a role sees: dealers get their position card, no map. */
export function performanceTabs(orgType: string | undefined): PerformanceTab[] {
  if (orgType === "center" || orgType === "distributor") {
    return ["panel", "ranking", "map", "widgets"];
  }
  return ["panel", "ranking", "widgets"];
}

/**
 * Tenant > Performance (TEC-496, performance module + performance.read):
 * KPI panel with the 12-month trend, ranking (center / distributor) or
 * "my position in the network" (dealer), region map (center /
 * distributor) and the corporate report widgets; one month picker drives
 * every tab.
 */
export function PerformancePage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const canRead = can(Permission.PerformanceRead);
  const months = useMemo(() => recentMonths(new Date(), 12), []);
  const [period, setPeriod] = useState(months[0]);
  const tabs = performanceTabs(org?.type);
  const [tab, setTab] = useState<PerformanceTab>("panel");
  const activeTab = tabs.includes(tab) ? tab : "panel";
  const isDealer = org?.type !== "center" && org?.type !== "distributor";

  const title = t("performance.page.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Gauge className="size-6" />}
      description={t("performance.page.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        canRead ? (
          <Select value={period} onValueChange={setPeriod}>
            <SelectTrigger
              className="w-44"
              aria-label={t("performance.period")}
              data-testid="performance-period"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {months.map((m) => (
                <SelectItem key={m} value={m}>
                  {format.dateParts(`${m}-01T12:00:00`, {
                    month: "long",
                    year: "numeric",
                  })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null
      }
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("performance.page.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <Tabs
        value={activeTab}
        onValueChange={(v) => setTab(v as PerformanceTab)}
      >
        <TabsList className="flex-wrap">
          {tabs.map((value) => (
            <TabsTrigger
              key={value}
              value={value}
              data-testid={`performance-tab-${value}`}
            >
              {value === "ranking" && isDealer
                ? t("performance.tabs.position")
                : t(`performance.tabs.${value}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="panel" className="mt-4">
          {activeTab === "panel" ? (
            <DashboardPanel slug={slug} period={period} />
          ) : null}
        </TabsContent>
        <TabsContent value="ranking" className="mt-4">
          {activeTab === "ranking" ? (
            isDealer ? (
              <PositionCard slug={slug} period={period} />
            ) : (
              <RankingTable orgType={org?.type ?? ""} period={period} />
            )
          ) : null}
        </TabsContent>
        {tabs.includes("map") ? (
          <TabsContent value="map" className="mt-4">
            {activeTab === "map" ? (
              <RegionMap slug={slug} period={period} />
            ) : null}
          </TabsContent>
        ) : null}
        <TabsContent value="widgets" className="mt-4">
          {activeTab === "widgets" ? <ReportWidgets /> : null}
        </TabsContent>
      </Tabs>
    </div>
  );
}
