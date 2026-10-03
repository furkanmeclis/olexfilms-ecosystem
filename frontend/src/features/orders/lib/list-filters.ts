import type {
  OrderListQuery,
  OrderSide,
  OrderStatus,
} from "@/features/orders/services/orders.service";
import {
  dayEndIso,
  dayStartIso,
  invalidDateRange,
} from "@/features/services/lib/list-filters";

export {
  invalidDateRange,
  pageCount,
} from "@/features/services/lib/list-filters";

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

export const ALL_STATUSES = "all";

/** Filter bar of the order list (TEC-170). Days are `YYYY-MM-DD`. */
export type OrderListFilters = {
  status: OrderStatus | typeof ALL_STATUSES;
  from: string;
  to: string;
};

export const EMPTY_ORDER_FILTERS: OrderListFilters = {
  status: ALL_STATUSES,
  from: "",
  to: "",
};

/**
 * Builds GET /v1/orders params: "incoming" is side=seller (orders placed to
 * the organization), "outgoing" side=buyer. Empty filters are left out, the
 * days become local-day ISO bounds and a reversed range drops the dates.
 */
export function buildOrderListQuery(
  side: OrderSide,
  filters: OrderListFilters,
  page: { limit: number; offset: number },
): OrderListQuery {
  const query: OrderListQuery = {
    side,
    limit: page.limit,
    offset: page.offset,
  };
  if (filters.status !== ALL_STATUSES) query.status = filters.status;
  if (!invalidDateRange(filters.from, filters.to)) {
    const from = dayStartIso(filters.from);
    const to = dayEndIso(filters.to);
    if (from) query.created_from = from;
    if (to) query.created_to = to;
  }
  return query;
}

export function hasActiveFilters(filters: OrderListFilters): boolean {
  return (
    filters.status !== ALL_STATUSES || filters.from !== "" || filters.to !== ""
  );
}
