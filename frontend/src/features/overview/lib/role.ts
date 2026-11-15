import { isAccountingHomeRole } from "@/features/accounting/lib/home-role";
import { isWarehouseHomeRole } from "@/features/warehouse/lib/home-role";

/**
 * TEC-1212: which overview a member lands on at /t/[slug]. The rule follows
 * permissions first (TEC-1066 warehouse, TEC-1070 accounting accounts),
 * then the organization type. Combined accounts (center owner) keep the
 * organization overview.
 */
export type OverviewRole =
  "center" | "distributor" | "dealer" | "warehouse" | "accounting";

export function resolveOverviewRole(
  can: (permission: string) => boolean,
  orgType: string | null | undefined,
): OverviewRole | null {
  if (isAccountingHomeRole(can, orgType)) return "accounting";
  if (isWarehouseHomeRole(can, orgType)) return "warehouse";
  switch (orgType) {
    case "center":
      return "center";
    case "distributor":
      return "distributor";
    case "dealer":
      return "dealer";
    default:
      return null;
  }
}
