import type {
  Order,
  OrderItemInput,
  OrderStatus,
} from "@/features/orders/services/orders.service";

/** API limits of OrderCreateInput / OrderItemInput. */
export const MAX_ORDER_LINES = 200;
export const MAX_QUANTITY = 1_000_000;
export const NOTE_MAX = 1000;
const METERS_RE = /^[0-9]{1,8}(\.[0-9]{1,2})?$/;
const INT_RE = /^[0-9]+$/;

/** One line of the order form; amount is the raw input. */
export type OrderFormLine = {
  product_uuid: string;
  sku: string;
  name: string;
  unit_type: string;
  amount: string;
};

export type LineError = "quantity_invalid" | "meters_invalid";

export type OrderFormErrors = {
  /** Whole-form problem: no line, or more than the API accepts. */
  form?: "empty" | "too_many";
  /** Errors by product_uuid. */
  lines: Record<string, LineError>;
};

export const isRoll = (unitType: string) => unitType === "roll_meter";

/** Trims and turns a decimal comma ("12,5") into a dot. */
export function normalizeAmount(raw: string): string {
  return raw.trim().replace(",", ".");
}

function lineError(line: OrderFormLine): LineError | null {
  const v = normalizeAmount(line.amount);
  if (isRoll(line.unit_type)) {
    return METERS_RE.test(v) && Number(v) > 0 ? null : "meters_invalid";
  }
  if (!INT_RE.test(v)) return "quantity_invalid";
  const n = Number(v);
  return n >= 1 && n <= MAX_QUANTITY ? null : "quantity_invalid";
}

/**
 * Client-side checks mirroring the API (TEC-166): 1..200 lines, pieces as
 * a whole number 1..1,000,000, roll products in meters (> 0, at most two
 * decimals). Prices are never sent (K8).
 */
export function validateOrderForm(lines: OrderFormLine[]): OrderFormErrors {
  const errors: OrderFormErrors = { lines: {} };
  if (lines.length === 0) errors.form = "empty";
  else if (lines.length > MAX_ORDER_LINES) errors.form = "too_many";
  for (const line of lines) {
    const err = lineError(line);
    if (err) errors.lines[line.product_uuid] = err;
  }
  return errors;
}

export function formIsValid(errors: OrderFormErrors): boolean {
  return !errors.form && Object.keys(errors.lines).length === 0;
}

/** OrderItemInput lines: quantity for pieces, meters for rolls. */
export function toItemInputs(lines: OrderFormLine[]): OrderItemInput[] {
  return lines.map((line) => {
    const v = normalizeAmount(line.amount);
    return isRoll(line.unit_type)
      ? { product_uuid: line.product_uuid, meters: v }
      : { product_uuid: line.product_uuid, quantity: Number(v) };
  });
}

/** Adds a product once (the API refuses a product twice). */
export function addLine(
  lines: OrderFormLine[],
  product: { uuid: string; sku: string; name: string; unit_type: string },
): OrderFormLine[] {
  if (lines.some((l) => l.product_uuid === product.uuid)) return lines;
  return [
    ...lines,
    {
      product_uuid: product.uuid,
      sku: product.sku,
      name: product.name,
      unit_type: product.unit_type,
      amount: isRoll(product.unit_type) ? "" : "1",
    },
  ];
}

/** Form lines from a saved draft. */
export function linesFromOrder(order: Order): OrderFormLine[] {
  return (order.items ?? []).map((item) => ({
    product_uuid: item.product.uuid,
    sku: item.product.sku,
    name: item.product.name,
    unit_type: item.product.unit_type,
    amount:
      item.meters !== null && item.meters !== undefined
        ? item.meters
        : String(item.quantity ?? ""),
  }));
}

export type Tone = "default" | "success" | "warning" | "danger";

const STATUS_TONE: Partial<Record<OrderStatus, Tone>> = {
  draft: "default",
  submitted: "warning",
  approved: "warning",
  preparing: "warning",
  ready: "warning",
  processing: "warning",
  shipped: "success",
  delivered: "success",
  received: "success",
  cancelling: "danger",
  cancelled: "danger",
};

export function orderStatusTone(status: OrderStatus): Tone {
  return STATUS_TONE[status] ?? "default";
}

/** Parses an API decimal string; 0 when absent or invalid. */
export function amountNumber(value: string | null | undefined): number {
  const n = Number(value ?? "");
  return Number.isFinite(n) ? n : 0;
}
