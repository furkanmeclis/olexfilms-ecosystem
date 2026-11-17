/**
 * "Sales staff" on mobile (TEC-1068, revize 5.8). The system has no role
 * with that name: the center's sales people are center accounts that sell to
 * the dealers, i.e. they create orders (`orders.write`) for the dealer
 * network they can read (`organizations.read`). Today that is center_staff
 * and center_owner; the service point (center_service), the warehouse and
 * accounting have no `orders.write`. The decision rests on permissions, not
 * on the role name, and the platform admin keeps its own bar (its
 * permissions list is not the source of truth). Pure, no `@/` imports, so
 * node --test runs it.
 */
export const SALES_PERMISSIONS = ['orders.write', 'organizations.read'] as const;

export function isSalesProfile(input: { orgType: string | null | undefined; permissions: readonly string[] }): boolean {
  if (input.orgType !== 'center') return false;
  return SALES_PERMISSIONS.every((p) => input.permissions.includes(p));
}

/**
 * Tab slots of the dealer/center workplace bar (`bayi`). Sales staff get
 * Orders in the bar (TEC-1068). Measuring sits in the bar for everyone who
 * can create a service, sales staff included (TEC-1222: center_owner and
 * center_staff measure from the bar; center_sales has no `services.write`
 * so its bar is unchanged). Without `services.write` Stock takes that slot.
 */
export function workplaceTabs(input: {
  sales: boolean;
  can: (permission: string) => boolean;
}): { orders: boolean; measure: boolean; stock: boolean } {
  const writes = input.can('services.write');
  return {
    orders: input.sales,
    measure: writes,
    stock: !writes && input.can('stock.read'),
  };
}
