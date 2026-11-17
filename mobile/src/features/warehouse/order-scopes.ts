/**
 * Warehouse order list filters (TEC-1224). Pure, no `@/` imports, so node --test runs it.
 */

/** Warehouse view of orders: what to prepare (seller) and what to receive and shelve (buyer). */
export type OrderScope = 'prepare' | 'receive' | 'all';

/**
 * Statuses of each scope. Receive covers shipped, delivered (at the door,
 * not taken in yet) and received (waiting to be shelved).
 */
export const ORDER_SCOPES: Record<OrderScope, { side?: 'seller' | 'buyer'; status?: string }> = {
  prepare: { side: 'seller', status: 'approved,processing,preparing,ready' },
  receive: { side: 'buyer', status: 'shipped,delivered,received' },
  all: {},
};

/**
 * The /v1/orders query of a scope. The prepare side follows the picked
 * warehouse (`seller_warehouse`); the API has no buyer warehouse filter
 * (an incoming order gets its bin only when it is shelved), so the receive
 * side lists the organization's. Every warehouse list and counter of the
 * prepare side (the list, the home counters and the pending badge) builds
 * its query here so they never drift apart.
 */
export function orderScopeQuery(
  scope: OrderScope,
  warehouse?: string,
  status?: string,
): { side?: 'seller' | 'buyer'; status?: string; seller_warehouse?: string } {
  const base = ORDER_SCOPES[scope];
  const q: { side?: 'seller' | 'buyer'; status?: string; seller_warehouse?: string } = { ...base };
  if (status !== undefined) q.status = status;
  if (scope === 'prepare' && warehouse) q.seller_warehouse = warehouse;
  return q;
}
