"use client";

import { useMemo } from "react";

import { shellNav, type ShellNav, type ShellVariant } from "@/config/shell-nav";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import type { NavAccess } from "@/features/nav-engine/lib/access";
import type { NavOrgContext } from "@/features/nav-engine/types";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { usePermission } from "@/providers/permission-provider";

export type ShellNavState = ShellNav & {
  /** Active organization for the tenant shell (type, role, modules). */
  org: NavOrgContext | null;
  access: NavAccess;
};

export function useShellNav(
  variant: ShellVariant,
  tenantSlug?: string,
): ShellNavState {
  const slug = variant === "tenant" ? tenantSlug : null;
  const nav = useMemo(
    () => shellNav(variant, tenantSlug),
    [variant, tenantSlug],
  );
  const activeOrg = useActiveOrganization(slug);
  const features = useEnabledFeatures(slug);
  const org = useMemo<NavOrgContext | null>(
    () =>
      activeOrg
        ? { type: activeOrg.type, role: activeOrg.role, features }
        : null,
    [activeOrg, features],
  );
  const { can, canAny } = usePermission();
  const access = useMemo<NavAccess>(
    () => ({ can, canAny, org }),
    [can, canAny, org],
  );

  return { ...nav, org, access };
}
