"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";

import {
  reportsKeys,
  reportsService,
  type ReportQuery,
} from "@/features/reports/services/reports.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { ApiError } from "@/lib/api/errors";

/**
 * A report the caller cannot open (403: permission, reach or module off;
 * 404: unknown report). An overview drops that panel; any other failure is
 * shown with a retry instead of vanishing (TEC-1212, the TEC-1072 cards).
 */
export function isUnavailable(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 403 || error.status === 404)
  );
}

/** One report envelope for the active organization. */
export function useReport(
  slug: string,
  key: string,
  query: ReportQuery,
  options: { enabled?: boolean } = {},
) {
  const org = useActiveOrganization(slug);
  const orgKey = org?.uuid ?? slug;
  return useQuery({
    queryKey: reportsKeys.report(orgKey, key, query),
    queryFn: () => reportsService.report(key, query),
    enabled: (options.enabled ?? true) && Boolean(key),
    placeholderData: keepPreviousData,
    staleTime: 60_000,
    retry: (count, error) => !isUnavailable(error) && count < 1,
  });
}

/** The report catalog (keys open to the caller). */
export function useReportCatalog(slug: string) {
  const org = useActiveOrganization(slug);
  return useQuery({
    queryKey: reportsKeys.catalog(org?.uuid ?? slug),
    queryFn: () => reportsService.catalog(),
    staleTime: 5 * 60_000,
  });
}
