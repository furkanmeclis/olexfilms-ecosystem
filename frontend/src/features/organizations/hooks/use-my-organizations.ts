"use client";

import { useQuery } from "@tanstack/react-query";

import { organizationsKeys } from "@/features/organizations/hooks/query-keys";
import { organizationsService } from "@/features/organizations/services/organizations.service";

/** Caller's memberships within the domain's brand (GET /v1/me/organizations). */
export function useMyOrganizations(enabled = true) {
  return useQuery({
    queryKey: organizationsKeys.mine(),
    queryFn: () => organizationsService.listMine(),
    enabled,
    staleTime: 60_000,
  });
}
