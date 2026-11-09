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
};

export type LineError = 'quantity_invalid' | 'meters_invalid' | 'price_invalid';
export type CartErrors = { form?: 'empty' | 'too_many'; lines: Record<string, LineError> };

export type ItemInput = {
  product_uuid: string;
  quantity?: number;
  meters?: string;
  unit_price?: string;
  plotter_cut?: boolean;
};

export const isRoll = (unitType: string) => unitType === 'roll_meter';

/** Trims and turns a decimal comma ("12,5") into a dot. */
export function normalizeAmount(raw: string): string {
  return raw.trim().replace(',', '.');
}

export function lineError(line: CartLine): LineError | null {
  const price = normalizeAmount(line.unit_price ?? '');
  if (price && !PRICE_RE.test(price)) return 'price_invalid';
  const v = normalizeAmount(line.amount);
  if (isRoll(line.unit_type)) return METERS_RE.test(v) && Number(v) > 0 ? null : 'meters_invalid';
  if (!INT_RE.test(v)) return 'quantity_invalid';
  const n = Number(v);
  return n >= 1 && n <= MAX_QUANTITY ? null : 'quantity_invalid';
}

export function validateCart(lines: CartLine[]): CartErrors {
  const errors: CartErrors = { lines: {} };
  if (lines.length === 0) errors.form = 'empty';
  else if (lines.length > MAX_ORDER_LINES) errors.form = 'too_many';
  for (const line of lines) {
    const err = lineError(line);
    if (err) errors.lines[line.product_uuid] = err;
  }
  return errors;
}

export function cartIsValid(errors: CartErrors): boolean {
  return !errors.form && Object.keys(errors.lines).length === 0;
}

/**
 * Order item inputs: quantity for pieces, meters for rolls. A dealer never
 * sends prices (K8); the center may send a manual unit price when it orders
 * for a dealer (`withPrice`, TEC-1024).
 */
export function toItemInputs(lines: CartLine[], withPrice = false): ItemInput[] {
  return lines.map((line) => {
    const v = normalizeAmount(line.amount);
    const item: ItemInput = isRoll(line.unit_type)
      ? { product_uuid: line.product_uuid, meters: v }
      : { product_uuid: line.product_uuid, quantity: Number(v) };
    const price = normalizeAmount(line.unit_price ?? '');
    if (withPrice && price) item.unit_price = price;
    if (line.plotter_cut !== undefined) item.plotter_cut = line.plotter_cut;
    return item;
  });
}

export type PreviewRow = { name: string; amount: string; roll: boolean };

/**
 * Lines listed in the "send the order?" confirmation: name and the amount as
 * typed (pieces or meters, local decimal comma kept), the first `max` lines
 * and how many more follow.
 */
export function submitPreview(lines: CartLine[], max = 8): { rows: PreviewRow[]; more: number } {
  const rows = lines.slice(0, max).map((l) => ({ name: l.name, amount: l.amount.trim(), roll: isRoll(l.unit_type) }));
  return { rows, more: Math.max(0, lines.length - max) };
}

/** Adds a product once (the API refuses a product twice); a roll starts empty, a piece at 1. */
export function addLine(lines: CartLine[], product: { uuid: string; sku: string; name: string; unit_type: string }): CartLine[] {
  if (lines.some((l) => l.product_uuid === product.uuid)) return lines;
  return [
    ...lines,
    { product_uuid: product.uuid, sku: product.sku, name: product.name, unit_type: product.unit_type, amount: isRoll(product.unit_type) ? '' : '1' },
  ];
}

/** One line of an order being edited (TEC-1057): a cart line plus what is already assigned to it. */
export type EditLine = CartLine & { assigned: number };

type EditableItem = {
  product: { uuid: string; sku: string; name: string; unit_type: string };
  quantity: number | null;
  meters: string | null;
  assigned: string;
  plotter_cut: boolean;
};

/**
 * The editor starts from the order's lines: the amount as stored, the
 * assigned amount and, for the seller (`withPlotter`), the plotter mark so it
 * is sent back; without it the mark is left out of the request.
 */
export function linesFromItems(items: readonly EditableItem[], withPlotter: boolean): EditLine[] {
  return items.map((i) => {
    const assigned = Number(i.assigned);
    const line: EditLine = {
      product_uuid: i.product.uuid,
      sku: i.product.sku,
      name: i.product.name,
      unit_type: i.product.unit_type,
      amount: i.meters != null ? i.meters : String(i.quantity ?? ''),
      assigned: Number.isFinite(assigned) ? assigned : 0,
    };
    if (withPlotter) line.plotter_cut = i.plotter_cut;
    return line;
  });
}

/**
 * Product uuids of the edited lines typed below what is already assigned
 * (the API answers 409 ORDER_LINE_BELOW_ASSIGNED; unassign first). An empty
 * or non-numeric amount is the line's own error, not this one.
 */
export function linesBelowAssigned(lines: readonly EditLine[]): Set<string> {
  const out = new Set<string>();
  for (const l of lines) {
    if (!(l.assigned > 0)) continue;
    const raw = normalizeAmount(l.amount);
    const v = Number(raw);
    if (raw !== '' && Number.isFinite(v) && v < l.assigned - 1e-9) out.add(l.product_uuid);
  }
  return out;
}

/** Adds a product to the edit once; a new line has nothing assigned and, for the seller, no plotter mark. */
export function addEditLine(
  lines: EditLine[],
  product: { uuid: string; sku: string; name: string; unit_type: string },
  withPlotter: boolean,
): EditLine[] {
  const next = addLine(lines, product);
  if (next === lines) return lines;
  const added = next[next.length - 1]!;
  return [...lines, { ...added, assigned: 0, ...(withPlotter ? { plotter_cut: false } : {}) }];
}

/** Whether the edit changes anything against the order: a line added or removed, an amount or a plotter mark. */
export function editChanged(before: readonly EditLine[], after: readonly EditLine[]): boolean {
  if (before.length !== after.length) return true;
  const old = new Map(before.map((l) => [l.product_uuid, l]));
  return after.some((l) => {
    const o = old.get(l.product_uuid);
    if (!o || o.plotter_cut !== l.plotter_cut) return true;
    const a = Number(normalizeAmount(o.amount));
    const b = Number(normalizeAmount(l.amount));
    return !(Number.isFinite(a) && Number.isFinite(b) && a === b);
  });
}
