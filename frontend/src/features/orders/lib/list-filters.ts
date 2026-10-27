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
