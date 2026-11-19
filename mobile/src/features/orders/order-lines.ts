/**
 * Order cart lines: pure rules mirrored from the API (TEC-166) and the web
 * form, so the mobile cart refuses what the server would refuse. No `@/`
 * imports: node --test runs this file directly.
 */

/** API limits of OrderCreateInput / OrderItemInput. */
export const MAX_ORDER_LINES = 200;
export const MAX_QUANTITY = 1_000_000;
export const NOTE_MAX = 1000;

const METERS_RE = /^[0-9]{1,8}(\.[0-9]{1,2})?$/;
/** Manual unit price of a center order (TEC-1024): ≥ 0, up to 4 decimals. */
const PRICE_RE = /^[0-9]{1,12}(\.[0-9]{1,4})?$/;
const INT_RE = /^[0-9]+$/;
/** Manual discount (TEC-1064): 0..100, up to 2 decimals. */
const DISCOUNT_RE = /^[0-9]{1,3}(\.[0-9]{1,2})?$/;
/** TEC-1221: usage credits of a plotter cut line, 1..1000. */
export const MAX_PLOTTER_CREDITS = 1000;

/** One cart line; amount is the raw input (pieces or meters). */
export type CartLine = {
  product_uuid: string;
  sku: string;
  name: string;
  unit_type: string;
  amount: string;
  /** Center orders only: a manual unit price; empty lets the server price the line. */
  unit_price?: string;
  /** TEC-1059: "sent plotter cut" (information only); undefined is not sent. */
  plotter_cut?: boolean;
  /**
   * TEC-1221: usage credits of the barcodes shipped on a plotter cut line
   * (raw input); empty uses the product default. Undefined is not sent.
   */
  plotter_credits?: string;
  /** Center orders only (TEC-1064): a manual discount; empty keeps the dealer's default. */
  discount_pct?: string;
  /** Center orders only (TEC-1064): the line is a gift, charged 0. */
  is_gift?: boolean;
  /**
   * TEC-1240: the line's measure. Undefined on lines saved before it: a
   * roll_meter product is a meter line, any other a box line. Never changes
   * on a line (K10): another measure is another line.
   */
  measure?: SaleMeasure;
  /** TEC-1240: product roll settings read at add time (box length, minimum cut, meter sale switch). */
  box_meters?: number | null;
  min_cut?: number | null;
  sellable_by_meter?: boolean;
  /** TEC-1240 (K3): the product's usage credits, the default credits of a meter line. */
  default_credits?: number | null;
  /** TEC-1240 (K3/K12): credits of each cut piece of a meter line (raw input); empty = default. */
  line_credits?: string;
  /** TEC-1240: an edited line's stored meters; unchanged meters are not checked against the minimum cut again. */
  kept_meters?: string;
};

/** TEC-1238/1240: a line counts boxes (pieces) or meters. */
export type SaleMeasure = "box" | "meter";

/** What the cart needs of a product: the catalog product, or the order line's product ref. */
export type CartProduct = {
  uuid: string;
  sku: string;
  name: string;
  unit_type: string;
  roll?: {
    default_meters: number | null;
    min_cut_meters: number | null;
    sellable_by_meter?: boolean;
  } | null;
  usage_credits?: number;
};

export type LineError =
  | "quantity_invalid"
  | "meters_invalid"
  | "below_min_cut"
  | "price_invalid"
  | "discount_invalid"
  | "credits_invalid"
  | "line_credits_invalid";
export type CartErrors = {
  form?: "empty" | "too_many";
  lines: Record<string, LineError>;
};

export type ItemInput = {
  product_uuid: string;
  quantity?: number;
  meters?: string;
  unit_price?: string;
  plotter_cut?: boolean;
  plotter_credits?: number;
  discount_pct?: string;
  is_gift?: boolean;
  /** TEC-1240: sent only on a meter line of a piece product; otherwise the server derives it. */
  sale_measure?: SaleMeasure;
  line_credits?: number;
};

