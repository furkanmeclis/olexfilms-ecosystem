import assert from 'node:assert/strict';
import { test } from 'node:test';

import { creditsText, hasCreditsLeft, parseCredits, productUsesCredits } from './credits.ts';

test('creditsText: remaining / total, nothing for a roll or a fixed barcode (TEC-1221)', () => {
  assert.equal(creditsText({ credits_total: 5, credits_remaining: 3 }), '3 / 5');
  assert.equal(creditsText(null), null);
  assert.equal(creditsText(undefined), null);
});

test('hasCreditsLeft: a unit with 0 credits cannot be selected, one without credits can', () => {
  assert.equal(hasCreditsLeft({ credits_total: 5, credits_remaining: 0 }), false);
  assert.equal(hasCreditsLeft({ credits_total: 5, credits_remaining: 1 }), true);
  assert.equal(hasCreditsLeft(null), true);
});

test('productUsesCredits: only serial piece products', () => {
  assert.equal(productUsesCredits({ unit_type: 'piece', uses_fixed_barcode: false }), true);
  assert.equal(productUsesCredits({ unit_type: 'piece', uses_fixed_barcode: true }), false);
  assert.equal(productUsesCredits({ unit_type: 'roll_meter', uses_fixed_barcode: false }), false);
});

test('parseCredits: whole numbers 1..1000', () => {
  assert.equal(parseCredits(' 7 '), 7);
  assert.equal(parseCredits('1000'), 1000);
  assert.equal(parseCredits('0'), null);
  assert.equal(parseCredits('1001'), null);
  assert.equal(parseCredits('2.5'), null);
  assert.equal(parseCredits(''), null);
});
