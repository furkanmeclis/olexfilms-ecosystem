import type { components } from '@/core/api/schema';

type S = components['schemas'];
export type SaleLookup = S['ProductSaleLookup'];
export type ProductSaleListItem = S['ProductSaleListItem'];
export type SalePaymentMethod = S['ProductSaleRequest']['payment_method'];
export type PurchasePaymentMethod = S['PurchaseRequest']['payment_method'];

export const SALE_PAYMENT_METHODS: SalePaymentMethod[] = ['cash', 'card', 'cari'];
export const PURCHASE_PAYMENT_METHODS: PurchasePaymentMethod[] = ['cash', 'card', 'bank_transfer', 'cari'];

/** Money as the backend accepts it: up to 16 digits, 2 decimals (web MONEY_PATTERN). */
export const MONEY_PATTERN = /^\d{1,16}(\.\d{1,2})?$/;

/** Decimal string → integer cents (null when not a money value). */
export function toCents(value: string | null | undefined): number | null {
  if (value === null || value === undefined) return null;
  const v = value.trim().replace(',', '.');
  if (!/^-?\d+(\.\d+)?$/.test(v)) return null;
  return Math.round(Number(v) * 100);
}

export function fromCents(cents: number): string {
  const sign = cents < 0 ? '-' : '';
  const abs = Math.abs(cents);
  return `${sign}${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, '0')}`;
}

/** Normalizes a typed price ("12,5" → "12.50"); null when it is not a money value. */
export function normalizeMoney(value: string): string | null {
  const v = value.trim().replace(',', '.');
  if (!MONEY_PATTERN.test(v)) return null;
  return fromCents(toCents(v) ?? 0);
}

/** The sale price is under the purchase price (selling at a loss). */
export function isBelowCost(salePrice: string | null | undefined, purchasePrice: string | null | undefined): boolean {
  const sale = toCents(salePrice);
  const cost = toCents(purchasePrice);
  return sale !== null && cost !== null && sale < cost;
}

/** Sale price minus purchase price; null when either is unknown. */
export function profitOf(salePrice: string | null | undefined, purchasePrice: string | null | undefined): string | null {
  const sale = toCents(salePrice);
  const cost = toCents(purchasePrice);
  if (sale === null || cost === null) return null;
  return fromCents(sale - cost);
}

/**
 * The unit barcode of a scanned code: unit labels carry `OFW:UNIT:<barcode>`
 * (web unitBarcode); anything else is the barcode itself.
 */
export function unitBarcode(raw: string): string {
  const v = raw.trim();
  return /^OFW:UNIT:/i.test(v) ? v.slice(9).trim() : v;
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
    unitPrice: item.sale_price ?? item.recommended_sale_price ?? '',
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
export function addLine(lines: CartLine[], line: CartLine): { lines: CartLine[]; added: boolean } {
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
    const normalized = normalizeMoney(line.unitPrice);
    const price = normalized === null ? null : toCents(normalized);
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
  return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate();
}

/** A sale may be voided only on its sale day and only once (TEC-344). */
export function canVoidSale(sale: Pick<ProductSaleListItem, 'sold_at' | 'voided'>, now: Date = new Date()) {
  return !sale.voided && isSameLocalDay(sale.sold_at, now);
}

/** Client-side checks of the quick sale before it is sent (web QuickSaleForm.submit). */
export function saleErrors(lines: CartLine[], payment: SalePaymentMethod, customer: string | null) {
  const out: { lines?: 'lines' | 'price'; customer?: 'cari_customer' } = {};
  if (lines.length === 0) out.lines = 'lines';
  else if (cartTotals(lines).invalid) out.lines = 'price';
  if (payment === 'cari' && !customer) out.customer = 'cari_customer';
  return out;
}

/** Client-side checks of a new purchase (web PurchaseForm.submit). */
export function purchaseErrors(supplier: string, amount: string) {
  const out: { supplier?: 'supplier'; amount?: 'amount' } = {};
  if (!supplier) out.supplier = 'supplier';
  const n = normalizeMoney(amount);
  if (n === null || (toCents(n) ?? 0) <= 0) out.amount = 'amount';
  return out;
}
