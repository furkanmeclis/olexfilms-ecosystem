import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  buildCreateBody,
  lineFromOrderUnit,
  lineQuantityValid,
  orderReturnCreate,
  toggleLine,
  validQuantity,
  type ReturnOrderUnit,
} from './returns.ts';

const unit = (over: Partial<ReturnOrderUnit> = {}): ReturnOrderUnit => ({
  unit_uuid: 'u1',
  barcode: 'OLX-1',
  unit_kind: 'serial',
  product: { uuid: 'p1', sku: 'PPF', name: 'PPF Rulo', unit_type: 'roll' } as ReturnOrderUnit['product'],
  quantity: null,
  meters: '15.00',
  returnable: 1,
  ...over,
});

test('validQuantity: empty or a whole number 1..9,999,999', () => {
  assert.equal(validQuantity(''), true);
  assert.equal(validQuantity(' 12 '), true);
  assert.equal(validQuantity('0'), false);
  assert.equal(validQuantity('1.5'), false);
  assert.equal(validQuantity('12345678'), false);
});

test('an order unit becomes a whole serial line or a fixed line capped at what is returnable', () => {
  assert.deepEqual(lineFromOrderUnit(unit()), { barcode: 'OLX-1', quantity: '', fixed: false, label: 'PPF Rulo' });
  assert.deepEqual(lineFromOrderUnit(unit({ barcode: 'FIX-9', unit_kind: 'fixed', quantity: 10, returnable: 4 })), {
    barcode: 'FIX-9',
    quantity: '1',
    fixed: true,
    max: 4,
    label: 'PPF Rulo',
  });
});

test('toggleLine adds a new barcode and removes one already listed (picked or scanned)', () => {
  const picked = lineFromOrderUnit(unit());
  const one = toggleLine([], picked);
  assert.equal(one.length, 1);
  assert.deepEqual(toggleLine(one, picked), []);
  assert.deepEqual(toggleLine([{ barcode: 'OLX-1', quantity: '' }], picked), []);
});

test('line quantities: serial none, fixed 1..max, scanned validQuantity', () => {
  assert.equal(lineQuantityValid({ barcode: 'a', quantity: '', fixed: false }), true);
  assert.equal(lineQuantityValid({ barcode: 'a', quantity: '2', fixed: false }), false);
  assert.equal(lineQuantityValid({ barcode: 'a', quantity: '4', fixed: true, max: 4 }), true);
  assert.equal(lineQuantityValid({ barcode: 'a', quantity: '5', fixed: true, max: 4 }), false);
  assert.equal(lineQuantityValid({ barcode: 'a', quantity: '', fixed: true, max: 4 }), false);
  assert.equal(lineQuantityValid({ barcode: 'a', quantity: '' }), true);
});

test('a return carries the child and the order; a sibling transfer never does', () => {
  const lines = [
    { barcode: 'OLX-1', quantity: '' },
    { barcode: 'FIX-9', quantity: ' 2 ', fixed: true, max: 4 },
  ];
  assert.deepEqual(buildCreateBody('parent', lines, '  not ', 'return', { fromOrgUuid: 'child', orderUuid: 'order' }), {
    kind: 'return',
    from_org_uuid: 'child',
    order_uuid: 'order',
    to_org_uuid: 'parent',
    note: 'not',
    items: [{ barcode: 'OLX-1' }, { barcode: 'FIX-9', quantity: 2 }],
  });
  assert.deepEqual(buildCreateBody('sib', lines, '', 'sibling', { fromOrgUuid: 'child', orderUuid: 'order' }), {
    kind: 'sibling',
    to_org_uuid: 'sib',
    items: [{ barcode: 'OLX-1' }, { barcode: 'FIX-9', quantity: 2 }],
  });
  assert.equal(buildCreateBody(null, lines, '', 'return'), null);
  assert.equal(buildCreateBody('parent', [], '', 'return'), null);
  assert.equal(buildCreateBody('parent', [{ barcode: 'x', quantity: '9', fixed: true, max: 1 }], '', 'return'), null);
});

test('order page: the seller opens the return for the buyer, the buyer its own; closed orders and observers none', () => {
  const order = { uuid: 'o1', status: 'received', role: 'seller', buyer: { uuid: 'b1' } };
  const both = { request: true, approve: true };
  assert.deepEqual(orderReturnCreate(order, both), { from: 'b1', order: 'o1' });
  assert.equal(orderReturnCreate(order, { request: true, approve: false }), null);
  assert.deepEqual(orderReturnCreate({ ...order, role: 'buyer' }, both), { order: 'o1' });
  assert.equal(orderReturnCreate({ ...order, role: 'buyer' }, { request: false, approve: true }), null);
  assert.equal(orderReturnCreate({ ...order, role: 'observer' }, both), null);
  for (const status of ['draft', 'cancelling', 'cancelled']) assert.equal(orderReturnCreate({ ...order, status }, both), null);
});