export const isRoll = (unitType: string) => unitType === "roll_meter";

/** The line's measure: as chosen, else derived from the product (lines saved before TEC-1240). */
export function lineMeasure(line: {
  unit_type: string;
  measure?: SaleMeasure;
}): SaleMeasure {
  return line.measure ?? (isRoll(line.unit_type) ? "meter" : "box");
}

/** Whether the line counts meters (a roll_meter product, or a piece product sold by the meter). */
export const byMeter = (line: { unit_type: string; measure?: SaleMeasure }) =>
  lineMeasure(line) === "meter";

/** A meter line of a piece product: cut from a box, its pieces carry the line's credits (K3). */
export const isCutLine = (line: { unit_type: string; measure?: SaleMeasure }) =>
  byMeter(line) && !isRoll(line.unit_type);

/** Cart key of a line: one line per product and measure (K10). */
export function lineKey(line: {
  product_uuid: string;
  unit_type: string;
  measure?: SaleMeasure;
}): string {
  return `${line.product_uuid}:${lineMeasure(line)}`;
}

/** Whether a product may be ordered in this measure: box (any non-roll product), meter (roll_meter or the switch on). */
export function canOrderBy(
  product: CartProduct,
  measure: SaleMeasure,
): boolean {
  if (isRoll(product.unit_type)) return measure === "meter";
  return measure === "box" || product.roll?.sellable_by_meter === true;
}

/** Trims and turns a decimal comma ("12,5") into a dot. */
export function normalizeAmount(raw: string): string {
  return raw.trim().replace(",", ".");
}

/** A manual discount the API would refuse; empty is valid (the default discount applies). */
export function discountInvalid(raw: string | undefined): boolean {
  const v = normalizeAmount(raw ?? "");
  return !!v && (!DISCOUNT_RE.test(v) || Number(v) > 100);
}

/** Plotter credits the API would refuse; empty is valid (the product default applies). */
export function plotterCreditsInvalid(raw: string | undefined): boolean {
  const v = (raw ?? "").trim();
  return (
    !!v && (!INT_RE.test(v) || Number(v) < 1 || Number(v) > MAX_PLOTTER_CREDITS)
  );
}

/** Meters of the line as a number; null while not a valid meter amount. */
export function lineMeters(line: CartLine): number | null {
  const v = normalizeAmount(line.amount);
  return METERS_RE.test(v) && Number(v) > 0 ? Number(v) : null;
}

/** K6: meters typed below the product's minimum cut (an edited line with its stored meters is not checked again). */
export function belowMinCut(line: CartLine): boolean {
  const m = lineMeters(line);
  if (m === null || !line.min_cut || !byMeter(line)) return false;
  const kept =
    line.kept_meters !== undefined
      ? Number(normalizeAmount(line.kept_meters))
      : NaN;
  if (Number.isFinite(kept) && Math.abs(kept - m) < 1e-9) return false;
  return m < line.min_cut - 1e-9;
}

export function lineError(line: CartLine): LineError | null {
  // K12: a meter line's plotter mark shares the line credits; it has no credits of its own.
  if (
    !isCutLine(line) &&
    line.plotter_cut === true &&
    plotterCreditsInvalid(line.plotter_credits)
  )
    return "credits_invalid";
  if (isCutLine(line) && plotterCreditsInvalid(line.line_credits))
    return "line_credits_invalid";
  const price = normalizeAmount(line.unit_price ?? "");
  if (price && !PRICE_RE.test(price)) return "price_invalid";
  if (discountInvalid(line.discount_pct)) return "discount_invalid";
  const v = normalizeAmount(line.amount);
  if (byMeter(line)) {
    if (lineMeters(line) === null) return "meters_invalid";
    return belowMinCut(line) ? "below_min_cut" : null;
  }
  if (!INT_RE.test(v)) return "quantity_invalid";
  const n = Number(v);
  return n >= 1 && n <= MAX_QUANTITY ? null : "quantity_invalid";
}

