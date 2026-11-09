import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addLine,
  cartIsValid,
  type CartLine,
  lineError,
  MAX_ORDER_LINES,
  submitPreview,
  toItemInputs,
  validateCart,
} from './order-lines.ts';

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

test('unit price (center orders): optional, ≥ 0, up to 4 decimals, sent only with withPrice', () => {
  const base: CartLine = { product_uuid: 'p', sku: 'S', name: 'N', unit_type: 'piece', amount: '2' };
  assert.equal(lineError({ ...base, unit_price: '' }), null);
  assert.equal(lineError({ ...base, unit_price: '12,5' }), null);
  assert.equal(lineError({ ...base, unit_price: '0' }), null);
  assert.equal(lineError({ ...base, unit_price: '-1' }), 'price_invalid');
  assert.equal(lineError({ ...base, unit_price: '1.23456' }), 'price_invalid');
  assert.deepEqual(toItemInputs([{ ...base, unit_price: '12,5' }]), [{ product_uuid: 'p', quantity: 2 }]);
  assert.deepEqual(toItemInputs([{ ...base, unit_price: '12,5' }], true), [{ product_uuid: 'p', quantity: 2, unit_price: '12.5' }]);
  assert.deepEqual(toItemInputs([{ ...base, unit_price: ' ' }], true), [{ product_uuid: 'p', quantity: 2 }]);
});

test('plotter mark is sent on every line, gift lines included', () => {
  const base: CartLine = { product_uuid: 'p', sku: 'S', name: 'N', unit_type: 'piece', amount: '2', plotter_cut: true };
  assert.deepEqual(toItemInputs([{ ...base, is_gift: true, unit_price: '5' }], true, true), [
    { product_uuid: 'p', quantity: 2, plotter_cut: true, is_gift: true },
  ]);
  assert.deepEqual(toItemInputs([{ ...base, discount_pct: '10' }], true, true), [
    { product_uuid: 'p', quantity: 2, plotter_cut: true, discount_pct: '10' },
  ]);
});

test('submitPreview: amounts with their unit, capped with a remainder', () => {
  const lines = addLine(addLine([], piece), roll).map((l) => (l.unit_type === 'roll_meter' ? { ...l, amount: ' 12,5 ' } : l));
  assert.deepEqual(submitPreview(lines).rows, [
    { name: 'PPF', amount: '1', roll: false },
    { name: 'Roll', amount: '12,5', roll: true },
  ]);
  assert.equal(submitPreview(lines).more, 0);
  const many: CartLine[] = Array.from({ length: 11 }, (_, i) => ({ product_uuid: `p${i}`, sku: 's', name: `n${i}`, unit_type: 'piece', amount: '2' }));
  assert.equal(submitPreview(many, 8).rows.length, 8);
  assert.equal(submitPreview(many, 8).more, 3);
});
