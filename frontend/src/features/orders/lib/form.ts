import type {
  Order,
  OrderInvoiceStatus,
  OrderItemInput,
  OrderStatus,
} from "@/features/orders/services/orders.service";

/**
 * What the assigned units cover of a line: the assigned amount less the
 * free extra meters of a whole roll (TEC-1250, K7: a 3.5 m roll on a 3 m
 * line covers 3 m). On a charge line (19.11) the line was raised to 3.5 m
 * and the roll covers all of it.
 */
export function lineCovered(item: {
  assigned?: string | null;
  excess_meters?: string | null;
  excess_billing?: string | null;
}): number {
  const assigned = Number(item.assigned ?? 0) || 0;
  // TEC-1250 (19.11): a charge line's meters were raised to its pieces.
  if (item.excess_billing === "charge") return assigned;
  const excess = Number(item.excess_meters ?? 0) || 0;
  return Math.max(assigned - Math.max(excess, 0), 0);
}

/** API limits of OrderCreateInput / OrderItemInput. */
export const MAX_ORDER_LINES = 200;
export const MAX_QUANTITY = 1_000_000;
export const NOTE_MAX = 1000;
const METERS_RE = /^[0-9]{1,8}(\.[0-9]{1,2})?$/;
const INT_RE = /^[0-9]+$/;
/** order_items.unit_price: NUMERIC(14,4), never negative (TEC-1024). */
const UNIT_PRICE_RE = /^[0-9]{1,10}(\.[0-9]{1,4})?$/;

/**
 * TEC-1238/1239: the measure of an order line — box counts pieces (closed
 * boxes), meter counts meters (a roll_meter product, or a piece product
 * sold by the meter and cut from a box). One line per product and measure
 * (K10); a line's measure never changes (remove and add again).
 */
export type SaleMeasure = "box" | "meter";

/** Default usage credits of a cut piece (K3). */
export const DEFAULT_LINE_CREDITS = 5;
/** order_items.line_credits bound (TEC-1238). */
export const MAX_LINE_CREDITS = 1000;
/** The +/- step of a meter amount (TEC-1239). */
export const METER_STEP = 0.5;

/** One line of the order form; amount is the raw input. */
export type OrderFormLine = {
  product_uuid: string;
  sku: string;
  name: string;
  unit_type: string;
  /** TEC-1239: the line's measure; absent = derived from unit_type. */
  sale_measure?: SaleMeasure;
  amount: string;
  /**
   * TEC-1239 (K3): typed usage credits of each cut piece of a meter line
   * of a piece product (shared with its plotter mark, K12).
   */
  line_credits?: string;
  /** Meters of one closed box (roll.default_meters); null = unknown. */
  box_meters?: number | null;
  /** The product's minimum cut (K6); null = none. */
  min_cut?: number | null;
  /**
   * The piece product is sold by the meter now; false on a kept draft
   * meter line whose switch was turned off (the line stays, K1).
   */
  sellable_by_meter?: boolean;
  /** Center order for a dealer only (TEC-1024): typed unit price, "" = none. */
  unit_price?: string;
  /**
   * Center order for a dealer only (TEC-1064): typed discount %, "" = the
   * dealer's default one.
   */
  discount_pct?: string;
  /** Center order for a dealer only (TEC-1064): gift line charged 0. */
  is_gift?: boolean;
  /** TEC-1058: "sent plotter cut" (information only); undefined = off. */
  plotter_cut?: boolean;
  /**
   * TEC-1221: typed usage credits of the barcodes shipped on a plotter cut
   * line, "" = the product default. Sent only with plotter_cut on.
   */
  plotter_credits?: string;
};

export type LineError =
  | "quantity_invalid"
  | "meters_invalid"
  | "below_min_cut"
  | "line_credits_invalid"
  | "price_invalid"
  | "discount_invalid"
  | "credits_invalid";

/** order_items.plotter_credits bound (TEC-1221). */
export const MAX_PLOTTER_CREDITS = 1000;

/** Typed plotter credits: empty (product default) or a whole 1..1000. */
export function plotterCreditsValid(raw: string | undefined): boolean {
  const v = (raw ?? "").trim();
  if (v === "") return true;
  return INT_RE.test(v) && Number(v) >= 1 && Number(v) <= MAX_PLOTTER_CREDITS;
}

