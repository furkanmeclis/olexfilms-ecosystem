import type {
  EffectivePrice,
  ListPriceInput,
  PriceViewer,
} from "@/features/catalog/services/pricing.service";

export type PriceField =
  "purchase_price" | "sale_price" | "recommended_sale_price";

/**
 * Columns a viewer type may ever see (K8, TEC-146 EffectivePrice):
 * center: own purchase, sale to distributors, recommended retail;
 * distributor: its purchase (override or list) and its price to dealers;
 * dealer: its purchase only.
 */
const VIEWER_FIELDS: Record<PriceViewer, PriceField[]> = {
  center: ["purchase_price", "sale_price", "recommended_sale_price"],
  distributor: ["purchase_price", "sale_price"],
  dealer: ["purchase_price"],
};

export type PriceColumn = {
  field: PriceField;
  /** i18n key of the column header. */
  labelKey: string;
};

const LABEL_KEYS: Record<PriceViewer, Partial<Record<PriceField, string>>> = {
  center: {
    purchase_price: "catalog.prices.center_purchase",
    sale_price: "catalog.prices.center_sale",
    recommended_sale_price: "catalog.prices.recommended",
  },
  distributor: {
    purchase_price: "catalog.prices.my_purchase",
    sale_price: "catalog.prices.distributor_sale",
  },
  dealer: {
    purchase_price: "catalog.prices.my_purchase",
  },
};

/**
 * Visible price columns. The API leaves out every field the caller may not
 * see, so a column is shown only when the viewer type allows it AND at least
 * one currency row carries the field; a dealer never gets a sale column even
 * if a field slipped through.
 */
export function visiblePriceColumns(
  viewer: PriceViewer,
  prices: readonly EffectivePrice[],
): PriceColumn[] {
  return VIEWER_FIELDS[viewer]
    .filter((field) => prices.some((row) => field in row))
    .map((field) => ({ field, labelKey: LABEL_KEYS[viewer][field] ?? "" }));
}

/** Parses a PriceAmount string ("1250.50") for display; null when absent. */
export function priceNumber(value: string | undefined | null): number | null {
  if (value === undefined || value === null || value === "") return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

/** PriceAmount pattern of the API (NUMERIC(14,4), non-negative). */
export const PRICE_AMOUNT_RE = /^[0-9]{1,10}(\.[0-9]{1,4})?$/;
export const CURRENCY_RE = /^[A-Z]{3}$/;

/** Trims the input and turns a decimal comma ("12,5") into a dot. */
export function normalizePriceInput(value: string): string {
  return value.trim().replace(",", ".");
}

/**
 * ListPriceInput from the dialog: a filled field is sent, an emptied field
 * that had a value is sent as null (clear), and a field the caller never saw
 * (masked) is left out so it keeps its stored value.
 */
export function listPriceBody(
  values: Record<string, string | undefined>,
  original: EffectivePrice | null,
  canWriteRecommended: boolean,
): ListPriceInput {
  const pick = (value: string | undefined, before: string | undefined) => {
    if (value) return value;
    return before !== undefined ? null : undefined;
  };
  const body: ListPriceInput = {};
  const purchase = pick(values.purchase_price, original?.purchase_price);
  if (purchase !== undefined) body.purchase_price = purchase;
  const sale = pick(values.sale_to_distributor_price, original?.sale_price);
  if (sale !== undefined) body.sale_to_distributor_price = sale;
  if (canWriteRecommended) {
    const rec = pick(
      values.recommended_sale_price,
      original?.recommended_sale_price,
    );
    if (rec !== undefined) body.recommended_sale_price = rec;
  }
  return body;
}
