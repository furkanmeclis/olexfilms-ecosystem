import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { stripNulls } from './native-props.ts';

describe('stripNulls', () => {
  it('iç içe nesne ve dizilerdeki null ve undefined alanları atar', () => {
    assert.deepEqual(
      stripNulls({ a: null, b: undefined, c: 'x', d: { e: null, f: 0, g: false, h: '' }, i: [null, { j: null, k: 1 }, undefined, 2] }),
      { c: 'x', d: { f: 0, g: false, h: '' }, i: [{ k: 1 }, 2] },
    );
  });

  it('girdiyi değiştirmez', () => {
    const input = { a: null, b: { c: null } };
    stripNulls(input);
    assert.deepEqual(input, { a: null, b: { c: null } });
  });

  it('nesne olmayan değerler aynen döner', () => {
    assert.equal(stripNulls('x'), 'x');
    assert.equal(stripNulls(null), null);
  });
});