/** Typed plotter credits as the API value: 0 clears (product default). */
export function plotterCreditsValue(raw: string | undefined): number {
  const v = (raw ?? "").trim();
  return v === "" ? 0 : Number(v);
}

/** order_items.discount_pct / organizations.*_discount_pct: 0..100. */
const DISCOUNT_RE = /^[0-9]{1,3}(\.[0-9]{1,2})?$/;

/** A typed discount: empty (default) or 0..100 with at most 2 decimals. */
export function discountValid(raw: string | undefined): boolean {
  const v = normalizeAmount(raw ?? "");
  return v === "" || (DISCOUNT_RE.test(v) && Number(v) <= 100);
}

/**
 * TEC-1064: the preview of one line — the discounted unit price (four
 * decimals, like order_items.unit_price) and the line total (two). A gift
 * is 0, a typed unit price is charged as is, no price at all is null. The
 * server recomputes every amount.
 */
export function previewLine(input: {
  list: string | null | undefined;
  discount: string | null | undefined;
  manual?: string;
  gift?: boolean;
  amount: string;
}): { unit: number | null; total: number | null } {
  const amount = Number(normalizeAmount(input.amount));
  const qty = Number.isFinite(amount) && amount > 0 ? amount : 0;
  const round = (n: number, d: number) =>
    Math.round((n + Number.EPSILON) * 10 ** d) / 10 ** d;
  if (input.gift) return { unit: 0, total: 0 };
  const manual = normalizeAmount(input.manual ?? "");
  let unit: number | null = null;
  if (manual !== "" && unitPriceValid(manual)) unit = Number(manual);
  else if (input.list !== null && input.list !== undefined) {
    const pct = Number(normalizeAmount(input.discount ?? "")) || 0;
    unit = round((Number(input.list) * (100 - pct)) / 100, 4);
  }
  if (unit === null) return { unit: null, total: null };
  return { unit, total: round(unit * qty, 2) };
}

export type OrderFormErrors = {
  /** Whole-form problem: no line, or more than the API accepts. */
  form?: "empty" | "too_many";
  /** Errors by line key (lineKey: product + measure). */
  lines: Record<string, LineError>;
};

export const isRoll = (unitType: string) => unitType === "roll_meter";

/** The measure of a form line (a roll_meter product is always meter). */
export function lineMeasure(line: {
  unit_type: string;
  sale_measure?: SaleMeasure;
}): SaleMeasure {
  if (line.sale_measure) return line.sale_measure;
  return isRoll(line.unit_type) ? "meter" : "box";
}

/** TEC-1239: a line is identified by its product and measure (K10). */
export function lineKey(line: {
  product_uuid: string;
  unit_type: string;
  sale_measure?: SaleMeasure;
}): string {
  return `${line.product_uuid}:${lineMeasure(line)}`;
}

/** The line counts meters. */
export const isMeterLine = (line: {
  unit_type: string;
  sale_measure?: SaleMeasure;
}) => lineMeasure(line) === "meter";

/**
 * TEC-1239: a meter line of a piece product — cut from a closed box; it
 * carries usage credits (K3) and the product's minimum cut (K6).
 */
export const isCutLine = (line: {
  unit_type: string;
  sale_measure?: SaleMeasure;
}) => isMeterLine(line) && !isRoll(line.unit_type);

/** Typed line credits: a whole 1..1000. */
export function lineCreditsValid(raw: string | undefined): boolean {
  const v = (raw ?? "").trim();
  return INT_RE.test(v) && Number(v) >= 1 && Number(v) <= MAX_LINE_CREDITS;
}

/** The typed meters as a number; NaN when not a valid amount. */
export function metersNumber(raw: string): number {
  const v = normalizeAmount(raw);
  return METERS_RE.test(v) ? Number(v) : Number.NaN;
}

/**
 * TEC-1239: +/- one step (0.5 m) on a meter amount. A typed decimal stays
 * as typed until stepped; the result never goes below one step.
 */
export function stepMeters(raw: string, dir: 1 | -1): string {
  const cur = metersNumber(raw);
  const base = Number.isFinite(cur) ? cur : 0;
  // Snap to the step grid first, so 2.3 + step = 2.5 and 2.3 - step = 2.
  const snapped =
    dir > 0
      ? Math.floor(base / METER_STEP + 1e-9) * METER_STEP + METER_STEP
      : Math.ceil(base / METER_STEP - 1e-9) * METER_STEP - METER_STEP;
  const next = Math.max(METER_STEP, Math.round(snapped * 100) / 100);
  return String(next);
}

