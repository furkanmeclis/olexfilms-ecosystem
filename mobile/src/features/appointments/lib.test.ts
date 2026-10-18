import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { addDays, canReschedule, nextStatuses, parseDay, startOfWeek, ymd } from './lib.ts';

describe('nextStatuses', () => {
  it('follows the web transitions', () => {
    assert.deepEqual(nextStatuses('scheduled'), ['confirmed', 'no_show', 'cancelled']);
    assert.deepEqual(nextStatuses('confirmed'), ['arrived', 'no_show', 'cancelled']);
    assert.deepEqual(nextStatuses('arrived'), []);
    assert.equal(canReschedule({ status: 'arrived' }), false);
  });
});

describe('week helpers', () => {
  it('starts the week on Monday', () => {
    assert.equal(ymd(startOfWeek(parseDay('2026-10-18'))), '2026-10-12'); // Sunday
    assert.equal(ymd(startOfWeek(parseDay('2026-10-19'))), '2026-10-19'); // Monday
  });
  it('adds days across month ends', () => {
    assert.equal(ymd(addDays(parseDay('2026-10-31'), 1)), '2026-11-01');
  });
});
