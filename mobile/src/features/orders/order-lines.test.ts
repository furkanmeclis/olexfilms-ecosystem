import assert from 'node:assert/strict';
import { test } from 'node:test';

import { addLine, cartIsValid, type CartLine, lineError, MAX_ORDER_LINES, toItemInputs, validateCart } from './order-lines.ts';

const piece = { uuid: 'p1', sku: 'PPF-1', name: 'PPF', unit_type: 'piece' };
const roll = { uuid: 'r1', sku: 'ROLL-1', name: 'Roll', unit_type: 'roll_meter' };

test('addLine: piece starts at 1, roll empty, no duplicates', () => {
  const a = addLine([], piece);
  assert.equal(a[0].amount, '1');
  const b = addLine(a, roll);
  assert.equal(b[1].amount, '');
  assert.equal(addLine(b, piece).length, 2);
});

test('lineError: pieces are whole numbers 1..1,000,000', () => {
  const l = (amount: string): CartLine => ({ product_uuid: 'p', sku: 's', name: 'n', unit_type: 'piece', amount });
  assert.equal(lineError(l('3')), null);
  assert.equal(lineError(l('0')), 'quantity_invalid');
  assert.equal(lineError(l('1.5')), 'quantity_invalid');
  assert.equal(lineError(l('1000001')), 'quantity_invalid');
});

test('lineError: rolls take meters with a decimal comma, at most two decimals', () => {
  const l = (amount: string): CartLine => ({ product_uuid: 'r', sku: 's', name: 'n', unit_type: 'roll_meter', amount });
  assert.equal(lineError(l('12,5')), null);
  assert.equal(lineError(l('0')), 'meters_invalid');
  assert.equal(lineError(l('1.234')), 'meters_invalid');
  assert.equal(lineError(l('')), 'meters_invalid');
});

test('validateCart: empty and too many lines', () => {
  assert.equal(validateCart([]).form, 'empty');
  const many = Array.from({ length: MAX_ORDER_LINES + 1 }, (_, i) => ({ product_uuid: `p${i}`, sku: 's', name: 'n', unit_type: 'piece', amount: '1' }));
  assert.equal(validateCart(many).form, 'too_many');
  assert.equal(cartIsValid(validateCart(addLine([], piece))), true);
});

test('toItemInputs: quantity for pieces, normalized meters for rolls', () => {
  const lines = addLine(addLine([], piece), roll).map((l) => (l.unit_type === 'roll_meter' ? { ...l, amount: ' 7,25 ' } : { ...l, amount: '4' }));
  assert.deepEqual(toItemInputs(lines), [
    { product_uuid: 'p1', quantity: 4 },
    { product_uuid: 'r1', meters: '7.25' },
  ]);
});
