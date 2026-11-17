/**
 * TEC-1220 (web TEC-1085): pure rules of the return form and the order page's
 * returns section, kept free of React so node:test covers them. They follow the
 * web's `lib/transfers.ts`, `return-order-picker.tsx` and `order-returns-card.tsx`.
 */
import type { components } from '@/core/api/schema';

type S = components['schemas'];
export type ReturnOrderUnit = S['StockReturnOrderUnit'];

/** One scanned or picked unit of the request form. */
export type TransferLine = {
  barcode: string;
  /** Fixed barcodes only; empty moves the whole unit. */
  quantity: string;
  /**
   * Set for a unit picked from an order: true for a fixed barcode (1..max is
   * required), false for a serial piece or roll (it moves whole, no quantity).
   * Unset for a scanned barcode.
   */
  fixed?: boolean;
  /** What may still be returned of a picked fixed barcode. */
  max?: number;
  /** Product name of a picked unit. */
  label?: string;
};

/** Fixed-barcode quantity: empty moves the whole unit, else a whole number 1..9,999,999 (web validQuantity). */
export function validQuantity(raw: string): boolean {
  const v = raw.trim();
  return v === '' || /^[1-9][0-9]{0,6}$/.test(v);
}

/** Line of an order unit (web orderUnitPickable + lineFromUnit): a fixed barcode starts at 1, capped at what is returnable. */
export function lineFromOrderUnit(u: ReturnOrderUnit): TransferLine {
  const fixed = u.unit_kind === 'fixed';
  return {
    barcode: u.barcode.trim(),
    quantity: fixed ? '1' : '',
    fixed,
    ...(fixed ? { max: Number(u.returnable) } : {}),
    label: u.product.name,
  };
}

/** Picks or unpicks a line: a barcode already on the list (picked or scanned) is removed, any other is added. */
export function toggleLine(lines: TransferLine[], line: TransferLine): TransferLine[] {
  if (line.barcode === '') return lines;
  if (lines.some((l) => l.barcode === line.barcode)) return lines.filter((l) => l.barcode !== line.barcode);
  return [...lines, line];
}

/** Quantity check of one line: a picked serial unit takes none, a picked fixed barcode 1..max, a scanned line validQuantity. */
export function lineQuantityValid(l: TransferLine): boolean {
  if (l.fixed === false) return l.quantity.trim() === '';
  if (l.fixed === true) {
    const v = l.quantity.trim();
    if (v === '' || !validQuantity(v)) return false;
    return l.max === undefined || Number(v) <= l.max;
  }
  return validQuantity(l.quantity);
}

/** Request body (web buildCreateBody); null while the target or the units are missing or a quantity is wrong. */
export function buildCreateBody(
  toOrgUuid: string | null,
  lines: TransferLine[],
  note: string,
  kind: S['StockTransferKind'],
  link: { fromOrgUuid?: string; orderUuid?: string } = {},
): S['StockTransferCreateInput'] | null {
  if (!toOrgUuid || lines.length === 0 || !lines.every(lineQuantityValid)) return null;
  const isReturn = kind === 'return';
  const n = note.trim();
  return {
    kind,
    // TEC-1085: a return opened for a child and / or linked to an order.
    ...(isReturn && link.fromOrgUuid ? { from_org_uuid: link.fromOrgUuid } : {}),
    ...(isReturn && link.orderUuid ? { order_uuid: link.orderUuid } : {}),
    to_org_uuid: toOrgUuid,
    ...(n ? { note: n } : {}),
    items: lines.map((l) => (l.quantity.trim() ? { barcode: l.barcode, quantity: Number(l.quantity.trim()) } : { barcode: l.barcode })),
  };
}

/** Order statuses a return cannot refer to (the server answers TRANSFER_ORDER_MISMATCH). */
export const CLOSED_ORDER_STATUSES = ['draft', 'cancelling', 'cancelled'];

/** The order fields the returns section needs. */
export type OrderReturnsTarget = { uuid: string; status: string; role: string; buyer: { uuid: string } };

/**
 * "Create return" of the order page (web OrderReturnsCard): the seller with
 * transfers.approve opens it in the buyer's name, the buyer with
 * transfers.request its own return to the seller; both preselect the order.
 * Null for a closed order, an observer or without the matching permission.
 */
export function orderReturnCreate(
  order: OrderReturnsTarget,
  can: { request: boolean; approve: boolean },
): { from?: string; order: string } | null {
  if (CLOSED_ORDER_STATUSES.includes(order.status)) return null;
  if (order.role === 'seller' && can.approve) return { from: order.buyer.uuid, order: order.uuid };
  if (order.role === 'buyer' && can.request) return { order: order.uuid };
  return null;
}
