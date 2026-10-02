"use client";

import { useMemo } from "react";

import { resolveAccountingAccess } from "@/features/accounting/lib/access";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { usePermission } from "@/providers/permission-provider";

/**
 * Accounting controls for the tenant route's organization. `orgUuid` keys
 * every query so switching organizations never shows another book.
 */
export function useAccountingAccess(slug: string) {
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const features = useEnabledFeatures(slug);
  const orgType = org?.type ?? null;
  const orgUuid = org?.uuid ?? "";
  return useMemo(
    () => ({
      orgUuid,
      orgType,
      ...resolveAccountingAccess({ can, orgType, features }),
    }),
    [can, features, orgType, orgUuid],
  );
}

export const accountingKeys = {
  all: (org: string) => ["accounting", org] as const,
  categories: (org: string) => ["accounting", org, "categories"] as const,
  currencies: ["accounting", "currencies"] as const,
  accounts: (org: string, params: unknown) =>
    ["accounting", org, "accounts", params] as const,
  cariList: (org: string, params: unknown) =>
    ["accounting", org, "cari", params] as const,
  cari: (org: string, uuid: string) =>
    ["accounting", org, "cari-detail", uuid] as const,
  entries: (org: string, params: unknown) =>
    ["accounting", org, "entries", params] as const,
};
