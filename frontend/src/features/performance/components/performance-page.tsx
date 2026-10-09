"use client";

import { Gauge } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
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
import { FeatureDisabled } from "@/features/modules/components/feature-guard";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import { BonusRulesTab } from "@/features/performance/components/bonus-rules-tab";
import { BonusesTab } from "@/features/performance/components/bonuses-tab";
import { DashboardPanel } from "@/features/performance/components/dashboard-panel";
import { PositionCard } from "@/features/performance/components/position-card";
import { RankingTable } from "@/features/performance/components/ranking-table";
import { RegionMap } from "@/features/performance/components/region-map";
import { ReportWidgets } from "@/features/performance/components/report-widgets";
import { TargetsTab } from "@/features/performance/components/targets-tab";
import { TeamTargetsTab } from "@/features/performance/components/team-targets-tab";
import { WeakRulesTab } from "@/features/performance/components/weak-rules-tab";
import { recentMonths } from "@/features/performance/lib/performance";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type PerformanceTab =
  | "panel"
  | "ranking"
  | "map"
  | "widgets"
  | "targets"
  | "rules"
  | "team"
  | "bonus_rules"
  | "bonuses";

/** Module of the bonus tabs (QUESTIONS S20: no bonus without it). */
export const BONUS_FEATURE = "dealer_accounting";

/** Tabs a role sees: dealers get their position card, no map. */
export function performanceTabs(orgType: string | undefined): PerformanceTab[] {
  if (orgType === "center" || orgType === "distributor") {
    return ["panel", "ranking", "map", "widgets"];
  }
  return ["panel", "ranking", "widgets"];
}

/**
 * Management tabs (TEC-497): network targets and weak dealer rules for a
 * center / distributor; team targets, bonus rules and bonuses for a
 * dealer, each behind its permission.
 */
export function managementTabs(
  orgType: string | undefined,
  can: (permission: string) => boolean,
): PerformanceTab[] {
  if (orgType === "center" || orgType === "distributor") {
    return [
      "targets",
      ...(can(Permission.PerformanceRulesManage) ? ["rules" as const] : []),
    ];
  }
  if (orgType !== "dealer") return [];
  return [
    ...(can(Permission.PerformanceStaffTargetsManage) ? ["team" as const] : []),
    ...(can(Permission.PerformanceBonusManage)
      ? (["bonus_rules", "bonuses"] as const)
      : []),
  ];
}

/** Bonus tabs need dealer_accounting; otherwise the "module off" card. */
function BonusFeatureGate({
  slug,
  children,
}: {
  slug: string;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const enabled = useEnabledFeatures(slug);
  if (enabled == null) return <Loading label={t("common.loading")} />;
  if (!enabled.includes(BONUS_FEATURE)) {
    return <FeatureDisabled feature={BONUS_FEATURE} />;
  }
  return <>{children}</>;
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
  const tabs = [
    ...performanceTabs(org?.type),
    ...managementTabs(org?.type, (p) => can(p)),
  ];
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
        {tabs.includes("targets") ? (
          <TabsContent value="targets" className="mt-4">
            {activeTab === "targets" ? (
              <TargetsTab
                orgType={org?.type ?? ""}
                orgUuid={org?.uuid ?? ""}
                period={period}
              />
            ) : null}
          </TabsContent>
        ) : null}
        {tabs.includes("rules") ? (
          <TabsContent value="rules" className="mt-4">
            {activeTab === "rules" ? (
              <WeakRulesTab orgType={org?.type ?? ""} />
            ) : null}
          </TabsContent>
        ) : null}
        {tabs.includes("team") ? (
          <TabsContent value="team" className="mt-4">
            {activeTab === "team" ? <TeamTargetsTab period={period} /> : null}
          </TabsContent>
        ) : null}
        {tabs.includes("bonus_rules") ? (
          <TabsContent value="bonus_rules" className="mt-4">
            {activeTab === "bonus_rules" ? (
              <BonusFeatureGate slug={slug}>
                <BonusRulesTab />
              </BonusFeatureGate>
            ) : null}
          </TabsContent>
        ) : null}
        {tabs.includes("bonuses") ? (
          <TabsContent value="bonuses" className="mt-4">
            {activeTab === "bonuses" ? (
              <BonusFeatureGate slug={slug}>
                <BonusesTab period={period} />
              </BonusFeatureGate>
            ) : null}
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
