"use client";

import { useQuery } from "@tanstack/react-query";

import { StatsCard } from "@/components/common/stats-card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  serviceWizardKeys,
  serviceWizardService,
} from "@/features/services/services/service-wizard.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-1075: summary cards above the service list, after the legacy hub's
 * ServiceOverviewStatsWidget: total, opened this month, in progress
 * (pending + processing + ready), completed and active warranties. Same
 * scope as the list; `organizationUuid` follows the center's own / network
 * switch. Hidden on error (the list still works).
 */
export function ServiceListStatsCards({
  enabled,
  organizationUuid,
}: {
  enabled: boolean;
  organizationUuid: string | null;
}) {
  const { t, format } = useLocale();
  const stats = useQuery({
    queryKey: serviceWizardKeys.listStats(organizationUuid),
    queryFn: () => serviceWizardService.listStats(organizationUuid),
    enabled,
    staleTime: 60_000,
  });
  if (!enabled || stats.isError) return null;
  if (!stats.data) {
    return (
      <div
        className="grid grid-cols-2 gap-3 lg:grid-cols-5"
        data-testid="service-list-stats-loading"
      >
        {Array.from({ length: 5 }, (_, i) => (
          <Skeleton key={i} className="h-[104px] rounded-xl" />
        ))}
      </div>
    );
  }
  const s = stats.data;
  const inProgress =
    s.by_status.pending + s.by_status.processing + s.by_status.ready;
  const n = (v: number) => format.number(v);
  return (
    <div
      className="grid grid-cols-2 gap-3 lg:grid-cols-5"
      data-testid="service-list-stats"
    >
      <StatsCard
        title={t("services.list.stats.total")}
        value={n(s.total)}
        hint={t("services.list.stats.total_hint", {
          cancelled: n(s.by_status.cancelled),
        })}
      />
      <StatsCard
        title={t("services.list.stats.this_month")}
        value={n(s.this_month)}
        hint={t("services.list.stats.this_month_hint")}
      />
      <StatsCard
        title={t("services.list.stats.in_progress")}
        value={n(inProgress)}
        hint={t("services.list.stats.in_progress_hint", {
          draft: n(s.by_status.draft),
        })}
      />
      <StatsCard
        title={t("services.list.stats.completed")}
        value={n(s.by_status.completed)}
      />
      <StatsCard
        title={t("services.list.stats.active_warranties")}
        value={n(s.active_warranties)}
        hint={t("services.list.stats.warranties_hint", {
          total: n(s.warranties),
        })}
      />
    </div>
  );
}
