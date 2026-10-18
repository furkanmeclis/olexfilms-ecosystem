import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { deviationTone, formatDeviation, num } from './lib.ts';

describe('num', () => {
  it('parses decimals and rejects empty values', () => {
    assert.equal(num('12.50'), 12.5);
    assert.equal(num(''), null);
    assert.equal(num(null), null);
    assert.equal(num('abc'), null);
  });
});

describe('formatDeviation', () => {
  it('signs the change and keeps one decimal', () => {
    assert.equal(formatDeviation('12.56', 'en'), '+12.6%');
    assert.equal(formatDeviation('-3', 'en'), '-3%');
    assert.equal(formatDeviation('0', 'en'), '0%');
    assert.equal(formatDeviation(null, 'en'), '–');
  });
});

describe('deviationTone', () => {
  it('is a problem at the threshold either way, a watch at half of it', () => {
    assert.equal(deviationTone('15', 15), 'problem');
    assert.equal(deviationTone('-20', 15), 'problem');
    assert.equal(deviationTone('8', 15), 'progress');
    assert.equal(deviationTone('2', 15), 'done');
    assert.equal(deviationTone(null, 15), 'neutral');
  });
});
