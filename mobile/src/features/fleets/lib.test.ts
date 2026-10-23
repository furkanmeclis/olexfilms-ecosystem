import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { closedPeriods, fleetReportTone, fleetUserTone, inviteErrors, lastClosedPeriods, monthRange, shiftMonth } from './lib.ts';

describe('fleet report periods', () => {
  it('defaults to the last closed month and quarter', () => {
    assert.deepEqual(lastClosedPeriods(new Date(2026, 9, 23)), { month: '2026-09', quarter: '2026-Q3' });
    assert.deepEqual(lastClosedPeriods(new Date(2026, 0, 5)), { month: '2025-12', quarter: '2025-Q4' });
  });

  it('lists closed periods newest first', () => {
    assert.deepEqual(closedPeriods('monthly', new Date(2026, 1, 10), 3), ['2026-01', '2025-12', '2025-11']);
    assert.deepEqual(closedPeriods('quarterly', new Date(2026, 4, 1), 3), ['2026-Q1', '2025-Q4', '2025-Q3']);
  });
});

describe('statement month', () => {
  it('shifts across years', () => {
    assert.equal(shiftMonth('2026-01', -1), '2025-12');
    assert.equal(shiftMonth('2025-12', 1), '2026-01');
  });

  it('covers the whole month, or up to today for the current one', () => {
    assert.deepEqual(monthRange('2026-02', new Date(2026, 9, 23)), { from: '2026-02-01', to: '2026-02-28' });
    assert.deepEqual(monthRange('2026-10', new Date(2026, 9, 23)), { from: '2026-10-01', to: '2026-10-23' });
  });
});

describe('fleet users', () => {
  it('needs a valid e-mail and a name to invite', () => {
    assert.deepEqual(inviteErrors({ email: 'x', name: ' ' }), ['email', 'name']);
    assert.deepEqual(inviteErrors({ email: 'a@b.co', name: 'Ali' }), []);
  });

  it('tones', () => {
    assert.equal(fleetUserTone('active'), 'done');
    assert.equal(fleetUserTone('disabled'), 'neutral');
    assert.equal(fleetReportTone('ready'), 'done');
    assert.equal(fleetReportTone('failed'), 'problem');
    assert.equal(fleetReportTone('pending'), 'progress');
  });
});
