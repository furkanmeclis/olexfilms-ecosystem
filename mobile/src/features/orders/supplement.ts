/**
 * Supplementary orders on mobile (TEC-1222, revize 5.9): a shipped or
 * delivered order gets a follow-up order (plexi, sign sets) with the same
 * dealer and warehouse. The cart carries the main order while the user
 * picks products. Pure, no `@/` imports: node --test runs this file.
 */

/** The main order the cart is a supplement of. */
export type SupplementParent = {
  uuid: string;
  order_no: string;
  buyer: { uuid: string; name: string };
  warehouse_uuid: string | null;
};

type OrderLike = {
  uuid: string;
  order_no: string;
  buyer: { uuid: string; name: string };
  seller_warehouse?: { uuid: string } | null;
  can_add_supplement?: boolean;
};

/** The cart target of "Add supplementary shipment"; null when the server does not allow one. */
export function supplementParentOf(o: OrderLike | null | undefined): SupplementParent | null {
  if (!o?.can_add_supplement) return null;
  return {
    uuid: o.uuid,
    order_no: o.order_no,
    buyer: { uuid: o.buyer.uuid, name: o.buyer.name },
    warehouse_uuid: o.seller_warehouse?.uuid ?? null,
  };
}

/** The new order screen offers it where the cart and that screen exist (the workplace) to a user who orders. */
export function canStartSupplement(o: OrderLike | null | undefined, input: { pathname: string; canOrder: boolean }): boolean {
  return !!supplementParentOf(o) && input.canOrder && input.pathname.startsWith('/bayi');
}

/** List card subtitle: the order number, the supplement mark and the warehouse. */
export function orderSubtitle(
  o: { order_no: string; is_supplement?: boolean },
  labels: { supplement: string; warehouse?: string | null },
): string {
  return [o.order_no, o.is_supplement ? labels.supplement : null, labels.warehouse ?? null].filter(Boolean).join(' · ');
}