/** Errors of the amount field itself (its border turns red); the others belong to price, discount or credits. */
export const isAmountError = (e: LineError | null | undefined) =>
  e === "quantity_invalid" || e === "meters_invalid" || e === "below_min_cut";

export function validateCart(lines: CartLine[]): CartErrors {
  const errors: CartErrors = { lines: {} };
  if (lines.length === 0) errors.form = "empty";
  else if (lines.length > MAX_ORDER_LINES) errors.form = "too_many";
  for (const line of lines) {
    const err = lineError(line);
    if (err) errors.lines[lineKey(line)] = err;
  }
  return errors;
}

export function cartIsValid(errors: CartErrors): boolean {
  return !errors.form && Object.keys(errors.lines).length === 0;
}

/**
 * Order item inputs: quantity for pieces, meters for rolls. A dealer never
 * sends prices (K8); the center may send a manual unit price when it orders
 * for a dealer (`withPrice`, TEC-1024), and a manual discount or the gift
 * flag (`withDiscount`, TEC-1064); a gift sends no price and no discount.
 */
export function toItemInputs(
  lines: CartLine[],
  withPrice = false,
  withDiscount = false,
): ItemInput[] {
  return lines.map((line) => {
    const v = normalizeAmount(line.amount);
    const item: ItemInput = byMeter(line)
      ? { product_uuid: line.product_uuid, meters: v }
      : { product_uuid: line.product_uuid, quantity: Number(v) };
    const price = normalizeAmount(line.unit_price ?? "");
    if (line.plotter_cut !== undefined) item.plotter_cut = line.plotter_cut;
    if (isCutLine(line)) {
      // TEC-1240: a piece product's meter line says so; its credits are the line's (K3/K12), empty = default or kept.
      item.sale_measure = "meter";
      const credits = (line.line_credits ?? "").trim();
      if (credits) item.line_credits = Number(credits);
    } else if (
      line.plotter_cut === true &&
      line.plotter_credits !== undefined
    ) {
      // TEC-1221: credits go only with a plotter cut line; an emptied field clears them (0).
      const credits = line.plotter_credits.trim();
      item.plotter_credits = credits ? Number(credits) : 0;
    }
    if (withDiscount && line.is_gift) {
      item.is_gift = true;
      return item;
    }
    if (withPrice && price) item.unit_price = price;
    const discount = normalizeAmount(line.discount_pct ?? "");
    if (withDiscount && discount) item.discount_pct = discount;
    return item;
  });
}

export type PreviewRow = { name: string; amount: string; roll: boolean };

/**
 * Lines listed in the "send the order?" confirmation: name and the amount as
 * typed (pieces or meters, local decimal comma kept), the first `max` lines
 * and how many more follow.
 */
export function submitPreview(
  lines: CartLine[],
  max = 8,
): { rows: PreviewRow[]; more: number } {
  const rows = lines
    .slice(0, max)
    .map((l) => ({ name: l.name, amount: l.amount.trim(), roll: byMeter(l) }));
  return { rows, more: Math.max(0, lines.length - max) };
}

/** Meters as typed in the cart: two decimals at most, no trailing zeros, the decimal separator of `like`. */
export function formatMeters(v: number, like = ""): string {
  const s = String(Math.round(v * 100) / 100);
  return like.includes(",") ? s.replace(".", ",") : s;
}

/**
 * Adds a product line once per measure (K10); a box line starts at 1, a meter
 * line at the product's minimum cut (empty without one). A measure the
 * product is not sold by is refused (the lines come back unchanged).
 */
