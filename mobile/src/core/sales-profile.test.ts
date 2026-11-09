import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { isSalesProfile } from './sales-profile.ts';

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
