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
const INT_RE = /^[0-9]+$/;

/** One cart line; amount is the raw input (pieces or meters). */
export type CartLine = {
  product_uuid: string;
  sku: string;
  name: string;
  unit_type: string;
  amount: string;
};

export type LineError = 'quantity_invalid' | 'meters_invalid';
export type CartErrors = { form?: 'empty' | 'too_many'; lines: Record<string, LineError> };

export type ItemInput = { product_uuid: string; quantity?: number; meters?: string };

export const isRoll = (unitType: string) => unitType === 'roll_meter';

/** Trims and turns a decimal comma ("12,5") into a dot. */
export function normalizeAmount(raw: string): string {
  return raw.trim().replace(',', '.');
}

export function lineError(line: CartLine): LineError | null {
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

/** Order item inputs: quantity for pieces, meters for rolls. Prices are never sent (K8). */
export function toItemInputs(lines: CartLine[]): ItemInput[] {
  return lines.map((line) => {
    const v = normalizeAmount(line.amount);
    return isRoll(line.unit_type) ? { product_uuid: line.product_uuid, meters: v } : { product_uuid: line.product_uuid, quantity: Number(v) };
  });
}

/** Adds a product once (the API refuses a product twice); a roll starts empty, a piece at 1. */
export function addLine(lines: CartLine[], product: { uuid: string; sku: string; name: string; unit_type: string }): CartLine[] {
  if (lines.some((l) => l.product_uuid === product.uuid)) return lines;
  return [
    ...lines,
    { product_uuid: product.uuid, sku: product.sku, name: product.name, unit_type: product.unit_type, amount: isRoll(product.unit_type) ? '' : '1' },
  ];
}
