/** Staff roles a member can be given per organization type (owner roles never). */
export const STAFF_ROLES: Record<string, readonly string[]> = {
  center: [
    "center_warehouse",
    "center_staff",
    "center_accounting",
    "center_social",
    "center_service",
  ],
  distributor: [
    "distributor_staff",
    "distributor_warehouse_staff",
    "distributor_accounting",
  ],
  dealer: ["dealer_staff", "dealer_accounting"],
};

/** Staff roles of an organization type (none for an unknown type). */
export function staffRoles(
  orgType: string | null | undefined,
): readonly string[] {
  return (orgType && STAFF_ROLES[orgType]) || [];
}

/** Roles that work in a warehouse: the form offers the warehouses for them. */
export function isWarehouseRole(slug: string): boolean {
  return slug === "center_warehouse" || slug === "distributor_warehouse_staff";
}
