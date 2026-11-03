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

/** A roll scanned with the meters box empty assigns the whole roll: asked first. */
export function asksWholeRoll(item: AssignItem, meters: string): boolean {
  return isRollItem(item) && meters.trim() === '';
}
