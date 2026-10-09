"use client";

import { useQuery } from "@tanstack/react-query";

import { DEFAULT_DEVIATION_THRESHOLD } from "@/features/pricing/lib/recommended";
import {
  pricingKeys,
  recommendedService,
} from "@/features/pricing/services/recommended.service";

/**
 * pricing.deviation_warning_pct as the price screens read it (TEC-507);
 * the catalog default until it loads or when the read fails.
 */
export function useDeviationThreshold(enabled: boolean) {
  const query = useQuery({
    queryKey: pricingKeys.settings,
    queryFn: () => recommendedService.settings(),
    enabled,
    staleTime: 5 * 60 * 1000,
  });
  return query.data?.deviation_warning_pct ?? DEFAULT_DEVIATION_THRESHOLD;
}
