"use client";

import { useQuery } from "@tanstack/react-query";

import { organizationsKeys } from "@/features/organizations/hooks/query-keys";
import {
  organizationsService,
  type ListOrganizationsParams,
} from "@/features/organizations/services/organizations.service";

export function useOrganizationsList(
  params: ListOrganizationsParams,
  enabled = true,
) {
  return useQuery({
    queryKey: organizationsKeys.list(params),
    queryFn: () => organizationsService.list(params),
    enabled,
    placeholderData: (previous) => previous,
  });
}

export function useOrganization(uuid: string | null, enabled = true) {
  return useQuery({
    queryKey: organizationsKeys.detail(uuid ?? ""),
    queryFn: () => organizationsService.get(uuid!),
    enabled: Boolean(uuid) && enabled,
  });
}

/** Direct sub-organizations of `uuid` (full array, no paging). */
export function useOrganizationChildren(uuid: string | null, enabled = true) {
  return useQuery({
    queryKey: organizationsKeys.children(uuid ?? ""),
    queryFn: () => organizationsService.children(uuid!),
    enabled: Boolean(uuid) && enabled,
  });
}

export function useOrganizationsMeta(enabled = true) {
  return useQuery({
    queryKey: organizationsKeys.meta(),
    queryFn: () => organizationsService.meta(),
    enabled,
    staleTime: 5 * 60_000,
  });
}
