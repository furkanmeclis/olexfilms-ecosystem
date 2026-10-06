import type { OrderStatus } from "@/features/orders/services/orders.service";

/**
 * Statuses offered in the filter, in flow order. delivered is in the schema
 * but not used (TEC-168: received follows shipped). processing is not
 * offered as a target but migrated orders sit in it (TEC-261), so it can be
 * filtered.
 */
export const ORDER_FILTER_STATUSES = [
  "draft",
  "submitted",
  "approved",
  "preparing",
  "ready",
  "processing",
  "shipped",
  "received",
  "cancelling",
  "cancelled",
] as const satisfies readonly OrderStatus[];
