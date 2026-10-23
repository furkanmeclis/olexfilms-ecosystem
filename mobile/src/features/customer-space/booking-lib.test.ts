import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  addDaysIso,
  dayState,
  dealerPlace,
  dedupeDealers,
  filterPastDealers,
  filterPublicDealers,
  formatSlotTime,
  isoDateInZone,
  parseRetryAfter,
  uuidParam,
} from './booking-lib.ts';

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

describe('public dealer search', () => {
  const rows = [
    { uuid: 'a', name: 'Işık Oto', city: 'İstanbul', district: 'Şişli', accepts_appointments: true },
    { uuid: 'b', name: 'Kapalı Bayi', city: 'İstanbul', district: 'Kadıköy', accepts_appointments: false },
    { uuid: 'c', name: 'Ege Film', city: 'İzmir', district: 'Bornova', accepts_appointments: true },
  ];

  it('keeps only dealers that accept appointments', () => {
    assert.deepEqual(filterPublicDealers(rows, '').map((d) => d.uuid), ['a', 'c']);
  });

  it('matches name, city and district, Turkish-aware', () => {
    assert.deepEqual(filterPublicDealers(rows, 'istanbul').map((d) => d.uuid), ['a']);
    assert.deepEqual(filterPublicDealers(rows, 'SİŞLİ').map((d) => d.uuid), ['a']);
    assert.deepEqual(filterPublicDealers(rows, 'isik').map((d) => d.uuid), ['a']);
    assert.deepEqual(filterPublicDealers(rows, 'bornova').map((d) => d.uuid), ['c']);
    assert.deepEqual(filterPublicDealers(rows, 'yok'), []);
  });

  it('shows district and city', () => {
    assert.equal(filterPublicDealers(rows, 'ege')[0].place, 'Bornova, İzmir');
    assert.equal(dealerPlace({ city: 'Ankara', district: '' }), 'Ankara');
  });

  it('dedupes against past dealers', () => {
    const past = [{ uuid: 'a', name: 'Işık Oto' }];
    assert.deepEqual(dedupeDealers(past, filterPublicDealers(rows, '')).map((d) => d.uuid), ['c']);
  });

  it('filters past dealers by text', () => {
    const past = [{ uuid: 'a', name: 'Işık Oto' }, { uuid: 'x', name: 'Zeta' }];
    assert.deepEqual(filterPastDealers(past, 'ışık').map((d) => d.uuid), ['a']);
    assert.equal(filterPastDealers(past, '').length, 2);
  });
});

describe('parseRetryAfter', () => {
  it('reads seconds only', () => {
    assert.equal(parseRetryAfter('7'), 7);
    assert.equal(parseRetryAfter(null), null);
    assert.equal(parseRetryAfter('Wed, 21 Oct 2026 07:28:00 GMT'), null);
  });
});
