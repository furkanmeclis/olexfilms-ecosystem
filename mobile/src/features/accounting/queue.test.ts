import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  canMarkInvoiced,
  invoiceInput,
  isISODate,
  processInput,
  queueCount,
  queueOrdersQuery,
  todayISO,
  validateInvoice,
} from './queue.ts';

describe('accounting queue', () => {
  it('reads the counter of each tab', () => {
    const s = { orders_to_invoice: 3, orders_in_transit: 5, returns_pending: 1 };
    assert.equal(queueCount(s, 'orders'), 3);
    assert.equal(queueCount(s, 'transit'), 5);
    assert.equal(queueCount(s, 'returns'), 1);
    assert.equal(queueCount(undefined, 'orders'), undefined);
  });

  it('lists the same orders as the web queue', () => {
    assert.deepEqual(queueOrdersQuery('orders'), { side: 'seller', status: 'received', invoice_status: 'pending,partial' });
    assert.deepEqual(queueOrdersQuery('transit'), { side: 'seller', status: 'shipped,delivered' });
  });

  it('marks invoices only on the orders tab with einvoice.manage', () => {
    assert.equal(canMarkInvoiced('orders', true), true);
    assert.equal(canMarkInvoiced('orders', false), false);
    assert.equal(canMarkInvoiced('transit', true), false);
    assert.equal(canMarkInvoiced('returns', true), false);
  });

  it('formats and checks calendar days', () => {
    assert.equal(todayISO(new Date(2026, 0, 5, 23, 30)), '2026-01-05');
    assert.equal(isISODate('2026-02-28'), true);
    assert.equal(isISODate('2026-02-30'), false);
    assert.equal(isISODate('28.02.2026'), false);
  });

  it('validates the invoice form', () => {
    assert.deepEqual(validateInvoice({ invoice_no: ' ', invoice_date: '2026-13-01', note: '' }), {
      invoice_no: 'required',
      invoice_date: 'date',
    });
    assert.deepEqual(validateInvoice({ invoice_no: 'A1', invoice_date: '2026-11-09', note: 'x'.repeat(1001) }), { note: 'too_long' });
    assert.deepEqual(validateInvoice({ invoice_no: 'A1', invoice_date: '2026-11-09', note: '' }), {});
  });

  it('builds the request bodies', () => {
    assert.deepEqual(invoiceInput({ invoice_no: ' GIB2026 ', invoice_date: '2026-11-09', note: '  ' }), {
      invoice_no: 'GIB2026',
      invoice_date: '2026-11-09',
    });
    assert.deepEqual(invoiceInput({ invoice_no: 'A', invoice_date: '2026-11-09', note: ' n ' }).note, 'n');
    assert.deepEqual(processInput('  '), {});
    assert.deepEqual(processInput(' CN-1 '), { note: 'CN-1' });
  });
});
