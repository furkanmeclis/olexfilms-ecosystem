import type { ServerListQuery } from "@/components/entity";
import type { components } from "@/generated/api";
import { apiPath } from "@/lib/api/api-path";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type StockForecastStatus = Schemas["StockForecastStatus"];
export type StockForecastProduct = Schemas["StockForecastProductRef"];
export type StockForecastRow = Schemas["StockForecast"];
export type StockForecastDetail = Schemas["StockForecastDetail"];
export type StockForecastThresholds = Schemas["StockForecastThresholds"];
export type StockForecastThresholdInput =
  Schemas["StockForecastThresholdInput"];
export type StockForecastParameters = Schemas["StockForecastParameters"];
export type StockForecastMeta = Schemas["StockForecastMeta"];
export type StockForecastOrderDraftInput =
  Schemas["StockForecastOrderDraftInput"];
export type StockForecastDraftLine =
  StockForecastOrderDraftInput["items"][number];
export type StockForecastOrderDraft = Schemas["StockForecastOrderDraft"];
export type NetworkDemandRow = Schemas["StockForecastNetworkDemand"];
export type SubtreeSummaryRow = Schemas["StockForecastSubtreeSummary"];

export type StockForecastListQuery = ServerListQuery & {
  status?: string;
  days_left_min?: number | string;
  days_left_max?: number | string;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** List export (csv, xlsx): `stock_forecasts.list`. */
export const STOCK_FORECAST_EXPORT_PATH = apiPath(
  "post",
  "/v1/stock-forecasts/export",
);
/** Center network demand export (xlsx): `stock_forecasts.network`. */
export const STOCK_FORECAST_NETWORK_EXPORT_PATH = apiPath(
  "post",
  "/v1/stock-forecasts/network/export",
);

export const stockForecastService = {
  list(params: StockForecastListQuery) {
    return platformRequest<Page<StockForecastRow>>(
      "GET",
      apiPath("get", "/v1/stock-forecasts"),
      { query: params },
    );
  },
  meta() {
    return platformRequest<StockForecastMeta>(
      "GET",
      apiPath("get", "/v1/stock-forecasts/meta"),
    );
  },
  detail(productUuid: string) {
    return platformRequest<StockForecastDetail>(
      "GET",
      apiPath("get", "/v1/stock-forecasts/{product_uuid}", {
        product_uuid: productUuid,
      }),
    );
  },
  /** Organization defaults plus the product overrides to upsert. */
  putThresholds(input: StockForecastThresholdInput) {
    return platformRequest<StockForecastThresholds>(
      "PUT",
      apiPath("put", "/v1/stock-forecasts/thresholds"),
      { body: input },
    );
  },
  createOrderDraft(input: StockForecastOrderDraftInput) {
    return platformRequest<StockForecastOrderDraft>(
      "POST",
      apiPath("post", "/v1/stock-forecasts/order-draft"),
      { body: input },
    );
  },
  network(params: ServerListQuery) {
    return platformRequest<Page<NetworkDemandRow>>(
      "GET",
      apiPath("get", "/v1/stock-forecasts/network"),
      { query: params },
    );
  },
  subtree(params: ServerListQuery) {
    return platformRequest<Page<SubtreeSummaryRow>>(
      "GET",
      apiPath("get", "/v1/stock-forecasts/subtree"),
      { query: params },
    );
  },
  /** Home widget: number of critical products (the list total). */
  async criticalCount() {
    const page = await stockForecastService.list({
      status: "critical",
      limit: 1,
      offset: 0,
    });
    return page.total;
  },
};

export const stockForecastKeys = {
  all: ["stock-forecast"] as const,
  list: (params: StockForecastListQuery) =>
    ["stock-forecast", "list", params] as const,
  meta: ["stock-forecast", "meta"] as const,
  detail: (uuid: string) => ["stock-forecast", "detail", uuid] as const,
  network: (params: ServerListQuery) =>
    ["stock-forecast", "network", params] as const,
  subtree: (params: ServerListQuery) =>
    ["stock-forecast", "subtree", params] as const,
  widget: ["stock-forecast", "widget"] as const,
};
