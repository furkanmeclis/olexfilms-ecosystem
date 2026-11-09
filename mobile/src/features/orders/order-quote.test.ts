import assert from 'node:assert/strict';
import { test } from 'node:test';

import { type CartLine, lineError, toItemInputs } from './order-lines.ts';
import { linePrice, type Quote, quoteTotals } from './order-quote.ts';

const quote: Quote = {
  currency: 'USD',
  first_order: true,
  discount_pct: '20.00',
  try_rate: '34.25',
  items: [
    { product_uuid: 'p1', list_unit_price: '100.0000', unit_price: '80.0000', price_source: 'list' },
    { product_uuid: 'r1', list_unit_price: '10.0000', unit_price: '8.0000', price_source: 'list' },
    { product_uuid: 'm1', list_unit_price: null, unit_price: null, price_source: 'missing' },
  ],
};

const line = (product_uuid: string, amount: string, extra: Partial<CartLine> = {}): CartLine => ({
  product_uuid,
  sku: product_uuid,
  name: product_uuid,
  unit_type: product_uuid.startsWith('r') ? 'roll_meter' : 'piece',
  amount,
  ...extra,
});

test('linePrice: default discount comes from the quote', () => {
  const p = linePrice(line('p1', '2'), quote);
  assert.deepEqual(p, { list: 100, discountPct: 20, unit: 80, total: 160, gift: false });
});

test('linePrice: a manual discount replaces the default on the list price', () => {
  const p = linePrice(line('r1', '12,5', { discount_pct: '35' }), quote);
  assert.equal(p.discountPct, 35);
  assert.equal(p.unit, 6.5);
  assert.equal(p.total, 81.25);
});

test('linePrice: a gift is charged 0, a manual price is charged as is', () => {
  assert.deepEqual(linePrice(line('p1', '3', { is_gift: true, discount_pct: '10' }), quote), {
    list: 100,
    discountPct: null,
    unit: 0,
    total: 0,
    gift: true,
  });
  const manual = linePrice(line('p1', '2', { unit_price: '55.5', discount_pct: '10' }), quote);
  assert.equal(manual.unit, 55.5);
  assert.equal(manual.discountPct, null);
});

test('linePrice: an invalid discount or amount leaves the total unknown or default', () => {
  assert.equal(linePrice(line('p1', '1', { discount_pct: '150' }), quote).unit, 80);
  assert.equal(linePrice(line('p1', '1.5'), quote).total, null);
  assert.equal(linePrice(line('m1', '1'), quote).unit, null);
});

test('quoteTotals: USD total, TRY at the current rate, incomplete flag', () => {
  const t = quoteTotals([line('p1', '2'), line('r1', '10', { is_gift: true })], quote);
  assert.deepEqual(t, { total: 160, totalTry: 5480, incomplete: false });
  const missing = quoteTotals([line('p1', '1'), line('m1', '1')], { ...quote, try_rate: null });
  assert.deepEqual(missing, { total: 80, totalTry: null, incomplete: true });
});

test('discount input: 0..100 with up to 2 decimals', () => {
  assert.equal(lineError(line('p1', '1', { discount_pct: '12,5' })), null);
  assert.equal(lineError(line('p1', '1', { discount_pct: '100' })), null);
  assert.equal(lineError(line('p1', '1', { discount_pct: '100.5' })), 'discount_invalid');
  assert.equal(lineError(line('p1', '1', { discount_pct: '1.234' })), 'discount_invalid');
});

test('toItemInputs: discount and gift only when allowed; a gift sends no price', () => {
  const lines = [line('p1', '2', { discount_pct: '15', unit_price: '9' }), line('r1', '3', { is_gift: true, unit_price: '4' })];
  assert.deepEqual(toItemInputs(lines, true), [
    { product_uuid: 'p1', quantity: 2, unit_price: '9' },
    { product_uuid: 'r1', meters: '3', unit_price: '4' },
  ]);
  assert.deepEqual(toItemInputs(lines, true, true), [
    { product_uuid: 'p1', quantity: 2, unit_price: '9', discount_pct: '15' },
    { product_uuid: 'r1', meters: '3', is_gift: true },
  ]);
});
