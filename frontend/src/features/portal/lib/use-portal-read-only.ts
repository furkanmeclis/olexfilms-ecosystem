"use client";

import { useQuery } from "@tanstack/react-query";

import { portalApi } from "@/features/portal/lib/portal-client";
import {
  isPortalReadOnly,
  PORTAL_ME_KEY,
} from "@/features/portal/lib/portal-vehicles";

/**
 * Whether the signed-in portal session is a read-only fleet account
 * (TEC-245). False while loading or on error: the API still refuses the
 * writes with 403 PORTAL_READ_ONLY, the UI only hides them.
 */
export function usePortalReadOnly(): boolean {
  const me = useQuery({
    queryKey: PORTAL_ME_KEY,
    queryFn: () => portalApi.me(),
    staleTime: 5 * 60_000,
    retry: false,
  });
  return isPortalReadOnly(me.data?.roles);
}
