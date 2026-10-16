import type {
  StockForecastDraftLine,
  StockForecastRow,
  StockForecastStatus,
  StockForecastThresholdInput,
  StockForecastThresholds,
} from "@/features/stock-forecast/services/stock-forecast.service";

export const STOCK_FORECAST_STATUSES: StockForecastStatus[] = [
  "critical",
  "warning",
  "ok",
  "no_consumption",
  "insufficient_data",
];

export function stockForecastStatusTone(status: StockForecastStatus) {
  if (status === "critical") return "danger" as const;
  if (status === "warning") return "warning" as const;
  if (status === "ok") return "success" as const;
  return "default" as const;
}

export function isSelectableForecast(row: StockForecastRow) {
  return row.status !== "insufficient_data";
}

/** Draft item of a row; an empty amount lets the backend use the suggestion. */
export function forecastDraftLine(
  row: StockForecastRow,
): StockForecastDraftLine {
  if (row.product.unit_type === "roll_meter") {
    return { product_uuid: row.product.uuid, meters: row.suggested_meters };
  }
  return { product_uuid: row.product.uuid, quantity: row.suggested_qty };
}

/** Drops empty amounts so the backend falls back to the suggestion. */
export function cleanDraftLine(
  line: StockForecastDraftLine,
): StockForecastDraftLine {
  const out: StockForecastDraftLine = { product_uuid: line.product_uuid };
  if (line.quantity != null && line.quantity > 0) out.quantity = line.quantity;
  if (line.meters != null && line.meters.trim() !== "") {
    out.meters = line.meters.trim();
  }
  return out;
}

export function missingDataDays(row: StockForecastRow, minDataDays: number) {
  return Math.max(0, minDataDays - row.data_days);
}

export type EffectiveThreshold = { warning_days: number; cover_days: number };

/** The product override, else the organization default. */
export function effectiveThreshold(
  thresholds: StockForecastThresholds | undefined,
  productUuid: string,
): EffectiveThreshold | null {
  if (!thresholds) return null;
  const override = thresholds.overrides?.find(
    (o) => o.product?.uuid === productUuid,
  );
  return {
    warning_days: override?.warning_days ?? thresholds.warning_days,
    cover_days: override?.cover_days ?? thresholds.cover_days,
  };
}

/**
 * PUT /v1/stock-forecasts/thresholds body that changes one product's cover
 * days: the organization defaults stay, the product override is upserted.
 */
export function productThresholdInput(
  thresholds: StockForecastThresholds,
  productUuid: string,
  coverDays: number,
): StockForecastThresholdInput {
  const current = effectiveThreshold(thresholds, productUuid)!;
  return {
    warning_days: thresholds.warning_days,
    cover_days: thresholds.cover_days,
    products: [
      {
        product_uuid: productUuid,
        warning_days: current.warning_days,
        cover_days: coverDays,
      },
    ],
  };
}
