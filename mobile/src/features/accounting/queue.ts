import type { components } from '@/core/api/schema';

type S = components['schemas'];
export type QueueSummary = S['AccountingQueueSummary'];
export type QueueReturn = S['AccountingQueueReturn'];
export type OrderInvoiceInput = S['OrderInvoiceInput'];
export type InvoiceStatus = S['OrderInvoiceStatus'];

/**
 * TEC-1071: the accounting queue (legacy olexfilms-warehouse "Muhasebe
 * kuyruğu", web TEC-1070). Received sales waiting for an invoice mark, sales on
 * the road (invoiceable once the buyer receives them) and received returns
 * waiting to be marked processed.
 */
export const QUEUE_TABS = ['orders', 'transit', 'returns'] as const;
export type QueueTab = (typeof QUEUE_TABS)[number];

/** Summary counter behind each tab. */
export function queueCount(summary: QueueSummary | undefined, tab: QueueTab): number | undefined {
  if (!summary) return undefined;
  if (tab === 'orders') return summary.orders_to_invoice;
  if (tab === 'transit') return summary.orders_in_transit;
  return summary.returns_pending;
}

/** GET /v1/orders filters of an order tab (the same lists the web queue reads). */
export function queueOrdersQuery(tab: Exclude<QueueTab, 'returns'>) {
  return tab === 'orders'
    ? { side: 'seller' as const, status: 'received', invoice_status: 'pending,partial' }
    : { side: 'seller' as const, status: 'shipped,delivered' };
}

/** Only the orders tab marks invoices, with einvoice.manage (TEC-1030). */
export function canMarkInvoiced(tab: QueueTab, canManageEinvoices: boolean) {
  return tab === 'orders' && canManageEinvoices;
}

export const QUEUE_NOTE_MAX = 1000;

/** Local calendar day as YYYY-MM-DD (the invoice date defaults to today). */
export function todayISO(now: Date = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${now.getFullYear()}-${p(now.getMonth() + 1)}-${p(now.getDate())}`;
}

/** A real calendar day written as YYYY-MM-DD. */
export function isISODate(v: string): boolean {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(v);
  if (!m) return false;
  const d = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])));
  return d.getUTCFullYear() === Number(m[1]) && d.getUTCMonth() === Number(m[2]) - 1 && d.getUTCDate() === Number(m[3]);
}

export type InvoiceForm = { invoice_no: string; invoice_date: string; note: string };
export type InvoiceErrors = { invoice_no?: 'required'; invoice_date?: 'date'; note?: 'too_long' };

export function validateInvoice(v: InvoiceForm): InvoiceErrors {
  const errors: InvoiceErrors = {};
  if (!v.invoice_no.trim()) errors.invoice_no = 'required';
  if (!isISODate(v.invoice_date.trim())) errors.invoice_date = 'date';
  if (v.note.length > QUEUE_NOTE_MAX) errors.note = 'too_long';
  return errors;
}

/** Body of POST /v1/orders/{uuid}/invoices: no item_uuids = every line not invoiced yet. */
export function invoiceInput(v: InvoiceForm): OrderInvoiceInput {
  const note = v.note.trim();
  return { invoice_no: v.invoice_no.trim(), invoice_date: v.invoice_date.trim(), ...(note ? { note } : {}) };
}

/** Body of POST .../processed: an optional note (e.g. the credit note number). */
export function processInput(note: string): { note?: string } {
  const n = note.trim();
  return n ? { note: n } : {};
}

export function invoiceStatusTone(s: InvoiceStatus): 'neutral' | 'progress' | 'done' {
  if (s === 'invoiced') return 'done';
  if (s === 'partial') return 'progress';
  return 'neutral';
}