/** How many boxes the meters are (K2 hint "≈ X boxes"); null = unknown. */
export function boxesOf(meters: number, boxMeters: number | null | undefined) {
  if (!boxMeters || !(boxMeters > 0) || !Number.isFinite(meters)) return null;
  return Math.round((meters / boxMeters) * 100) / 100;
}

/**
 * TEC-1239: the typed meters reach a whole box — "a whole box is better".
 */
export function wholeBoxBetter(line: OrderFormLine): boolean {
  if (!isCutLine(line) || !line.box_meters) return false;
  const m = metersNumber(line.amount);
  return Number.isFinite(m) && m >= line.box_meters - 1e-9;
}

/**
 * TEC-1239: what "convert to boxes" makes of a meter line: whole boxes,
 * and the rest in meters when it is still a valid cut (at least the
 * minimum cut); a shorter rest is dropped.
 */
export function boxConversion(line: OrderFormLine): {
  boxes: number;
  rest: number;
} | null {
  if (!wholeBoxBetter(line) || !line.box_meters) return null;
  const m = metersNumber(line.amount);
  const boxes = Math.floor(m / line.box_meters + 1e-9);
  const rawRest = Math.round((m - boxes * line.box_meters) * 100) / 100;
  const rest =
    rawRest > 1e-9 && (line.min_cut ? rawRest >= line.min_cut - 1e-9 : true)
      ? rawRest
      : 0;
  return { boxes, rest };
}

/**
 * Applies "convert to boxes" to one meter line: the boxes go to the
 * product's box line (added, or summed into an existing one) and the
 * meter line keeps the rest or goes away.
 */
export function convertToBoxes(
  lines: OrderFormLine[],
  key: string,
): OrderFormLine[] {
  const line = lines.find((l) => lineKey(l) === key);
  const conv = line ? boxConversion(line) : null;
  if (!line || !conv) return lines;
  const boxKey = `${line.product_uuid}:box`;
  const box = lines.find((l) => lineKey(l) === boxKey);
  let out = lines.flatMap((l) => {
    if (lineKey(l) !== key) return [l];
    return conv.rest > 0 ? [{ ...l, amount: String(conv.rest) }] : [];
  });
  if (box) {
    const cur = Number(normalizeAmount(box.amount));
    const sum =
      (INT_RE.test(normalizeAmount(box.amount)) ? cur : 0) + conv.boxes;
    out = out.map((l) =>
      lineKey(l) === boxKey ? { ...l, amount: String(sum) } : l,
    );
  } else {
    const at = lines.findIndex((l) => lineKey(l) === key);
    const boxLine: OrderFormLine = {
      product_uuid: line.product_uuid,
      sku: line.sku,
      name: line.name,
      unit_type: line.unit_type,
      sale_measure: "box",
      amount: String(conv.boxes),
      box_meters: line.box_meters,
      min_cut: line.min_cut,
      sellable_by_meter: line.sellable_by_meter,
      ...(line.unit_price !== undefined
        ? { unit_price: "", discount_pct: "" }
        : {}),
    };
    out = [...out];
    out.splice(Math.min(at, out.length), 0, boxLine);
  }
  return out;
}

/** Trims and turns a decimal comma ("12,5") into a dot. */
export function normalizeAmount(raw: string): string {
  return raw.trim().replace(",", ".");
}

/** A typed unit price: empty (none) or >= 0 with at most 4 decimals. */
export function unitPriceValid(raw: string | undefined): boolean {
  const v = normalizeAmount(raw ?? "");
  return v === "" || UNIT_PRICE_RE.test(v);
}

function lineError(line: OrderFormLine): LineError | null {
  if (!unitPriceValid(line.unit_price)) return "price_invalid";
  if (!discountValid(line.discount_pct)) return "discount_invalid";
  if (
    line.plotter_cut === true &&
    !isCutLine(line) &&
    !plotterCreditsValid(line.plotter_credits)
  ) {
    return "credits_invalid";
  }
  const v = normalizeAmount(line.amount);
  if (isMeterLine(line)) {
    if (!(METERS_RE.test(v) && Number(v) > 0)) return "meters_invalid";
    if (!isCutLine(line)) return null;
    // TEC-1239 (K6): a cut is not shorter than the product's minimum cut.
    if (line.min_cut && Number(v) < line.min_cut - 1e-9) return "below_min_cut";
    return lineCreditsValid(line.line_credits) ? null : "line_credits_invalid";
  }
  if (!INT_RE.test(v)) return "quantity_invalid";
  const n = Number(v);
  return n >= 1 && n <= MAX_QUANTITY ? null : "quantity_invalid";
}

