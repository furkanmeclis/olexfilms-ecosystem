/**
 * Order preparation rules of the assign scanner, free of React so node:test
 * runs them. No `@/` imports.
 */

export type AssignItem = {
  uuid: string;
  product: { uuid: string; unit_type: string };
  meters?: string | number | null;
  quantity?: number | null;
  assigned: string | number;
};

export const isRollItem = (i: AssignItem) => i.product.unit_type === 'roll_meter';
export const requiredOf = (i: AssignItem) => Number(isRollItem(i) ? (i.meters ?? 0) : (i.quantity ?? 0));
export const isCovered = (i: AssignItem) => Number(i.assigned) >= requiredOf(i) - 1e-9;

/** Every line has what it asks for (an order without lines is not complete). */
export function allCovered(items: readonly AssignItem[]): boolean {
  return items.length > 0 && items.every(isCovered);
}

/**
 * Line a scanned unit of `productUuid` goes to: the first open line of that
 * product. `not_on_order` when no line has the product, `covered` when every
 * line of it is already full (nothing is sent then).
 */
export function assignTarget<T extends AssignItem>(
  items: readonly T[],
  productUuid: string | null | undefined,
): { kind: 'line'; item: T } | { kind: 'not_on_order' } | { kind: 'covered'; item: T } {
  const lines = items.filter((i) => i.product.uuid === productUuid);
  if (!lines.length) return { kind: 'not_on_order' };
  const open = lines.find((i) => !isCovered(i));
  return open ? { kind: 'line', item: open } : { kind: 'covered', item: lines[0] };
}

/** Amount a line still needs (pieces or meters), never below 0. */
export const missingOf = (i: AssignItem) => Math.max(0, requiredOf(i) - Number(i.assigned));

/**
 * Quantity sent for a scanned fixed barcode (TEC-1048): what the user set on
 * the stepper, or, while the stepper is untouched, what the line still needs
 * capped at what the barcode holds. A fixed barcode line (e.g. 10 squeegees)
 * is then prepared with one scan instead of ten.
 */
export function fixedAssignQuantity(
  item: AssignItem,
  chosen: number,
  touched: boolean,
  onHand?: number | null,
): number {
  if (touched) return Math.max(1, Math.round(chosen));
  const need = Math.max(1, Math.ceil(missingOf(item)));
  return onHand != null && onHand > 0 ? Math.min(need, onHand) : need;
}

type FixedSuggestion = { unit_uuid: string; unit_kind: string; quantity: number | null };

/**
 * Fixed barcode suggestions of a line as apply input (TEC-1048: the product
 * code of a fixed-barcode product was scanned). With a cap (the stepper was
 * set) the quantities are cut down to it; serial and roll rows are dropped.
 */
export function fixedSuggestionUnits(
  suggestions: readonly FixedSuggestion[],
  cap: number | null,
): { unit_uuid: string; quantity: number }[] {
  let left = cap ?? Number.POSITIVE_INFINITY;
  const out: { unit_uuid: string; quantity: number }[] = [];
  for (const s of suggestions) {
    if (s.unit_kind !== 'fixed' || !s.quantity || left <= 0) continue;
    const quantity = Math.min(s.quantity, left);
    out.push({ unit_uuid: s.unit_uuid, quantity });
    left -= quantity;
  }
  return out;
}

/** A roll scanned with the meters box empty assigns the whole roll: asked first. */
export function asksWholeRoll(item: AssignItem, meters: string): boolean {
  return isRollItem(item) && meters.trim() === '';
}
