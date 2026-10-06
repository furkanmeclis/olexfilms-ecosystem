import type { ServerListQuery } from "@/components/entity";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Order = Schemas["Order"];
export type OrderStatus = Schemas["OrderStatus"];
export type OrderItem = Schemas["OrderItem"];
export type OrderHistoryEntry = Schemas["OrderHistoryEntry"];
export type OrderItemInput = Schemas["OrderItemInput"];
export type OrderCreateInput = Schemas["OrderCreateInput"];
export type OrderUnitAssignInput = Schemas["OrderUnitAssignInput"];
export type OrderRateSnapshot = Schemas["OrderRateSnapshot"];

/** Side of the active organization in the list: sales or purchases. */
export type OrderSide = "seller" | "buyer";

/**
 * GET /v1/orders params (TEC-170, TEC-373): `side` plus the list contract
 * (`sort` order_no/status/total/created_at, `q`, CSV `status`,
 * `seller_org_uuid` / `buyer_org_uuid`, `total_min` / `total_max`,
 * `created_from` / `created_to`).
 */
export type OrderListQuery = ServerListQuery & { side: OrderSide };

/** POST /v1/orders/export (TEC-373): csv, xlsx or pdf of the list. */
export const ORDERS_EXPORT_PATH = "/v1/orders/export";

/** An organization in the caller's organizations.read scope. */
export type ScopeOrganization = { uuid: string; name: string };

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

const enc = encodeURIComponent;

/**
 * Orders API (TEC-165..169) through the BFF with the active organization.
 * Writes go through platformRequest, so a STEP_UP_REQUIRED answer opens the
 * step-up dialog and retries (withStepUpRetry).
 */
export const ordersService = {
  list(params: OrderListQuery) {
    return platformRequest<Page<Order>>("GET", "/v1/orders", {
      query: params,
    });
  },
  get(uuid: string) {
    return platformRequest<Order>("GET", `/v1/orders/${enc(uuid)}`);
  },
  /** Party filter options: organizations in the organizations.read scope. */
  async listOrganizations(): Promise<ScopeOrganization[]> {
    const data = await platformRequest<{ items: ScopeOrganization[] }>(
      "GET",
      "/v1/tenant/organizations",
      { query: { limit: 100 } },
    );
    return (data.items ?? []).map((o) => ({ uuid: o.uuid, name: o.name }));
  },
  create(body: OrderCreateInput) {
    return platformRequest<Order>("POST", "/v1/orders", { body });
  },
  replaceItems(uuid: string, items: OrderItemInput[]) {
    return platformRequest<Order>("PUT", `/v1/orders/${enc(uuid)}/items`, {
      body: { items },
    });
  },
  transition(uuid: string, status: OrderStatus, reason?: string) {
    return platformRequest<Order>(
      "POST",
      `/v1/orders/${enc(uuid)}/transitions`,
      { body: reason ? { status, reason } : { status } },
    );
  },
  assignUnit(uuid: string, item: string, body: OrderUnitAssignInput) {
    return platformRequest<Order>(
      "POST",
      `/v1/orders/${enc(uuid)}/items/${enc(item)}/units`,
      { body },
    );
  },
  unassignUnit(uuid: string, item: string, unit: string) {
    return platformRequest<Order>(
      "DELETE",
      `/v1/orders/${enc(uuid)}/items/${enc(item)}/units/${enc(unit)}`,
    );
  },
};

export const orderKeys = {
  all: ["orders"] as const,
  list: (params: OrderListQuery) => ["orders", "list", params] as const,
  detail: (uuid: string) => ["orders", "detail", uuid] as const,
  organizations: ["orders", "organizations"] as const,
  products: (q: string) => ["orders", "products", q] as const,
  price: (uuid: string) => ["orders", "price", uuid] as const,
};
