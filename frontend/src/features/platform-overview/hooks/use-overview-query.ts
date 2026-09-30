"use client";

import { useQuery } from "@tanstack/react-query";

import { OVERVIEW_STALE_MS } from "@/features/platform-overview/constants";
import { overviewKeys } from "@/features/platform-overview/hooks/query-keys";
import {
  overviewService,
  type OverviewFetchScopes,
} from "@/features/platform-overview/services/overview.service";

export function useOverviewStats(scopes: OverviewFetchScopes, enabled = true) {
  const anyScope = Boolean(
    scopes.roles || scopes.users || scopes.notifications,
  );

  return useQuery({
    queryKey: overviewKeys.stats(scopes),
    queryFn: () => overviewService.getStats(scopes),
    enabled: enabled && anyScope,
    staleTime: OVERVIEW_STALE_MS,
  });
}
