"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { catalogKeys } from "@/features/catalog/hooks/use-catalog-access";
import { catalogService } from "@/features/catalog/services/catalog.service";

/** Categories of the brand as select options (first 100, API maximum). */
export function useCategoryOptions(enabled: boolean) {
  const params = { limit: 100, offset: 0 };
  const query = useQuery({
    queryKey: catalogKeys.categories(params),
    queryFn: () => catalogService.listCategories(params),
    enabled,
    staleTime: 60_000,
  });
  const options = useMemo(
    () =>
      (query.data?.items ?? []).map((c) => ({
        value: c.uuid,
        label: c.name,
        category: c,
      })),
    [query.data?.items],
  );
  return { options, isLoading: query.isLoading };
}
