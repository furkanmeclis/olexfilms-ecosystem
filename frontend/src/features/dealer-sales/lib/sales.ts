import type {
  SaleLookup,
  ProductSaleListItem,
} from "@/features/dealer-sales/services/dealer-sales.service";

/** Money as the backend accepts it: up to 16 digits, 2 decimals. */
export const MONEY_PATTERN = /^\d{1,16}(\.\d{1,2})?$/;

/** Decimal string → integer cents (null when not a money value). */
export function toCents(value: string | null | undefined): number | null {
  if (value === null || value === undefined) return null;
  const v = value.trim().replace(",", ".");
  if (!/^-?\d+(\.\d+)?$/.test(v)) return null;
  return Math.round(Number(v) * 100);
}

export function fromCents(cents: number): string {
  const sign = cents < 0 ? "-" : "";
  const abs = Math.abs(cents);
  return `${sign}${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, "0")}`;
}

/** Normalizes a typed price ("1.200,5" is not accepted; "12,5" is). */
export function normalizeMoney(value: string): string | null {
  const v = value.trim().replace(",", ".");
  if (!MONEY_PATTERN.test(v)) return null;
  return fromCents(toCents(v) ?? 0);
}

/** The sale price is under the purchase price (selling at a loss). */
export function isBelowCost(
  salePrice: string | null | undefined,
  purchasePrice: string | null | undefined,
): boolean {
  const sale = toCents(salePrice);
  const cost = toCents(purchasePrice);
  return sale !== null && cost !== null && sale < cost;
}

/** Sale price minus purchase price; null when either is unknown. */
export function profitOf(
  salePrice: string | null | undefined,
  purchasePrice: string | null | undefined,
): string | null {
  const sale = toCents(salePrice);
  const cost = toCents(purchasePrice);
  if (sale === null || cost === null) return null;
  return fromCents(sale - cost);
}

/** One line of the quick sale cart (one scanned barcode). */
export type CartLine = {
  barcode: string;
  productUuid: string;
  name: string;
  sku: string;
  unitKind: string;
  /** Pieces the dealer holds under this barcode (fixed barcodes). */
  available: number;
  quantity: number;
  unitPrice: string;
  purchasePrice: string | null;
  currency: string;
};

export function lineFromLookup(item: SaleLookup): CartLine {
  return {
    barcode: item.barcode,
    productUuid: item.product_uuid,
    name: item.name,
    sku: item.sku,
    unitKind: item.unit_kind,
    available: Math.max(1, item.quantity_on_hand),
    quantity: 1,
    unitPrice: item.sale_price ?? item.recommended_sale_price ?? "",
    purchasePrice: item.purchase_price ?? null,
    currency: item.currency,
  };
}

/** Whether a barcode already has a line (a barcode is added only once). */
export function hasBarcode(lines: CartLine[], barcode: string): boolean {
  const key = barcode.trim();
  return lines.some((l) => l.barcode === key);
}

/** Adds a line unless its barcode is already in the cart. */
export function addLine(
  lines: CartLine[],
  line: CartLine,
): { lines: CartLine[]; added: boolean } {
  if (hasBarcode(lines, line.barcode)) return { lines, added: false };
  return { lines: [...lines, line], added: true };
}

export type CartTotals = {
  total: string;
  /** Profit of the lines with a known purchase price. */
  profit: string | null;
  /** Some lines have no purchase price (profit is partial). */
  profitIncomplete: boolean;
  /** Some lines have no valid price yet. */
  invalid: boolean;
};

export function cartTotals(lines: CartLine[]): CartTotals {
  let total = 0;
  let profit = 0;
  let withCost = 0;
  let invalid = false;
  for (const line of lines) {
    const price = MONEY_PATTERN.test(line.unitPrice.trim())
      ? toCents(line.unitPrice)
      : null;
    if (price === null || line.quantity < 1) {
      invalid = true;
      continue;
    }
    total += price * line.quantity;
    const cost = toCents(line.purchasePrice);
    if (cost !== null) {
      profit += (price - cost) * line.quantity;
      withCost += 1;
    }
  }
  return {
    total: fromCents(total),
    profit: withCost > 0 ? fromCents(profit) : null,
    profitIncomplete: withCost > 0 && withCost < lines.length,
    invalid,
  };
}

/** Same calendar day in the viewer's time zone. */
export function isSameLocalDay(iso: string, now: Date = new Date()) {
  const d = new Date(iso);
  return (
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate()
  );
}

/** A sale may be voided only on its sale day and only once (TEC-344). */
export function canVoidSale(
  sale: Pick<ProductSaleListItem, "sold_at" | "voided">,
  now: Date = new Date(),
) {
  return !sale.voided && isSameLocalDay(sale.sold_at, now);
}
