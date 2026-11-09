import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { dealersWithoutOrders, isSale, periodStart, topBuyers, topOpeners } from './lib.ts';

const now = new Date('2026-11-09T10:00:00Z');

describe('periodStart', () => {
  it('counts today in the day periods', () => {
    assert.equal(periodStart('7d', now), '2026-11-03');
    assert.equal(periodStart('30d', now), '2026-10-11');
    assert.equal(periodStart('90d', now), '2026-08-12');
  });
  it('covers the last twelve months', () => {
    assert.equal(periodStart('12m', now), '2025-11-10');
  });
});

const a = { uuid: 'a', name: 'Antalya Bayi' };
const b = { uuid: 'b', name: 'Bursa Bayi' };
const c = { uuid: 'c', name: 'Çorlu Bayi' };

describe('topBuyers', () => {
  const orders = [
    { status: 'approved', buyer: b, total: '100.50', currency: 'TRY' },
    { status: 'received', buyer: b, total: '50', currency: 'TRY' },
    { status: 'shipped', buyer: a, total: '10', currency: 'USD' },
    { status: 'draft', buyer: c, total: '999', currency: 'TRY' },
    { status: 'cancelled', buyer: a, total: '999', currency: 'USD' },
  ];
  it('ranks dealers by sales, drafts and cancellations left out', () => {
    const rows = topBuyers(orders);
    assert.deepEqual(
      rows.map((r) => [r.uuid, r.count, r.totals]),
      [
        ['b', 2, { TRY: 150.5 }],
        ['a', 1, { USD: 10 }],
      ],
    );
  });
  it('keeps no amount the API hid', () => {
    const [row] = topBuyers([{ status: 'approved', buyer: a, total: null, currency: 'TRY' }]);
    assert.deepEqual(row.totals, {});
  });
  it('cuts at the limit', () => {
    assert.equal(topBuyers(orders, 1).length, 1);
  });
});

describe('dealersWithoutOrders', () => {
  it('lists the active dealers without a sale', () => {
    const dealers = [
      { ...c, status: 'active' },
      { ...a, status: 'active' },
      { ...b, status: 'active' },
      { uuid: 'd', name: 'Denizli', status: 'suspended' },
    ];
    const orders = [
      { status: 'approved', buyer: b },
      { status: 'draft', buyer: a },
    ];
    assert.deepEqual(
      dealersWithoutOrders(dealers, orders).map((d) => d.uuid),
      ['a', 'c'],
    );
  });
  it('treats orders as sales by status', () => {
    assert.equal(isSale({ status: 'cancelling' }), false);
    assert.equal(isSale({ status: 'submitted' }), true);
  });
});

describe('topOpeners', () => {
  it('counts the dealers that opened services, not the center', () => {
    const rows = topOpeners([
      { status: 'completed', organization: { ...a, type: 'dealer' } },
      { status: 'processing', organization: { ...a, type: 'dealer' } },
      { status: 'cancelled', organization: { ...b, type: 'dealer' } },
      { status: 'completed', organization: { uuid: 'x', name: 'Merkez', type: 'center' } },
      { status: 'draft', organization: { ...c, type: 'dealer' } },
    ]);
    assert.deepEqual(
      rows.map((r) => [r.uuid, r.count]),
      [
        ['a', 2],
        ['c', 1],
      ],
    );
  });
});
