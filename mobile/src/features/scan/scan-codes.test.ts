import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { classifyCode, scanAction } from './scan-codes.ts';

describe('scan action', () => {
  it('opens the stock unit screen of the space with stock.read', () => {
    assert.deepEqual(scanAction(classifyCode('OFW:UNIT:PPF 01'), { space: 'bayi', canStock: true }), {
      kind: 'unit',
      path: '/bayi/menu/stock/unit/PPF%2001',
    });
    assert.deepEqual(scanAction(classifyCode('ABC-1234'), { space: 'admin', canStock: true }), {
      kind: 'unit',
      path: '/admin/menu/stock/unit/ABC-1234',
    });
    assert.equal(scanAction(classifyCode('ABC-1234'), { space: 'bayi', canStock: false }), null);
  });

  it('opens a warranty by its public code', () => {
    assert.deepEqual(scanAction(classifyCode('https://olexfilms.com/warranty/W12345'), { space: 'bayi', canStock: false }), {
      kind: 'warranty',
      code: 'W12345',
    });
  });

  it('has nothing for locations and unknown codes', () => {
    assert.equal(scanAction(classifyCode('OFW:LOC:A-01'), { space: 'bayi', canStock: true }), null);
    assert.equal(scanAction(classifyCode('hello world!'), { space: 'bayi', canStock: true }), null);
  });
});
