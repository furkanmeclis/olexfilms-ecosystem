import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { parseMeters } from './split-meters.ts';

describe('parseMeters (TEC-1230)', () => {
  it('accepts positive meters with a dot or comma and two decimals', () => {
    assert.equal(parseMeters('12'), '12');
    assert.equal(parseMeters(' 2,5 '), '2.5');
    assert.equal(parseMeters('0.75'), '0.75');
  });
  it('refuses zero, negatives and too many decimals', () => {
    for (const bad of ['', '0', '-1', '1.234', 'abc', '1.']) assert.equal(parseMeters(bad), null);
  });
});