/**
 * Client-side checks mirroring the API (TEC-166): 1..200 lines, pieces as
 * a whole number 1..1,000,000, meter lines in meters (> 0, at most two
 * decimals). TEC-1239: a cut line is at least the minimum cut and carries
 * 1..1000 credits. Prices are never sent (K8).
 */
export function validateOrderForm(lines: OrderFormLine[]): OrderFormErrors {
  const errors: OrderFormErrors = { lines: {} };
  if (lines.length === 0) errors.form = "empty";
  else if (lines.length > MAX_ORDER_LINES) errors.form = "too_many";
  for (const line of lines) {
    const err = lineError(line);
    if (err) errors.lines[lineKey(line)] = err;
  }
  return errors;
}

export function formIsValid(errors: OrderFormErrors): boolean {
  return !errors.form && Object.keys(errors.lines).length === 0;
}

/**
 * OrderItemInput lines: quantity for box lines, meters for meter lines
 * (TEC-1239: with their measure, and a cut line with its credits); with
 * withPrice (center order for a dealer, TEC-1024) a typed unit price too.
 */
export function toItemInputs(
  lines: OrderFormLine[],
  withPrice = false,
): OrderItemInput[] {
  return lines.map((line) => {
    const v = normalizeAmount(line.amount);
    const cut = isCutLine(line);
    const base: OrderItemInput = isMeterLine(line)
      ? {
          product_uuid: line.product_uuid,
          meters: v,
          sale_measure: "meter",
          ...(cut ? { line_credits: Number(line.line_credits) } : {}),
        }
      : {
          product_uuid: line.product_uuid,
          quantity: Number(v),
          sale_measure: "box",
        };
    // TEC-1058: the plotter mark rides on every kind of line.
    // TEC-1221: a plotter cut line always states its credits (0 = the
    // product default), so a cleared field clears the stored override.
    // TEC-1239 (K12): a cut line's plotter mark shares its line credits.
    const marked: OrderItemInput =
      line.plotter_cut === undefined
        ? base
        : line.plotter_cut
          ? {
              ...base,
              plotter_cut: true,
              plotter_credits: cut
                ? Number(line.line_credits)
                : plotterCreditsValue(line.plotter_credits),
            }
          : { ...base, plotter_cut: false };
    if (!withPrice) return marked;
    const price = normalizeAmount(line.unit_price ?? "");
    const discount = normalizeAmount(line.discount_pct ?? "");
    // TEC-1064: a gift line carries neither a price nor a discount.
    if (line.is_gift) return { ...marked, is_gift: true };
    return {
      ...marked,
      ...(price !== "" ? { unit_price: price } : {}),
      ...(discount !== "" ? { discount_pct: discount } : {}),
    };
  });
}

/** A product picked in the order product search (TEC-1239). */
export type PickedProduct = {
  uuid: string;
  sku: string;
  name: string;
  unit_type: string;
  /** roll.sellable_by_meter */
  sellable_by_meter?: boolean;
  /** roll.default_meters: meters of one box */
  box_meters?: number | null;
  /** roll.min_cut_meters */
  min_cut?: number | null;
  /** usage_credits: default credits of a cut piece */
  usage_credits?: number | null;
};

/** The measures a product can be added with (+Box / +Meter). */
export function productMeasures(p: {
  unit_type: string;
  sellable_by_meter?: boolean;
}): SaleMeasure[] {
  if (isRoll(p.unit_type)) return ["meter"];
  return p.sellable_by_meter ? ["box", "meter"] : ["box"];
}

/**
 * Adds a product once per measure (the API refuses a product twice in the
 * same measure, K10). A box line starts at 1; a meter line is typed (a cut
 * line starts with the product's credits, K3).
 */
