import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  actionKey,
  creatorOf,
  liveTryAmount,
  newestFirst,
  orderAmounts,
  sellerWarehouseFilter,
  stockByProduct,
  stockState,
} from './order-summary.ts';

test('actionKey: statuses read as the status, steps as step.*, unknown is null', () => {
  assert.equal(actionKey('preparing'), 'status.preparing');
  assert.equal(actionKey('created'), 'step.created');
  assert.equal(actionKey('unit_assigned'), 'step.unit_assigned');
  assert.equal(actionKey('updated'), 'step.updated');
  assert.equal(actionKey('something_new'), null);
});

test('orderAmounts: pieces and meters only when present', () => {
  assert.deepEqual(orderAmounts({ total_quantity: 3, total_meters: null }), { pieces: 3, meters: null });
  assert.deepEqual(orderAmounts({ total_quantity: 0, total_meters: '12.50' }), { pieces: null, meters: 12.5 });
  assert.deepEqual(orderAmounts({ total_quantity: 2, total_meters: '4.00' }), { pieces: 2, meters: 4 });
  assert.deepEqual(orderAmounts({ total_quantity: 0, total_meters: '' }), { pieces: null, meters: null });
});

test('liveTryAmount: only a foreign currency with a rate', () => {
  assert.equal(liveTryAmount({ currency: 'USD', live_total_try: '4200.00' }), '4200.00');
  assert.equal(liveTryAmount({ currency: 'USD', live_total_try: null }), null);
  assert.equal(liveTryAmount({ currency: 'TRY', live_total_try: '100.00' }), null);
});

test('creatorOf and newestFirst', () => {
  const tl = [
    { action: 'created', at: '2026-11-01T10:00:00Z', user: { uuid: 'u1', name: 'Zorbey' } },
    { action: 'preparing', at: '2026-11-02T10:00:00Z', user: { uuid: 'u2', name: 'Melike' } },
  ];
  assert.deepEqual(creatorOf(tl), { uuid: 'u1', name: 'Zorbey' });
  assert.equal(creatorOf([]), null);
  assert.equal(creatorOf(undefined), null);
  assert.deepEqual(
    newestFirst(tl).map((s) => s.action),
    ['preparing', 'created'],
  );
});

test('sellerWarehouseFilter', () => {
  assert.deepEqual(sellerWarehouseFilter('wh-1'), { seller_warehouse: 'wh-1' });
  assert.deepEqual(sellerWarehouseFilter(undefined), {});
});

test('stockState: in, out, unknown', () => {
  const map = stockByProduct([
    { product: { uuid: 'p1' }, quantity: 4, meters: '0' },
    { product: { uuid: 'p2' }, quantity: 0, meters: '25.5' },
    { product: { uuid: 'p3' }, quantity: 0, meters: '0' },
  ]);
  assert.deepEqual(stockState(map, 'p1', true), { kind: 'in', quantity: 4, meters: 0 });
  assert.deepEqual(stockState(map, 'p2', false), { kind: 'in', quantity: 0, meters: 25.5 });
  assert.deepEqual(stockState(map, 'p3', false), { kind: 'out' });
  assert.deepEqual(stockState(map, 'p9', true), { kind: 'out' });
  assert.deepEqual(stockState(map, 'p9', false), { kind: 'unknown' });
  assert.deepEqual(stockState(undefined, 'p1', true), { kind: 'unknown' });
});
