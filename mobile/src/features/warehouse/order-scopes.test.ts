import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { orderScopeQuery } from './order-scopes.ts';

describe('orderScopeQuery (TEC-1224)', () => {
  it('lists delivered orders on the receive side', () => {
    assert.deepEqual(orderScopeQuery('receive', 'w-1'), { side: 'buyer', status: 'shipped,delivered,received' });
  });
  it('keeps the prepare side on the picked warehouse, with or without a status override', () => {
    assert.deepEqual(orderScopeQuery('prepare', 'w-1'), {
      side: 'seller',
      status: 'approved,processing,preparing,ready',
      sort: 'queue',
      seller_warehouse: 'w-1',
    });
    assert.deepEqual(orderScopeQuery('prepare', 'w-1', 'approved'), { side: 'seller', status: 'approved', sort: 'queue', seller_warehouse: 'w-1' });
    assert.deepEqual(orderScopeQuery('prepare'), { side: 'seller', status: 'approved,processing,preparing,ready', sort: 'queue' });
  });
});
