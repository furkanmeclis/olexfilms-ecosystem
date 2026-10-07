"use client";

import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import { useEnabledFeatures } from "@/features/modules/hooks/use-features";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { usePermission } from "@/providers/permission-provider";

type Can = (permission: string) => boolean;

export type StaffAccess = {
  /** staff.manage on a writable book: staff cards open and edit. */
  canManage: boolean;
  /** staff_payments.write on a writable book: payments and payroll. */
  canPay: boolean;
  /** accounting.read with the accounting module: the report page. */
  canReadReports: boolean;
  /** pricing.purchase.read: margin cost columns carry values. */
  canSeeCost: boolean;
};

/**
 * Mirrors the API gates (TEC-345/346): the accounting module everywhere and,
 * in a dealer, the dealer_accounting module for staff writes.
 */
export function resolveStaffAccess({
  can,
  orgType,
  features,
}: {
  can: Can;
  orgType?: string | null;
  features?: readonly string[] | null;
}): StaffAccess {
  const accounting = Boolean(features?.includes("accounting"));
  const book =
    accounting &&
    (orgType !== "dealer" || Boolean(features?.includes("dealer_accounting")));
  return {
    canManage: book && can(permissions.staff.manage),
    canPay:
      book &&
      can(permissions.staff.manage) &&
      can(permissions.staff.paymentsWrite),
    canReadReports: accounting && can(permissions.accounting.read),
    canSeeCost: can(permissions.pricing.purchaseRead),
  };
}

/** Staff and report controls for the tenant route's organization. */
export function useStaffAccess(slug: string) {
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const features = useEnabledFeatures(slug);
  const orgType = org?.type ?? null;
  const orgUuid = org?.uuid ?? "";
  const loaded = Boolean(orgUuid) && features != null;
  return useMemo(
    () => ({
      orgUuid,
      loaded,
      ...resolveStaffAccess({ can, orgType, features }),
    }),
    [can, features, loaded, orgType, orgUuid],
  );
}
