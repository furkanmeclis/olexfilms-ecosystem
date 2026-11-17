import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { canStartSupplement, orderSubtitle, supplementParentOf } from './supplement.ts';

const shipped = {
  uuid: 'o1',
  order_no: 'ORD-1',
  buyer: { uuid: 'd1', name: 'Bayi' },
  seller_warehouse: { uuid: 'w1' },
  can_add_supplement: true,
};

describe('supplementParentOf (TEC-1222)', () => {
  it('copies the dealer and the warehouse of the main order', () => {
    assert.deepEqual(supplementParentOf(shipped), {
      uuid: 'o1',
      order_no: 'ORD-1',
      buyer: { uuid: 'd1', name: 'Bayi' },
      warehouse_uuid: 'w1',
    });
    assert.equal(supplementParentOf({ ...shipped, seller_warehouse: null })?.warehouse_uuid, null);
  });
  it('is null when the server does not allow a supplement', () => {
    assert.equal(supplementParentOf({ ...shipped, can_add_supplement: false }), null);
    assert.equal(supplementParentOf(undefined), null);
  });
});

describe('canStartSupplement (TEC-1222)', () => {
  it('needs the workplace and the right to order', () => {
    assert.equal(canStartSupplement(shipped, { pathname: '/bayi/menu/orders/o1', canOrder: true }), true);
    assert.equal(canStartSupplement(shipped, { pathname: '/depo/menu/orders/o1', canOrder: true }), false);
    assert.equal(canStartSupplement(shipped, { pathname: '/bayi/orders/o1', canOrder: false }), false);
    assert.equal(canStartSupplement({ ...shipped, can_add_supplement: false }, { pathname: '/bayi/orders/o1', canOrder: true }), false);
  });
});

describe('orderSubtitle (TEC-1222)', () => {
  it('marks a supplementary order in the list', () => {
    assert.equal(orderSubtitle({ order_no: 'ORD-2', is_supplement: true }, { supplement: 'Ek gönderim', warehouse: 'Depo: A' }), 'ORD-2 · Ek gönderim · Depo: A');
    assert.equal(orderSubtitle({ order_no: 'ORD-1', is_supplement: false }, { supplement: 'Ek gönderim' }), 'ORD-1');
  });
});
