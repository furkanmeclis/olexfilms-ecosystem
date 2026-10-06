"use client";

import { useMemo } from "react";

import {
  resolveCatalogAccess,
  resolvePricingAccess,
} from "@/features/catalog/lib/access";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { usePermission } from "@/providers/permission-provider";

/** Catalog + pricing controls for the tenant route's organization. */
export function useCatalogAccess(slug: string) {
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const orgType = org?.type ?? null;
  return useMemo(
    () => ({
      orgType,
      catalog: resolveCatalogAccess({ can, orgType }),
      pricing: resolvePricingAccess({ can, orgType }),
    }),
    [can, orgType],
  );
}

export const catalogKeys = {
  all: ["catalog"] as const,
  categories: (params: unknown) => ["catalog", "categories", params] as const,
  products: (params: unknown) => ["catalog", "products", params] as const,
  product: (uuid: string) => ["catalog", "product", uuid] as const,
  prices: (uuid: string) => ["catalog", "prices", uuid] as const,
  distributorPrices: (uuid: string, params?: unknown) =>
    params === undefined
      ? (["catalog", "distributor-prices", uuid] as const)
      : (["catalog", "distributor-prices", uuid, params] as const),
  distributors: ["catalog", "distributors"] as const,
};
