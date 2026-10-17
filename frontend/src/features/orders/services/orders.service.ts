import type { ServerListQuery } from "@/components/entity";
import type { components } from "@/generated/api";
import {
  platformDownloadFile,
  platformFormRequest,
} from "@/lib/api/platform-form-request";
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
export type OrderSuggestions = Schemas["OrderSuggestions"];
export type OrderSuggestion = Schemas["OrderSuggestion"];
export type OrderPickLocation = Schemas["OrderPickLocation"];
export type OrderPickList = Schemas["OrderPickList"];
export type OrderPickRow = Schemas["OrderPickRow"];
export type DeliveryMethod = Schemas["DeliveryMethod"];
export type DeliveryMethodInput = Schemas["DeliveryMethodInput"];
export type OrderDocumentKind = "shipping" | "receipt";

/** Shipping info sent with the ship transition (TEC-768). */
export type OrderShippingInput = {
  delivery_method_uuid?: string;
  tracking_no?: string;
};

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
  transition(
    uuid: string,
    status: OrderStatus,
    reason?: string,
    shipping?: OrderShippingInput,
  ) {
    return platformRequest<Order>(
      "POST",
      `/v1/orders/${enc(uuid)}/transitions`,
      { body: { status, ...(reason ? { reason } : {}), ...shipping } },
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
  /** TEC-768: units proposed for the rest of a preparing line. */
  suggest(uuid: string, item: string, includeUnlocated: boolean) {
    return platformRequest<OrderSuggestions>(
      "GET",
      `/v1/orders/${enc(uuid)}/items/${enc(item)}/suggestions`,
      { query: includeUnlocated ? { include_unlocated: "true" } : {} },
    );
  },
  /** Assigns the confirmed suggestions in one transaction. */
  applySuggestions(uuid: string, item: string, rows: OrderSuggestion[]) {
    const units = rows.map((s) => ({
      unit_uuid: s.unit_uuid,
      ...(s.meters ? { meters: s.meters } : {}),
      ...(s.quantity != null && !s.meters ? { quantity: s.quantity } : {}),
    }));
    return platformRequest<Order>(
      "POST",
      `/v1/orders/${enc(uuid)}/items/${enc(item)}/suggestions/apply`,
      { body: { units } },
    );
  },
  async pickLocations(uuid: string, item: string) {
    const data = await platformRequest<{ items: OrderPickLocation[] }>(
      "GET",
      `/v1/orders/${enc(uuid)}/items/${enc(item)}/pick-locations`,
    );
    return data.items ?? [];
  },
  pickList(uuid: string) {
    return platformRequest<OrderPickList>(
      "GET",
      `/v1/orders/${enc(uuid)}/pick-list`,
    );
  },
  regenerateShippingDocument(uuid: string) {
    return platformRequest<Order>(
      "POST",
      `/v1/orders/${enc(uuid)}/shipping-document`,
    );
  },
  downloadOrderSlip(uuid: string, lang: string, tz: string) {
    const query = new URLSearchParams({ lang, tz });
    return platformDownloadFile(
      `/v1/orders/${enc(uuid)}/order-slip.pdf?${query}`,
    );
  },
  downloadDocument(uuid: string, kind: OrderDocumentKind) {
    return platformDownloadFile(`/v1/orders/${enc(uuid)}/documents/${kind}`);
  },
  attachReceipt(uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return platformFormRequest<Order>(
      "POST",
      `/v1/orders/${enc(uuid)}/receipt-document`,
      form,
    );
  },
};

/** TEC-768: delivery method catalog of the active organization. */
export const deliveryMethodsService = {
  list(params: ServerListQuery) {
    return platformRequest<Page<DeliveryMethod>>(
      "GET",
      "/v1/delivery-methods",
      { query: params },
    );
  },
  /** Active methods for the ship dialog (catalog order). */
  async listActive(): Promise<DeliveryMethod[]> {
    const data = await platformRequest<Page<DeliveryMethod>>(
      "GET",
      "/v1/delivery-methods",
      { query: { active: "true", sort: "sort_order", limit: 100 } },
    );
    return data.items ?? [];
  },
  create(body: DeliveryMethodInput) {
    return platformRequest<DeliveryMethod>("POST", "/v1/delivery-methods", {
      body,
    });
  },
  update(uuid: string, body: DeliveryMethodInput) {
    return platformRequest<DeliveryMethod>(
      "PUT",
      `/v1/delivery-methods/${enc(uuid)}`,
      { body },
    );
  },
  remove(uuid: string) {
    return platformRequest<void>("DELETE", `/v1/delivery-methods/${enc(uuid)}`);
  },
};

export const orderKeys = {
  all: ["orders"] as const,
  list: (params: OrderListQuery) => ["orders", "list", params] as const,
  detail: (uuid: string) => ["orders", "detail", uuid] as const,
  organizations: ["orders", "organizations"] as const,
  products: (q: string) => ["orders", "products", q] as const,
  price: (uuid: string) => ["orders", "price", uuid] as const,
  suggestions: (uuid: string, item: string, unlocated: boolean) =>
    ["orders", "suggestions", uuid, item, unlocated] as const,
  pickLocations: (uuid: string, item: string) =>
    ["orders", "pick-locations", uuid, item] as const,
  pickList: (uuid: string) => ["orders", "pick-list", uuid] as const,
  deliveryMethods: (params: ServerListQuery) =>
    ["orders", "delivery-methods", params] as const,
  activeDeliveryMethods: ["orders", "delivery-methods", "active"] as const,
};
