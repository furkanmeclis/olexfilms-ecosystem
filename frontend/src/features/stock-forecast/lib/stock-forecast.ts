import type {
  StockForecastDraftLine,
  StockForecastRow,
  StockForecastStatus,
} from "@/features/stock-forecast/services/stock-forecast.service";

export const STOCK_FORECAST_STATUSES: StockForecastStatus[] = [
  "critical",
  "warning",
  "healthy",
  "insufficient_data",
];

export function stockForecastStatusTone(status: StockForecastStatus) {
  if (status === "critical") return "danger" as const;
  if (status === "warning") return "warning" as const;
  if (status === "healthy") return "success" as const;
  return "default" as const;
}

export function isSelectableForecast(row: StockForecastRow) {
  return row.status !== "insufficient_data";
}

export function forecastDraftLine(
  row: StockForecastRow,
): StockForecastDraftLine {
  return {
    product_uuid: row.product.uuid,
    quantity: row.suggested_qty ?? null,
    meters: row.suggested_meters == null ? null : Number(row.suggested_meters),
  };
}

export function missingDataDays(row: StockForecastRow) {
  return Math.max(0, row.min_data_days - row.data_days);
}
