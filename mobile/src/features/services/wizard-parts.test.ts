import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { highlightedParts, itemPartRows, itemPartsMode, togglePart } from './wizard.ts';

describe('itemPartsMode (TEC-1077)', () => {
  it('assigns the chosen regions without the body diagram when one fits', () => {
    assert.equal(itemPartsMode(['hood', 'roof'], ['hood', 'trunk']), 'assign');
  });
  it('falls back to the diagram without a preset or without a fitting region', () => {
    assert.equal(itemPartsMode([], ['hood']), 'picker');
    assert.equal(itemPartsMode(['roof'], ['hood']), 'picker');
  });
  it('asks nothing when the product carries no parts', () => {
    assert.equal(itemPartsMode(['hood'], []), 'none');
    assert.equal(itemPartsMode(['hood'], null), 'none');
  });
});

describe('togglePart', () => {
  it('adds and removes, in the order of the chosen regions', () => {
    const order = ['hood', 'roof', 'trunk'];
    assert.deepEqual(togglePart(['trunk'], 'hood', order), ['hood', 'trunk']);
    assert.deepEqual(togglePart(['hood', 'trunk'], 'hood', order), ['trunk']);
  });
});

describe('itemPartRows / highlightedParts', () => {
  const rows = itemPartRows([
    { uuid: 'i1', product: { name: 'PPF' }, applied_parts: ['hood', 'hood', 'roof'] },
    { uuid: 'i2', product: { name: 'Seramik' }, applied_parts: null },
    { uuid: 'i3', product: { name: 'Cam filmi' }, applied_parts: ['roof', 'windshield'] },
  ]);
  it('lists each product with its regions once', () => {
    assert.deepEqual(rows, [
      { uuid: 'i1', name: 'PPF', parts: ['hood', 'roof'] },
      { uuid: 'i2', name: 'Seramik', parts: [] },
      { uuid: 'i3', name: 'Cam filmi', parts: ['roof', 'windshield'] },
    ]);
  });
  it('highlights the focused product, else every product', () => {
    assert.deepEqual(highlightedParts(rows, 'i3'), ['roof', 'windshield']);
    assert.deepEqual(highlightedParts(rows, null), ['hood', 'roof', 'windshield']);
    assert.deepEqual(highlightedParts(rows, 'gone'), ['hood', 'roof', 'windshield']);
  });
});
