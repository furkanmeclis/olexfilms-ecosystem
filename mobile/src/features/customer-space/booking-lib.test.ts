import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { addDaysIso, dayState, formatSlotTime, isoDateInZone, uuidParam } from './booking-lib.ts';

describe('addDaysIso', () => {
  it('crosses month and year ends', () => {
    assert.equal(addDaysIso('2026-12-30', 3), '2027-01-02');
    assert.equal(addDaysIso('2026-03-01', -1), '2026-02-28');
  });
});

describe('dayState', () => {
  it('orders closed, full, none, bookable', () => {
    assert.equal(dayState({ closed: true, remaining_capacity: 3, slots: [1] }), 'closed');
    assert.equal(dayState({ closed: false, remaining_capacity: 0, slots: [1] }), 'full');
    assert.equal(dayState({ closed: false, remaining_capacity: 2, slots: [] }), 'none');
    assert.equal(dayState({ closed: false, remaining_capacity: 2, slots: [1] }), 'bookable');
  });
});

describe('formatSlotTime', () => {
  it('uses the dealer zone', () => {
    assert.equal(formatSlotTime('2026-10-20T07:00:00Z', 'en-GB', 'Europe/Istanbul'), '10:00');
    assert.equal(formatSlotTime('2026-10-20T07:00:00Z', 'en-GB', 'UTC'), '07:00');
  });
  it('falls back on a bad zone', () => {
    assert.match(formatSlotTime('2026-10-20T07:00:00Z', 'en-GB', 'Nope/Zone'), /^\d\d:\d\d$/);
  });
});

describe('uuidParam', () => {
  it('accepts only UUIDs', () => {
    assert.equal(uuidParam('0b9f6c1e-2f4a-4c1d-9a55-3a1f0e7d2b10'), '0b9f6c1e-2f4a-4c1d-9a55-3a1f0e7d2b10');
    assert.equal(uuidParam('nope'), null);
    assert.equal(uuidParam(['0b9f6c1e-2f4a-4c1d-9a55-3a1f0e7d2b10']), null);
    assert.equal(uuidParam(undefined), null);
  });
});

describe('isoDateInZone', () => {
  it('derives the date in the dealer zone', () => {
    const now = new Date('2026-10-20T22:30:00Z');
    assert.equal(isoDateInZone(now, 'Europe/Istanbul'), '2026-10-21');
    assert.equal(isoDateInZone(now, 'UTC'), '2026-10-20');
  });
  it('falls back on a bad zone', () => {
    assert.match(isoDateInZone(new Date(), 'Nope/Zone'), /^\d{4}-\d{2}-\d{2}$/);
  });
});