export function addLine(
  lines: CartLine[],
  product: CartProduct,
  measure?: SaleMeasure,
): CartLine[] {
  const m: SaleMeasure =
    measure ?? (isRoll(product.unit_type) ? "meter" : "box");
  if (!canOrderBy(product, m)) return lines;
  const key = `${product.uuid}:${m}`;
  if (lines.some((l) => lineKey(l) === key)) return lines;
  const minCut = product.roll?.min_cut_meters ?? null;
  const line: CartLine = {
    product_uuid: product.uuid,
    sku: product.sku,
    name: product.name,
    unit_type: product.unit_type,
    amount: m === "meter" ? (minCut ? formatMeters(minCut) : "") : "1",
  };
  if (product.roll || !isRoll(product.unit_type)) {
    line.measure = m;
    line.box_meters = product.roll?.default_meters ?? null;
    line.min_cut = minCut;
    line.sellable_by_meter = product.roll?.sellable_by_meter === true;
  }
  if (m === "meter" && !isRoll(product.unit_type)) {
    line.default_credits = product.usage_credits ?? null;
    line.line_credits = "";
  }
  return [...lines, line];
}

/** Meter step of the cart's +/- buttons. */
export const METER_STEP = 0.5;

/**
 * The meters after a +/- tap: one step from the typed value, never below the
 * minimum cut (or one step); from an empty or invalid field "+" starts at the
 * minimum cut. The decimal separator the user typed is kept.
 */
export function stepMeters(
  raw: string,
  dir: 1 | -1,
  minCut?: number | null,
): string {
  const floor = minCut && minCut > 0 ? minCut : METER_STEP;
  const v = Number(normalizeAmount(raw));
  if (!normalizeAmount(raw) || !Number.isFinite(v) || v <= 0)
    return formatMeters(floor, raw);
  const next = Math.round((v + dir * METER_STEP) * 100) / 100;
  return formatMeters(Math.max(floor, next), raw);
}

/** "≈ X boxes" of a meter line: meters / box length, one decimal; null without both. */
export function boxEquivalent(line: CartLine): number | null {
  const m = lineMeters(line);
  if (m === null || !line.box_meters || !isCutLine(line)) return null;
  return Math.round((m / line.box_meters) * 10) / 10;
}

/**
 * Whole box suggestion of a meter line (TEC-1240). With the box length B, the
 * minimum cut C and the meters M:
 * - what a box would leave after the cut is shorter than max(C, 10 % of B)
 *   (the rest would be a remnant): order ceil(M / B) boxes instead;
 * - M covers a whole box and the rest is a valid cut: order floor(M / B)
 *   boxes and the rest by the meter.
 * Null when neither applies (or the line is not a meter line of a box).
 */
export function wholeBoxSuggestion(
  line: CartLine,
): { boxes: number; meters: number } | null {
  const m = lineMeters(line);
  const box = line.box_meters;
  if (m === null || !box || box <= 0 || !isCutLine(line)) return null;
  const up = Math.ceil(m / box - 1e-9);
  const gap = up * box - m;
  if (gap < Math.max(line.min_cut ?? 0, box * 0.1) - 1e-9)
    return { boxes: up, meters: 0 };
  const boxes = Math.floor(m / box + 1e-9);
  if (boxes < 1) return null;
  const rest = Math.round((m - boxes * box) * 100) / 100;
  return rest >= (line.min_cut ?? 0) ? { boxes, meters: rest } : null;
}

/**
 * Applies the whole box suggestion of the meter line `key` (one tap): the
 * boxes go to the product's box line (added, or its pieces increased), the
 * rest stays on the meter line, which is dropped when nothing is left.
 * `extra` completes a new box line (e.g. the editor's assigned 0).
 */
