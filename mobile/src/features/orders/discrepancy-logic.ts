import type { components } from '@/core/api/schema';

type S = components['schemas'];
type Order = S['Order'];

/** TEC-1227: receipt state of one shipped unit. */
export type ReceiptMark = { kind: 'ok' | 'missing' | 'damaged'; reason: string };

/** The shipped units of an order the receipt check lists. */
export function shippedUnits(o: Pick<Order, 'items'>): { unitUuid: string; barcode: string; product: string }[] {
  return (o.items ?? []).flatMap((it) =>
    (it.units ?? []).map((u) => ({ unitUuid: u.unit_uuid, barcode: u.barcode, product: it.product.name })),
  );
}

/** The discrepancies sent with the received step (reported units only). */
export function receiptDiscrepancies(marks: Record<string, ReceiptMark>): S['OrderDiscrepancyInput'][] {
  return Object.entries(marks)
    .filter(([, m]) => m.kind !== 'ok')
    .map(([unit, m]) => ({ unit_uuid: unit, kind: m.kind as 'missing' | 'damaged', reason: m.reason.trim() }));
}

/** Every reported unit has a reason and at least one unit is received. */
export function receiptValid(o: Pick<Order, 'items'>, marks: Record<string, ReceiptMark>): boolean {
  const list = receiptDiscrepancies(marks);
  const units = shippedUnits(o).length;
  const missing = list.filter((d) => d.kind === 'missing').length;
  return list.every((d) => d.reason.length > 0) && (units === 0 || missing < units);
}

/** TEC-1227: the lines a partial shipment moves to the backorder. */
export function shortLines(o: Pick<Order, 'items'>): { name: string; picked: number; rest: number; belowMinCut?: string }[] {
  return (o.items ?? [])
    .map((it) => {
      const want = it.meters != null ? Number(it.meters) : (it.quantity ?? 0);
      const picked = Number(it.assigned) || 0;
      const rest = want - picked;
      // TEC-1241: a meter rest shorter than the product's minimum cut is warned about.
      const min = it.min_cut_meters ? Number(it.min_cut_meters) : 0;
      const below = it.sale_measure === 'meter' && min > 0 && rest > 1e-9 && rest < min - 1e-9;
      return { name: it.product.name, picked, rest, ...(below ? { belowMinCut: it.min_cut_meters ?? '' } : {}) };
    })
    .filter((l) => l.rest > 1e-9);
}

/** A preparing order can ship its picked part: something picked, something short. */
export function canPartialShip(o: Pick<Order, 'items' | 'can_partial_ship'> | undefined): boolean {
  if (!o?.can_partial_ship) return false;
  const anyPicked = (o.items ?? []).some((it) => Number(it.assigned) > 0);
  return anyPicked && shortLines(o).length > 0;
}

/** TEC-1227: the transfer completion body: unticked lines are missing. */
export function transferMissing(lines: { uuid: string }[], arrived: Set<string>): { line_uuid: string }[] {
  return lines.filter((l) => !arrived.has(l.uuid)).map((l) => ({ line_uuid: l.uuid }));
}
