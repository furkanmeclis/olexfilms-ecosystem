"use client";

import { PRICE_VIEW_PERMISSIONS } from "@/config/permissions";
import { usePermission } from "@/providers/permission-provider";

/** TEC-1050: whether a permission check grants any price permission. */
export function canSeePrices(can: (permission: string) => boolean): boolean {
  return PRICE_VIEW_PERMISSIONS.some((slug) => can(slug));
}

/**
 * TEC-1050: whether the user may see prices, amounts, totals and rates
 * (any of `PRICE_VIEW_PERMISSIONS`, any scope). Screens hide their money
 * columns and fields when false (warehouse roles).
 */
export function useCanSeePrices(): boolean {
  const { can } = usePermission();
  return canSeePrices(can);
}