export function convertToBoxes<T extends CartLine>(
  lines: T[],
  key: string,
  extra: Partial<T> = {},
): T[] {
  const meter = lines.find((l) => lineKey(l) === key);
  const s = meter ? wholeBoxSuggestion(meter) : null;
  if (!meter || !s) return lines;
  const boxKey = `${meter.product_uuid}:box`;
  const hasBox = lines.some((l) => lineKey(l) === boxKey);
  const out: T[] = [];
  for (const l of lines) {
    const k = lineKey(l);
    if (k === boxKey) {
      const n = Number(normalizeAmount(l.amount));
      out.push({
        ...l,
        amount: String(
          (INT_RE.test(normalizeAmount(l.amount)) ? n : 0) + s.boxes,
        ),
      });
    } else if (k === key) {
      if (!hasBox) {
        const box = {
          ...extra,
          product_uuid: meter.product_uuid,
          sku: meter.sku,
          name: meter.name,
          unit_type: meter.unit_type,
          amount: String(s.boxes),
          measure: "box",
          box_meters: meter.box_meters,
          min_cut: meter.min_cut,
          sellable_by_meter: meter.sellable_by_meter,
        } as T;
        out.push(box);
      }
      if (s.meters > 0)
        out.push({ ...l, amount: formatMeters(s.meters, l.amount) });
    } else out.push(l);
  }
  return out;
}

/**
 * Product settings the cart reads at add time, filled in on lines that miss
 * them (an order's lines carry only the product ref; the editor reads the
 * catalog product). Lines that already know them are kept as they are.
 */
export function withProductInfo<T extends CartLine>(
  lines: T[],
  products: ReadonlyMap<string, CartProduct>,
): T[] {
  let changed = false;
  const out = lines.map((l) => {
    const p = products.get(l.product_uuid);
    if (!p || l.sellable_by_meter !== undefined) return l;
    changed = true;
    const next: T = {
      ...l,
      measure: lineMeasure(l),
      box_meters: p.roll?.default_meters ?? null,
      min_cut: p.roll?.min_cut_meters ?? null,
      sellable_by_meter: p.roll?.sellable_by_meter === true,
    };
    if (isCutLine(l)) next.default_credits = p.usage_credits ?? null;
    return next;
  });
  return changed ? out : lines;
}

/** The measure a product line could still be added in from the cart ("+ Metre" / "+ Kutu"); null when none. */
export function otherMeasure(
  lines: readonly CartLine[],
  line: CartLine,
): SaleMeasure | null {
  if (isRoll(line.unit_type) || !line.sellable_by_meter) return null;
  const other: SaleMeasure = lineMeasure(line) === "box" ? "meter" : "box";
  return lines.some(
    (l) => l.product_uuid === line.product_uuid && lineMeasure(l) === other,
  )
    ? null
    : other;
}

/** Rebuilds the product an existing line was added from (to add its other measure). */
export function productOf(line: CartLine): CartProduct {
  return {
    uuid: line.product_uuid,
    sku: line.sku,
    name: line.name,
    unit_type: line.unit_type,
    roll: {
      default_meters: line.box_meters ?? null,
      min_cut_meters: line.min_cut ?? null,
      sellable_by_meter: line.sellable_by_meter,
    },
    ...(line.default_credits != null
      ? { usage_credits: line.default_credits }
      : {}),
  };
}

/** One line of an order being edited (TEC-1057): a cart line plus what is already assigned to it. */
export type EditLine = CartLine & { assigned: number };

type EditableItem = {
  product: { uuid: string; sku: string; name: string; unit_type: string };
  quantity: number | null;
  meters: string | null;
  assigned: string;
  plotter_cut: boolean;
  plotter_credits?: number | null;
  /** TEC-1238: the line's measure and, on a meter line of a piece product, its credits. */
  sale_measure?: SaleMeasure;
  line_credits?: number | null;
  /** TEC-1250 (K7): free extra meters of a whole roll on the line. */
  excess_meters?: string | null;
  /** TEC-1250 (19.11): on a charge line meters were raised to the pieces. */
  excess_billing?: string | null;
};

