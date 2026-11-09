import assert from 'node:assert/strict';
import { test } from 'node:test';

import { familyStatusTone, looksLikeUnitCode, mergeCuts, siblingUnitPath } from './family.ts';

test('looksLikeUnitCode: barcodes yes, product names no', () => {
  assert.equal(looksLikeUnitCode('2026AUR230-D001'), true);
  assert.equal(looksLikeUnitCode(' OLX-00012345-3M '), true);
  assert.equal(looksLikeUnitCode('PPF Gloss'), false);
  assert.equal(looksLikeUnitCode('film'), false);
  assert.equal(looksLikeUnitCode('A1'), false);
  assert.equal(looksLikeUnitCode('X'.repeat(64) + '1'), false);
});

test('familyStatusTone', () => {
  assert.equal(familyStatusTone('on_hand'), 'done');
  assert.equal(familyStatusTone('in_transit'), 'progress');
  assert.equal(familyStatusTone('used'), 'problem');
  assert.equal(familyStatusTone('sold'), 'neutral');
  assert.equal(familyStatusTone('other'), 'neutral');
});

test('siblingUnitPath keeps the stack', () => {
  assert.equal(siblingUnitPath('/depo/stock/unit/OLX-1', 'OLX-1-3M'), '/depo/stock/unit/OLX-1-3M');
  assert.equal(siblingUnitPath('/admin/menu/stock/unit/A%2FB', 'B C'), '/admin/menu/stock/unit/B%20C');
});

test('mergeCuts: newest first, no repeats, capped', () => {
  const a = { uuid: 'a', new_barcode: 'R-3M' };
  const b = { uuid: 'b', new_barcode: 'R-3M-2' };
  const c = { uuid: 'c', new_barcode: 'R-2.5M' };
  assert.deepEqual(mergeCuts([], {}), []);
  assert.deepEqual(mergeCuts([a], { split: b }), [b, a]);
  assert.deepEqual(mergeCuts([a], { split: a }), [a]);
  assert.deepEqual(mergeCuts([a], { splits: [b, c] }), [c, b, a]);
  assert.deepEqual(mergeCuts([a, b], { split: c }, 2), [c, a]);
});
