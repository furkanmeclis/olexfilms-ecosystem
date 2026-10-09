"use client";

import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { usePermission } from "@/providers/permission-provider";

/**
 * Dealer sales controls for the tenant route's organization: a dealer with
 * the accounting and dealer_accounting modules (same gates as the API).
 * `orgUuid` keys every query so switching organizations never shows
 * another book.
 */
export function useDealerSalesAccess(slug: string) {
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const features = useEnabledFeatures(slug);
  const orgUuid = org?.uuid ?? "";
  return useMemo(() => {
    const enabled =
      org?.type === "dealer" &&
      Boolean(features?.includes("accounting")) &&
      Boolean(features?.includes("dealer_accounting"));
    return {
      orgUuid,
      enabled,
      canPrices: enabled && can(permissions.dealerSales.pricingWrite),
      canSales: enabled && can(permissions.dealerSales.salesWrite),
      canSuppliers: enabled && can(permissions.dealerSales.suppliersManage),
      canPurchases: enabled && can(permissions.dealerSales.purchasesWrite),
      canSeePurchasePrice: can(permissions.pricing.purchaseRead),
      // TEC-507: recommended price column + deviation badge.
      canSeeRecommended: can(permissions.pricing.recommendedRead),
    };
  }, [can, features, org?.type, orgUuid]);
}