/**
 * The editor starts from the order's lines: the measure and the amount as
 * stored (meters kept so an unchanged line is not checked against the
 * minimum cut again), the assigned amount, the line credits of a meter line
 * and, for the seller (`withPlotter`), the plotter mark so it is sent back;
 * without it the mark is left out of the request.
 */
export function linesFromItems(
  items: readonly EditableItem[],
  withPlotter: boolean,
): EditLine[] {
  return items.map((i) => {
    // TEC-1250 (K7): a whole roll's free extra meters do not count — a
    // 3.5 m roll on a 3 m line covers 3 m, so keeping 3 m is not "below".
    // On a charge line (19.11) the line was raised to the roll: all counts.
    const assigned =
      i.excess_billing === "charge"
        ? Number(i.assigned)
        : Number(i.assigned) - Math.max(Number(i.excess_meters ?? 0) || 0, 0);
    const measure: SaleMeasure =
      i.sale_measure ?? (i.meters != null ? "meter" : "box");
    const line: EditLine = {
      product_uuid: i.product.uuid,
      sku: i.product.sku,
      name: i.product.name,
      unit_type: i.product.unit_type,
      amount: i.meters != null ? i.meters : String(i.quantity ?? ""),
      assigned: Number.isFinite(assigned) ? assigned : 0,
      measure,
    };
    if (measure === "meter" && i.meters != null) line.kept_meters = i.meters;
    if (isCutLine({ unit_type: i.product.unit_type, measure })) {
      line.line_credits = i.line_credits != null ? String(i.line_credits) : "";
    }
    if (withPlotter) {
      line.plotter_cut = i.plotter_cut;
      // Undefined until typed: a line without credits sends none (the API keeps it).
      if (i.plotter_credits != null && !isCutLine(line))
        line.plotter_credits = String(i.plotter_credits);
    }
    return line;
  });
}

/**
 * Keys of the edited lines typed below what is already assigned
 * (the API answers 409 ORDER_LINE_BELOW_ASSIGNED; unassign first). An empty
 * or non-numeric amount is the line's own error, not this one.
 */
export function linesBelowAssigned(lines: readonly EditLine[]): Set<string> {
  const out = new Set<string>();
  for (const l of lines) {
    if (!(l.assigned > 0)) continue;
    const raw = normalizeAmount(l.amount);
    const v = Number(raw);
    if (raw !== "" && Number.isFinite(v) && v < l.assigned - 1e-9)
      out.add(lineKey(l));
  }
  return out;
}

/** Adds a product line to the edit once per measure; a new line has nothing assigned and, for the seller, no plotter mark. */
export function addEditLine(
  lines: EditLine[],
  product: CartProduct,
  withPlotter: boolean,
  measure?: SaleMeasure,
): EditLine[] {
  const next = addLine(lines, product, measure);
  if (next === lines) return lines;
  const added = next[next.length - 1]!;
  return [
    ...lines,
    { ...added, assigned: 0, ...(withPlotter ? { plotter_cut: false } : {}) },
  ];
}

/** Whether the edit changes anything against the order: a line added or removed, an amount, a plotter mark or the credits. */
export function editChanged(
  before: readonly EditLine[],
  after: readonly EditLine[],
): boolean {
  if (before.length !== after.length) return true;
  const old = new Map(before.map((l) => [lineKey(l), l]));
  return after.some((l) => {
    const o = old.get(lineKey(l));
    if (!o || o.plotter_cut !== l.plotter_cut) return true;
    if (
      l.plotter_cut &&
      (o.plotter_credits ?? "").trim() !== (l.plotter_credits ?? "").trim()
    )
      return true;
    if (
      isCutLine(l) &&
      (o.line_credits ?? "").trim() !== (l.line_credits ?? "").trim()
    )
      return true;
    const a = Number(normalizeAmount(o.amount));
    const b = Number(normalizeAmount(l.amount));
    return !(Number.isFinite(a) && Number.isFinite(b) && a === b);
  });
}
