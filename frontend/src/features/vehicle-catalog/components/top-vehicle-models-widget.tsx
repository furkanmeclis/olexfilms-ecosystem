"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { vehicleCatalogKeys } from "@/features/vehicle-catalog/hooks/use-vehicle-catalog";
import { canSeeTopVehicles } from "@/features/vehicle-catalog/lib/top-vehicles";
import {
  vehicleStatsService,
  type StatsGroup,
  type StatsPeriod,
} from "@/features/vehicle-catalog/services/vehicle-stats.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { usePermission } from "@/providers/permission-provider";

import { TopVehicleModelsChart } from "./top-vehicle-models-chart";

/**
 * TEC-151: top-10 car brands / models widget of the center dashboard.
 * Renders nothing for dealers and distributors (the API answers 403).
 * TEC-1072: with `period` the page owns the period (center home) and the
 * chart shows no period switch of its own.
 */
export function TopVehicleModelsWidget({
  slug,
  period: fixedPeriod,
}: {
  slug: string;
  period?: StatsPeriod;
}) {
  const { scopeFor } = usePermission();
  const org = useActiveOrganization(slug);
  const visible = canSeeTopVehicles(scopeFor, org?.type);
  const [ownPeriod, setPeriod] = useState<StatsPeriod>("30d");
  const period = fixedPeriod ?? ownPeriod;
  const [group, setGroup] = useState<StatsGroup>("model");

  const query = useQuery({
    queryKey: [...vehicleCatalogKeys.all, "top", slug, period, group] as const,
    queryFn: () => vehicleStatsService.topVehicleModels({ period, group }),
    enabled: visible,
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });

  if (!visible) return null;
  return (
    <TopVehicleModelsChart
      items={query.data?.items}
      period={period}
      group={group}
      onPeriodChange={setPeriod}
      onGroupChange={setGroup}
      hidePeriod={fixedPeriod !== undefined}
      isLoading={query.isPending}
      isError={query.isError}
      onRetry={() => void query.refetch()}
    />
  );
}
