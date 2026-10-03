import { permissions } from "@/config/permissions";
import type {
  Order,
  OrderItem,
  OrderSide,
  OrderStatus,
} from "@/features/orders/services/orders.service";
import type { OrganizationType } from "@/lib/auth/types";

type Can = (permission: string) => boolean;

export type OrderListAccess = {
  /** List and detail open (GET /v1/orders needs orders.read). */
  canRead: boolean;
  /**
   * "New order": orders.write and an organization that has a supplier
   * (distributor -> center, dealer -> distributor; K6) plus catalog.read
   * for the product search. The center buys from nobody.
   */
  canCreate: boolean;
  /** Tabs: incoming (sales, side=seller) and outgoing (purchases). */
  sides: OrderSide[];
};

/**
 * Tabs by organization type: the center only sells, a dealer only buys, a
 * distributor does both. Unknown type (no membership info) shows both.
 */
export function resolveOrderListAccess(
  can: Can,
  orgType: OrganizationType | null | undefined,
): OrderListAccess {
  const sides: OrderSide[] =
    orgType === "center"
      ? ["seller"]
      : orgType === "dealer"
        ? ["buyer"]
        : ["seller", "buyer"];
  return {
    canRead: can(permissions.orders.read),
    canCreate:
      can(permissions.orders.write) &&
      can(permissions.catalog.read) &&
      orgType !== "center",
    sides,
  };
}

export type OrderActionKind =
  | "submit"
  | "approve"
  | "reject"
  | "start_preparing"
  | "mark_ready"
  | "ship"
  | "receive"
  | "cancel"
  | "request_cancel"
  | "complete_cancel";

export type OrderAction = {
  kind: OrderActionKind;
  target: OrderStatus;
  /** Destructive styling and a reason field. */
  destructive: boolean;
  /** A reason must be written before confirming. */
  reasonRequired: boolean;
};

/**
 * Targets the UI never offers: processing is the legacy warehouse step
 * (the UI uses preparing; a processing order still moves on to preparing or
 * cancelled, TEC-261) and delivered is not used (TEC-168: received follows
 * shipped).
 */
const HIDDEN_TARGETS = new Set<OrderStatus>(["processing", "delivered"]);

function actionFor(order: Order, target: OrderStatus): OrderAction | null {
  const seller = order.role === "seller";
  switch (target) {
    case "submitted":
      return {
        kind: "submit",
        target,
        destructive: false,
        reasonRequired: false,
      };
    case "approved":
      return {
        kind: "approve",
        target,
        destructive: false,
        reasonRequired: false,
      };
    case "preparing":
      return {
        kind: "start_preparing",
        target,
        destructive: false,
        reasonRequired: false,
      };
    case "ready":
      return {
        kind: "mark_ready",
        target,
        destructive: false,
        reasonRequired: false,
      };
    case "shipped":
      return {
        kind: "ship",
        target,
        destructive: false,
        reasonRequired: false,
      };
    case "received":
      return {
        kind: "receive",
        target,
        destructive: false,
        reasonRequired: false,
      };
    case "cancelling":
      return {
        kind: "request_cancel",
        target,
        destructive: true,
        reasonRequired: true,
      };
    case "cancelled":
      if (order.status === "cancelling") {
        return {
          kind: "complete_cancel",
          target,
          destructive: true,
          reasonRequired: false,
        };
      }
      // The seller turning down a submitted order is a rejection.
      if (seller && order.status === "submitted") {
        return {
          kind: "reject",
          target,
          destructive: true,
          reasonRequired: true,
        };
      }
      return {
        kind: "cancel",
        target,
        destructive: true,
        reasonRequired: true,
      };
    default:
      return null;
  }
}

/**
 * Action buttons of the detail page. The server lists what the caller's
 * side and permissions allow (available_transitions, the same state
 * machine the API enforces); an observer (read scope only) gets none.
 */
export function orderActions(order: Order): OrderAction[] {
  if (order.role === "observer") return [];
  const out: OrderAction[] = [];
  for (const target of order.available_transitions ?? []) {
    if (HIDDEN_TARGETS.has(target)) continue;
    const action = actionFor(order, target);
    if (action) out.push(action);
  }
  return out;
}

/** Draft lines can be edited by the buyer with orders.write. */
export function canEditDraft(can: Can, order: Order): boolean {
  return (
    order.role === "buyer" &&
    order.status === "draft" &&
    can(permissions.orders.write)
  );
}

/** The seller assigns units (barcode scan) while the order is preparing. */
export function canAssignUnits(can: Can, order: Order): boolean {
  return (
    order.role === "seller" &&
    order.status === "preparing" &&
    can(permissions.orders.ship)
  );
}

/** Ordered amount of a line: pieces, or meters for roll lines. */
export function lineAmount(item: OrderItem): number {
  if (item.meters !== null && item.meters !== undefined) {
    return Number(item.meters);
  }
  return item.quantity ?? 0;
}

/** The assigned units cover the whole line. */
export function lineFullyAssigned(item: OrderItem): boolean {
  const assigned = Number(item.assigned);
  return Number.isFinite(assigned) && assigned >= lineAmount(item) - 1e-9;
}

/** Every line is covered: preparing -> ready is allowed by the API. */
export function orderFullyAssigned(order: Order): boolean {
  const items = order.items ?? [];
  return items.length > 0 && items.every(lineFullyAssigned);
}
