import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { splitTimeLeft } from './time-left.ts';

describe('splitTimeLeft', () => {
  it('never reports 12 months', () => {
    assert.deepEqual(splitTimeLeft(2553), { years: 6, months: 11 });
    assert.deepEqual(splitTimeLeft(364), { years: 0, months: 11 });
  });

  it('rolls over to whole years', () => {
    assert.deepEqual(splitTimeLeft(366), { years: 1, months: 0 });
    assert.deepEqual(splitTimeLeft(2557), { years: 7, months: 0 });
  });

  it('has no months under a month', () => {
    assert.deepEqual(splitTimeLeft(29), { years: 0, months: 0 });
  });
});
