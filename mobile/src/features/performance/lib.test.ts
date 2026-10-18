import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { formatMetric, metricKind, metricNumber, recentMonths } from './lib.ts';

const fmt = {
  number: (n: number, d = 0) => n.toFixed(d),
  money: (n: number, c: string) => `${n.toFixed(2)} ${c}`,
};

describe('formatMetric', () => {
  it('shows ratios as percent, money with its currency, counts whole', () => {
    assert.equal(formatMetric('warranty_start_rate', { value: '0.456' }, fmt), '45.6%');
    assert.equal(formatMetric('order_volume', { value: '1200', currency: 'TRY' }, fmt), '1200.00 TRY');
    assert.equal(formatMetric('order_volume', { value: '1200' }, fmt, 'EUR'), '1200.00 EUR');
    assert.equal(formatMetric('services_count', { value: '42.4' }, fmt), '42');
  });
  it('shows a dash without a value', () => {
    assert.equal(formatMetric('services_count', undefined, fmt), '–');
    assert.equal(formatMetric('services_count', { value: '' }, fmt), '–');
    assert.equal(metricNumber({ value: 'x' }), null);
  });
  it('knows metric kinds and defaults unknown ones to score', () => {
    assert.equal(metricKind('services_count'), 'count');
    assert.equal(metricKind('whatever'), 'score');
  });
});

describe('recentMonths', () => {
  it('lists months newest first across a year end', () => {
    assert.deepEqual(recentMonths(new Date(2026, 1, 15), 4), ['2026-02', '2026-01', '2025-12', '2025-11']);
  });
});
