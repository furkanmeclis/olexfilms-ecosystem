import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { isSalesProfile, workplaceTabs } from './sales-profile.ts';

describe('isSalesProfile (TEC-1068)', () => {
  it('is a center account that orders for the dealer network', () => {
    assert.equal(isSalesProfile({ orgType: 'center', permissions: ['orders.write', 'organizations.read', 'services.write'] }), true);
  });
  it('leaves the service point, warehouse and accounting roles alone', () => {
    assert.equal(isSalesProfile({ orgType: 'center', permissions: ['services.write', 'orders.read'] }), false);
    assert.equal(isSalesProfile({ orgType: 'center', permissions: ['orders.read', 'organizations.read', 'warehouse.read'] }), false);
  });
  it('never applies to dealers or distributors', () => {
    assert.equal(isSalesProfile({ orgType: 'dealer', permissions: ['orders.write', 'organizations.read'] }), false);
    assert.equal(isSalesProfile({ orgType: 'distributor', permissions: ['orders.write', 'organizations.read'] }), false);
    assert.equal(isSalesProfile({ orgType: undefined, permissions: [] }), false);
  });
});

describe('workplaceTabs (TEC-1222)', () => {
  const of = (list: string[]) => (p: string) => list.includes(p);
  const owner = ['orders.write', 'organizations.read', 'services.read', 'services.write', 'stock.read'];
  const sales = ['orders.write', 'organizations.read', 'services.read'];
  it('gives center_owner and center_staff both Orders and Measure', () => {
    assert.deepEqual(workplaceTabs({ sales: true, can: of(owner) }), { orders: true, measure: true, stock: false });
  });
  it('leaves the center_sales bar without Measure', () => {
    assert.deepEqual(workplaceTabs({ sales: true, can: of(sales) }), { orders: true, measure: false, stock: false });
  });
  it('keeps the dealer bar: Measure for service writers, Stock otherwise', () => {
    assert.deepEqual(workplaceTabs({ sales: false, can: of(['services.write', 'stock.read']) }), { orders: false, measure: true, stock: false });
    assert.deepEqual(workplaceTabs({ sales: false, can: of(['stock.read']) }), { orders: false, measure: false, stock: true });
  });
});
