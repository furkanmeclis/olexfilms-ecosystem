import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  addLine,
  canVoidSale,
  cartTotals,
  type CartLine,
  isBelowCost,
  lineFromLookup,
  normalizeMoney,
  profitOf,
  purchaseErrors,
  saleErrors,
  unitBarcode,
} from './lib.ts';

const line = (patch: Partial<CartLine> = {}): CartLine => ({
  barcode: 'B1',
  productUuid: 'p1',
  name: 'Film',
  sku: 'SKU',
  unitKind: 'serial',
  available: 1,
  quantity: 1,
  unitPrice: '100.00',
  purchasePrice: '60.00',
  currency: 'TRY',
  ...patch,
});

describe('money', () => {
  it('normalizes typed prices', () => {
    assert.equal(normalizeMoney('12,5'), '12.50');
    assert.equal(normalizeMoney(' 1200 '), '1200.00');
    assert.equal(normalizeMoney('1.200,5'), null);
    assert.equal(normalizeMoney('abc'), null);
    assert.equal(normalizeMoney('1.234'), null);
  });
  it('compares with the purchase price', () => {
    assert.equal(isBelowCost('50.00', '60.00'), true);
    assert.equal(isBelowCost('60.00', '60.00'), false);
    assert.equal(isBelowCost('50.00', null), false);
    assert.equal(profitOf('100.00', '60.50'), '39.50');
    assert.equal(profitOf('50.00', '60.00'), '-10.00');
    assert.equal(profitOf(null, '60.00'), null);
  });
});

describe('unitBarcode', () => {
  it('strips the unit label prefix', () => {
    assert.equal(unitBarcode('OFW:UNIT:ABC123'), 'ABC123');
    assert.equal(unitBarcode(' ABC123 '), 'ABC123');
  });
});

describe('cart', () => {
  it('adds a barcode once', () => {
    const first = addLine([], line());
    assert.equal(first.added, true);
    const again = addLine(first.lines, line());
    assert.equal(again.added, false);
    assert.equal(again.lines.length, 1);
  });
  it('takes the dealer price, else the recommended one', () => {
    const base = {
      product_uuid: 'p',
      sku: 's',
      name: 'n',
      barcode: 'b',
      unit_kind: 'fixed',
      quantity_on_hand: 4,
      currency: 'TRY',
      purchase_price: null,
    };
    assert.equal(lineFromLookup({ ...base, sale_price: '10.00', recommended_sale_price: '12.00' }).unitPrice, '10.00');
    assert.equal(lineFromLookup({ ...base, sale_price: null, recommended_sale_price: '12.00' }).unitPrice, '12.00');
    assert.equal(lineFromLookup({ ...base, sale_price: null, recommended_sale_price: null }).available, 4);
  });
  it('sums totals and profit', () => {
    const t = cartTotals([line({ quantity: 2 }), line({ barcode: 'B2', purchasePrice: null, unitPrice: '10,5' })]);
    assert.equal(t.total, '210.50');
    assert.equal(t.profit, '80.00');
    assert.equal(t.profitIncomplete, true);
    assert.equal(t.invalid, false);
    assert.equal(cartTotals([line({ unitPrice: '' })]).invalid, true);
  });
});

describe('validation', () => {
  it('checks a sale', () => {
    assert.deepEqual(saleErrors([], 'cash', null), { lines: 'lines' });
    assert.deepEqual(saleErrors([line({ unitPrice: 'x' })], 'cash', null), { lines: 'price' });
    assert.deepEqual(saleErrors([line()], 'cari', null), { customer: 'cari_customer' });
    assert.deepEqual(saleErrors([line()], 'cari', 'c1'), {});
  });
  it('checks a purchase', () => {
    assert.deepEqual(purchaseErrors('', '0'), { supplier: 'supplier', amount: 'amount' });
    assert.deepEqual(purchaseErrors('s', '12,5'), {});
  });
});

describe('canVoidSale', () => {
  it('allows the sale day only, once', () => {
    const now = new Date(2026, 9, 23, 15, 0);
    assert.equal(canVoidSale({ sold_at: new Date(2026, 9, 23, 9, 0).toISOString(), voided: false }, now), true);
    assert.equal(canVoidSale({ sold_at: new Date(2026, 9, 22, 23, 0).toISOString(), voided: false }, now), false);
    assert.equal(canVoidSale({ sold_at: new Date(2026, 9, 23, 9, 0).toISOString(), voided: true }, now), false);
  });
});
