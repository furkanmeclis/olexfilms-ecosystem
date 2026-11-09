import assert from 'node:assert/strict';
import { test } from 'node:test';

import { hasAmount, pricesVisible } from './price-visibility.ts';

const base = { permissions: [] as string[], superAdmin: false, primarySpace: 'bayi', pathname: '/bayi/menu/orders' };

test('TEC-1051: no prices anywhere in the warehouse space, even for the owner', () => {
  assert.equal(pricesVisible({ ...base, pathname: '/depo/menu/orders/1' }), false);
  assert.equal(pricesVisible({ ...base, pathname: '/depo' }), false);
  assert.equal(pricesVisible({ ...base, superAdmin: true, pathname: '/depo/jobs/order/1' }), false);
  assert.equal(pricesVisible({ ...base, permissions: ['pricing.purchase.read'], pathname: '/depo/stock' }), false);
});

test('TEC-1051: a warehouse role without pricing permissions sees no prices in other spaces', () => {
  assert.equal(pricesVisible({ ...base, primarySpace: 'depo' }), false);
  assert.equal(pricesVisible({ ...base, primarySpace: 'depo', permissions: ['pricing.sale.read'] }), true);
  assert.equal(pricesVisible({ ...base, primarySpace: 'depo', superAdmin: true }), true);
});

test('other users keep their prices outside the warehouse space', () => {
  assert.equal(pricesVisible(base), true);
  assert.equal(pricesVisible({ ...base, primarySpace: 'admin', pathname: '/admin/menu/orders' }), true);
  // "/depots" is not the warehouse space.
  assert.equal(pricesVisible({ ...base, pathname: '/depots' }), true);
});

test('hasAmount: null, empty and NaN are not shown', () => {
  assert.equal(hasAmount(null), false);
  assert.equal(hasAmount(undefined), false);
  assert.equal(hasAmount(''), false);
  assert.equal(hasAmount('abc'), false);
  assert.equal(hasAmount('0.00'), true);
  assert.equal(hasAmount(12), true);
});
