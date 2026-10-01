"use client";

import { useMemo } from "react";

import type { OrganizationSummary } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";

/** Membership of the signed-in user for the tenant route slug, if any. */
export function useActiveOrganization(
  slug: string | null | undefined,
): OrganizationSummary | null {
  const { user } = useAuth();
  return useMemo(() => {
    if (!slug) return null;
    return user?.organizations.find((org) => org.slug === slug) ?? null;
  }, [slug, user?.organizations]);
}
