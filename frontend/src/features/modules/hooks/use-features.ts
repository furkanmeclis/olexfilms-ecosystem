"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { modulesService } from "@/features/modules/services/modules.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";

export const modulesKeys = {
  all: ["modules"] as const,
  features: (orgUuid: string) => ["modules", "features", orgUuid] as const,
  dealers: (orgUuid: string) => ["modules", "dealers", orgUuid] as const,
  standard: (orgUuid: string) => ["modules", "standard", orgUuid] as const,
  platform: ["modules", "platform"] as const,
  requests: (scope: string) => ["modules", "requests", scope] as const,
  platformOrg: (uuid: string) => ["modules", "platform", "org", uuid] as const,
};

/** Same window as the backend Redis cache. */
export const FEATURES_STALE_MS = 30_000;

/**
 * Modules of the tenant route's organization. The query key carries the
 * organization, so switching organizations refetches.
 */
export function useFeatures(slug: string | null | undefined) {
  const org = useActiveOrganization(slug);
  const orgUuid = org?.uuid ?? "";
  return useQuery({
    queryKey: modulesKeys.features(orgUuid),
    queryFn: () => modulesService.list(),
    enabled: Boolean(orgUuid),
    staleTime: FEATURES_STALE_MS,
  });
}

/** Enabled module keys, or null while unknown (nav entries stay hidden). */
export function useEnabledFeatures(slug: string | null | undefined) {
  const { data } = useFeatures(slug);
  return useMemo(() => data?.enabled ?? null, [data]);
}

/** Whether one module is on for the tenant route's organization. */
export function useFeature(slug: string | null | undefined, key: string) {
  const query = useFeatures(slug);
  return {
    enabled: Boolean(query.data?.enabled.includes(key)),
    isLoading: query.isLoading,
    isError: query.isError,
  };
}