export function addLine(
  lines: OrderFormLine[],
  product: PickedProduct,
  measure: SaleMeasure = isRoll(product.unit_type) ? "meter" : "box",
): OrderFormLine[] {
  const line: OrderFormLine = {
    product_uuid: product.uuid,
    sku: product.sku,
    name: product.name,
    unit_type: product.unit_type,
    sale_measure: measure,
    amount: measure === "meter" ? "" : "1",
    box_meters: product.box_meters ?? null,
    min_cut: product.min_cut ?? null,
    sellable_by_meter: product.sellable_by_meter ?? false,
  };
  if (lines.some((l) => lineKey(l) === lineKey(line))) return lines;
  if (isCutLine(line)) {
    line.line_credits = String(product.usage_credits ?? DEFAULT_LINE_CREDITS);
  }
  return [...lines, line];
}

/** Removes one line (product + measure). */
export function removeLine(
  lines: OrderFormLine[],
  key: string,
): OrderFormLine[] {
  return lines.filter((l) => lineKey(l) !== key);
}

/** Patches one line (product + measure). */
export function patchLine(
  lines: OrderFormLine[],
  key: string,
  patch: Partial<OrderFormLine>,
): OrderFormLine[] {
  return lines.map((l) => (lineKey(l) === key ? { ...l, ...patch } : l));
}

/** Form lines from a saved draft. */
export function linesFromOrder(order: Order): OrderFormLine[] {
  return (order.items ?? []).map((item) => {
    const meter = itemMeasure(item) === "meter";
    const num = (v: string | null | undefined) =>
      v !== null && v !== undefined && Number(v) > 0 ? Number(v) : null;
    return {
      product_uuid: item.product.uuid,
      sku: item.product.sku,
      name: item.product.name,
      unit_type: item.product.unit_type,
      sale_measure: meter ? "meter" : "box",
      amount:
        item.meters !== null && item.meters !== undefined
          ? item.meters
          : String(item.quantity ?? ""),
      plotter_cut: item.plotter_cut ?? false,
      plotter_credits:
        item.plotter_credits != null ? String(item.plotter_credits) : "",
      ...(item.line_credits != null
        ? { line_credits: String(item.line_credits) }
        : meter && !isRoll(item.product.unit_type)
          ? { line_credits: String(DEFAULT_LINE_CREDITS) }
          : {}),
      box_meters: num(item.product.box_meters),
      min_cut: num(item.product.min_cut_meters),
      sellable_by_meter: item.product.sellable_by_meter ?? true,
    };
  });
}

/** TEC-1239: the measure of a saved order line. */
export function itemMeasure(item: {
  sale_measure?: SaleMeasure;
  meters?: string | null;
}): SaleMeasure {
  if (item.sale_measure) return item.sale_measure;
  return item.meters !== null && item.meters !== undefined ? "meter" : "box";
}

/** TEC-1239: a saved meter line of a piece product (a cut line). */
export function cutItem(item: {
  sale_measure?: SaleMeasure;
  meters?: string | null;
  product: { unit_type: string };
}): boolean {
  return itemMeasure(item) === "meter" && !isRoll(item.product.unit_type);
}

/** The order line of a form line (product + measure). */
export function orderItemOf(order: Order, line: OrderFormLine) {
  const measure = lineMeasure(line);
  return order.items?.find(
    (i) => i.product.uuid === line.product_uuid && itemMeasure(i) === measure,
  );
}

/**
 * TEC-1056: line keys of the edited lines whose amount is below the
 * units already assigned to them (the API answers 409
 * ORDER_LINE_BELOW_ASSIGNED; unassign first).
 */
export function linesBelowAssigned(
  order: Order,
  lines: OrderFormLine[],
): Set<string> {
  const out = new Set<string>();
  for (const line of lines) {
    const item = orderItemOf(order, line);
    const assigned = item ? lineCovered(item) : 0;
    if (!item || !(assigned > 0)) continue;
    const v = Number(normalizeAmount(line.amount));
    if (Number.isFinite(v) && v < assigned - 1e-9) out.add(lineKey(line));
  }
  return out;
}

/**
 * TEC-1239: kept draft cut lines of a product no longer sold by the meter
 * (K1: the line stays; the buyer is warned).
 */
export function staleMeterLine(line: OrderFormLine): boolean {
  return isCutLine(line) && line.sellable_by_meter === false;
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

/** Tone of the manual invoice status (TEC-1030). */
export function invoiceStatusTone(status: OrderInvoiceStatus): Tone {
  if (status === "invoiced") return "success";
  if (status === "partial") return "warning";
  return "default";
}

/** Parses an API decimal string; 0 when absent or invalid. */
export function amountNumber(value: string | null | undefined): number {
  const n = Number(value ?? "");
  return Number.isFinite(n) ? n : 0;
}
