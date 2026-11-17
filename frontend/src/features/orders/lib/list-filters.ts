import type {
  OrderInvoiceStatus,
  OrderStatus,
} from "@/features/orders/services/orders.service";

/** Manual invoice statuses offered in the filter (TEC-1030). */
export const ORDER_INVOICE_STATUSES = [
  "pending",
  "partial",
  "invoiced",
] as const satisfies readonly OrderInvoiceStatus[];

/**
 * Statuses offered in the filter, in flow order. delivered is the seller's
 * hand / depot delivery step before the buyer receives (TEC-1024).
 * processing is not offered as a target but migrated orders sit in it
 * (TEC-261), so it can be filtered.
 */
export const ORDER_FILTER_STATUSES = [
  "draft",
  "submitted",
  "approved",
  "preparing",
  "ready",
  "processing",
  "shipped",
  "delivered",
  "received",
  "cancelling",
  "cancelled",
] as const satisfies readonly OrderStatus[];

/** TEC-1228: order priorities (the priority filter). */
export const ORDER_PRIORITIES = ["normal", "urgent"] as const;

/** TEC-1228: statuses of a warehouse job; a list of only these sorts as the queue. */
export const ORDER_QUEUE_STATUSES: readonly string[] = [
  "approved",
  "processing",
  "preparing",
  "ready",
];

/** Whether a status filter is a work queue (only open warehouse statuses). */
export function isWorkQueueFilter(statuses: readonly string[]): boolean {
  return (
    statuses.length > 0 &&
    statuses.every((s) => ORDER_QUEUE_STATUSES.includes(s))
  );
}
