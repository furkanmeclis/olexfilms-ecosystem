import assert from 'node:assert/strict';
import { test } from 'node:test';

import { plainText } from './plain-text.ts';

test('removes line-start markers and turns bullets into dots', () => {
  assert.equal(plainText('# Title\n> quoted\n- one\n* two'), 'Title\nquoted\n• one\n• two');
});

test('removes emphasis and code at word boundaries', () => {
  assert.equal(plainText('A **strong** and _soft_ word, `code`.'), 'A strong and soft word, code.');
  assert.equal(plainText('(*note*)'), '(note)');
});

test('keeps underscores and stars inside words and numbers', () => {
  assert.equal(plainText('SKU PPF_8MIL_CLEAR fits 2*3 m'), 'SKU PPF_8MIL_CLEAR fits 2*3 m');
  assert.equal(plainText('snake_case_name and a*b*c'), 'snake_case_name and a*b*c');
});
