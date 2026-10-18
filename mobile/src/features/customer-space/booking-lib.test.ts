import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { addDaysIso, dayState, filterDealers, formatSlotTime } from './booking-lib.ts';

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

describe('filterDealers', () => {
  const d = [
    { name: 'Alfa Oto', city: 'Ankara', district: 'Çankaya' },
    { name: 'Beta', city: 'İzmir', district: 'Bornova' },
  ];
  it('matches name, city and district', () => {
    assert.equal(filterDealers(d, 'ankara').length, 1);
    assert.equal(filterDealers(d, 'bornova')[0].name, 'Beta');
    assert.equal(filterDealers(d, '  ').length, 2);
    assert.equal(filterDealers(d, 'zzz').length, 0);
  });
});
