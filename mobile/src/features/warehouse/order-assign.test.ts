import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { allCovered, asksWholeRoll, assignTarget, isCovered } from './order-assign.ts';

const roll = (uuid: string, meters: string, assigned: string) => ({
  uuid,
  product: { uuid: 'film', unit_type: 'roll_meter' },
  meters,
  quantity: null,
  assigned,
});
const piece = (uuid: string, quantity: number, assigned: number) => ({
  uuid,
  product: { uuid: 'kit', unit_type: 'piece' },
  meters: null,
  quantity,
  assigned,
});

describe('order assign', () => {
  it('covers a line by meters or pieces', () => {
    assert.equal(isCovered(roll('a', '15', '15.00')), true);
    assert.equal(isCovered(roll('a', '15', '14.5')), false);
    assert.equal(isCovered(piece('b', 2, 2)), true);
    assert.equal(isCovered(piece('b', 2, 1)), false);
  });

  it('knows a complete order', () => {
    assert.equal(allCovered([]), false);
    assert.equal(allCovered([roll('a', '15', '15'), piece('b', 2, 2)]), true);
    assert.equal(allCovered([roll('a', '15', '15'), piece('b', 2, 1)]), false);
  });

  it('sends a unit to the first open line of its product, never to a full one', () => {
    const items = [roll('a', '15', '15'), roll('c', '10', '0'), piece('b', 2, 2)];
    assert.deepEqual(assignTarget(items, 'film'), { kind: 'line', item: items[1] });
    assert.deepEqual(assignTarget(items, 'kit'), { kind: 'covered', item: items[2] });
    assert.deepEqual(assignTarget(items, 'other'), { kind: 'not_on_order' });
    assert.deepEqual(assignTarget(items, null), { kind: 'not_on_order' });
  });

  it('asks before a whole roll goes out with empty meters', () => {
    assert.equal(asksWholeRoll(roll('a', '15', '0'), ''), true);
    assert.equal(asksWholeRoll(roll('a', '15', '0'), ' 5 '), false);
    assert.equal(asksWholeRoll(piece('b', 2, 0), ''), false);
  });
});
