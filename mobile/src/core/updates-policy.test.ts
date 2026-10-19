import assert from 'node:assert/strict';
import { test } from 'node:test';

import { shouldCheckForUpdate, UPDATE_CHECK_INTERVAL_MS } from './updates-policy.ts';

test('first check always runs', () => {
  assert.equal(shouldCheckForUpdate(null, 1000), true);
});

test('checks are throttled inside the interval', () => {
  assert.equal(shouldCheckForUpdate(1000, 1000 + UPDATE_CHECK_INTERVAL_MS - 1), false);
});

test('checks resume once the interval has passed', () => {
  assert.equal(shouldCheckForUpdate(1000, 1000 + UPDATE_CHECK_INTERVAL_MS), true);
});
