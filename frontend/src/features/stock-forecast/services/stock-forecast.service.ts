import type { ServerListQuery } from "@/components/entity";
import { platformRequest } from "@/lib/api/platform-request";

export type StockForecastStatus =
  "critical" | "warning" | "healthy" | "insufficient_data";

export type StockForecastProduct = {
  uuid: string;
  sku: string;
  name: string;
  category?: { uuid: string; name: string } | null;
  unit_type?: "piece" | "roll_meter" | string;
};

export type StockForecastRow = {
  uuid: string;
  product: StockForecastProduct;
  on_hand_qty: number;
  on_hand_meters?: string | number | null;
  avg_daily_30: string | number;
  avg_daily_90: string | number;
  seasonality_factor: string | number;
  days_left?: string | number | null;
  depletion_date?: string | null;
  data_days: number;
  min_data_days: number;
  status: StockForecastStatus;
  suggested_qty?: number | null;
  suggested_meters?: string | number | null;
  warning_days: number;
  cover_days: number;
  vehicles_left?: string | number | null;
};

export type StockForecastListQuery = ServerListQuery & {
  status?: string;
  category_uuid?: string;
  days_left_min?: number | string;
  days_left_max?: number | string;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type StockForecastHistoryPoint = {
  date: string;
  actual_qty?: number | null;
  actual_meters?: number | null;
  projected_qty?: number | null;
  projected_meters?: number | null;
  threshold?: number | null;
  vehicles_left?: number | null;
};

export type StockForecastDetail = {
  forecast: StockForecastRow;
  history: StockForecastHistoryPoint[];
};

export type StockForecastThresholdInput = {
  warning_days: number;
  cover_days: number;
};

export type StockForecastDraftLine = {
  product_uuid: string;
  quantity?: number | null;
  meters?: number | null;
};

export type StockForecastOrderDraft = {
  uuid: string;
  order_no: string;
};

export type NetworkDemandRow = {
  uuid: string;
  product: StockForecastProduct;
  forecast_month: string;
  network_on_hand_qty: number;
  network_on_hand_meters?: string | number | null;
  open_order_qty: number;
  open_order_meters?: string | number | null;
  suggested_production_qty: number;
  suggested_production_meters?: string | number | null;
};

export type DealerSummaryRow = {
  uuid: string;
  dealer_name: string;
  critical_count: number;
  warning_count: number;
  suggested_qty: number;
  suggested_meters?: string | number | null;
};

export type StockForecastWidgetSummary = {
  critical_count: number;
};

export const STOCK_FORECAST_EXPORT_PATH = "/v1/stock-forecasts/export";
export const STOCK_FORECAST_NETWORK_EXPORT_PATH =
  "/v1/stock-forecasts/network-demand/export";

const enc = encodeURIComponent;

export const stockForecastService = {
  list(params: StockForecastListQuery) {
    return platformRequest<Page<StockForecastRow>>(
      "GET",
      "/v1/stock-forecasts",
      {
        query: params,
      },
    );
  },
  detail(productUuid: string) {
    return platformRequest<StockForecastDetail>(
      "GET",
      `/v1/stock-forecasts/${enc(productUuid)}`,
    );
  },
  updateThreshold(productUuid: string, input: StockForecastThresholdInput) {
    return platformRequest<StockForecastRow>(
      "PUT",
      `/v1/stock-forecasts/${enc(productUuid)}/threshold`,
      { body: input },
    );
  },
  createOrderDraft(lines: StockForecastDraftLine[]) {
    return platformRequest<StockForecastOrderDraft>(
      "POST",
      "/v1/stock-forecasts/order-drafts",
      { body: { lines } },
    );
  },
  networkDemand(params: ServerListQuery) {
    return platformRequest<Page<NetworkDemandRow>>(
      "GET",
      "/v1/stock-forecasts/network-demand",
      { query: params },
    );
  },
  dealerSummary() {
    return platformRequest<{ items: DealerSummaryRow[] }>(
      "GET",
      "/v1/stock-forecasts/dealers-summary",
    );
  },
  widget() {
    return platformRequest<StockForecastWidgetSummary>(
      "GET",
      "/v1/stock-forecasts/widget",
    );
  },
};

export const stockForecastKeys = {
  all: ["stock-forecast"] as const,
  list: (params: StockForecastListQuery) =>
    ["stock-forecast", "list", params] as const,
  detail: (uuid: string) => ["stock-forecast", "detail", uuid] as const,
  network: (params: ServerListQuery) =>
    ["stock-forecast", "network", params] as const,
  dealers: ["stock-forecast", "dealers"] as const,
  widget: ["stock-forecast", "widget"] as const,
};
