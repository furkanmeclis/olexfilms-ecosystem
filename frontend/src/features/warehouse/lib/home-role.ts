import { Permission } from "@/config/permissions";

/**
 * TEC-1066: a warehouse account (center_warehouse, distributor warehouse
 * staff) opens the warehouse home instead of the general tenant home. The
 * rule follows permissions, not role names (same idea as the mobile app's
 * "warehouse first" space, mobile/src/core/spaces.ts): a center or
 * distributor member who reads the warehouse but neither opens services nor
 * reads accounting. A combined account (center owner, warehouse + accounting)
 * keeps the general home.
 */
export function isWarehouseHomeRole(
  can: (permission: string) => boolean,
  orgType: string | null | undefined,
): boolean {
  if (orgType !== "center" && orgType !== "distributor") return false;
  return (
    can(Permission.WarehouseRead) &&
    !can(Permission.ServicesWrite) &&
    !can(Permission.AccountingRead)
  );
}
