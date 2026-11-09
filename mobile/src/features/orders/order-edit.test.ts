import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addEditLine,
  type CartLine,
  editChanged,
  linesBelowAssigned,
  linesFromItems,
  toItemInputs,
} from './order-lines.ts';
import { actionKey, canMarkPlotter, stepParts } from './order-summary.ts';

const item = (over: Partial<Parameters<typeof linesFromItems>[0][number]> = {}) => ({
  product: { uuid: 'p1', sku: 'PPF-1', name: 'PPF', unit_type: 'piece' },
  quantity: 5,
  meters: null,
  assigned: '2',
  plotter_cut: true,
  ...over,
});
const roll = item({
  product: { uuid: 'p2', sku: 'ROLL-1', name: 'Roll', unit_type: 'roll_meter' },
  quantity: null,
  meters: '12.50',
  assigned: '10.00',
  plotter_cut: false,
});

test('linesFromItems: amount as stored, assigned as a number, plotter only for the seller (TEC-1057)', () => {
  const [a, b] = linesFromItems([item(), roll], true);
  assert.equal(a!.amount, '5');
  assert.equal(a!.assigned, 2);
  assert.equal(a!.plotter_cut, true);
  assert.equal(b!.amount, '12.50');
  assert.equal(b!.assigned, 10);
  assert.equal(b!.plotter_cut, false);
  const [buyer] = linesFromItems([item()], false);
  assert.equal(buyer!.plotter_cut, undefined);
  assert.deepEqual(toItemInputs([buyer!]), [{ product_uuid: 'p1', quantity: 5 }]);
});

test('linesBelowAssigned: an amount under the assigned units is refused on the client', () => {
  const lines = linesFromItems([item(), roll], true);
  assert.equal(linesBelowAssigned(lines).size, 0);
  const lower = lines.map((l) => (l.product_uuid === 'p1' ? { ...l, amount: '1' } : { ...l, amount: '9,5' }));
  assert.deepEqual([...linesBelowAssigned(lower)].sort(), ['p1', 'p2']);
  // Equal to the assigned amount is allowed; an empty amount is the line's own error.
  const same = lines.map((l) => (l.product_uuid === 'p1' ? { ...l, amount: '2' } : { ...l, amount: '' }));
  assert.equal(linesBelowAssigned(same).size, 0);
});

test('toItemInputs: the full line set with the plotter marks (PUT items)', () => {
  const lines = linesFromItems([item(), roll], true);
  assert.deepEqual(toItemInputs(lines), [
    { product_uuid: 'p1', quantity: 5, plotter_cut: true },
    { product_uuid: 'p2', meters: '12.50', plotter_cut: false },
  ]);
  const cart: CartLine[] = [{ product_uuid: 'p3', sku: 'S', name: 'N', unit_type: 'piece', amount: '2', plotter_cut: true }];
  assert.deepEqual(toItemInputs(cart, true), [{ product_uuid: 'p3', quantity: 2, plotter_cut: true }]);
});

test('addEditLine / editChanged', () => {
  const initial = linesFromItems([item()], true);
  assert.equal(editChanged(initial, initial), false);
  const added = addEditLine(initial, { uuid: 'p9', sku: 'X', name: 'New', unit_type: 'piece' }, true);
  assert.equal(added.length, 2);
  assert.deepEqual(added[1], { product_uuid: 'p9', sku: 'X', name: 'New', unit_type: 'piece', amount: '1', assigned: 0, plotter_cut: false });
  assert.equal(addEditLine(added, { uuid: 'p9', sku: 'X', name: 'New', unit_type: 'piece' }, true), added);
  assert.equal(editChanged(initial, added), true);
  assert.equal(editChanged(initial, [{ ...initial[0]!, amount: '5.0' }]), false);
  assert.equal(editChanged(initial, [{ ...initial[0]!, amount: '6' }]), true);
  assert.equal(editChanged(initial, [{ ...initial[0]!, plotter_cut: false }]), true);
  assert.equal(editChanged(initial, []), true);
});

test('canMarkPlotter: the seller with orders.write or orders.ship, not on a draft or cancelled order (TEC-1059)', () => {
  const both = { write: true, ship: true };
  assert.equal(canMarkPlotter({ role: 'seller', status: 'preparing' }, both), true);
  assert.equal(canMarkPlotter({ role: 'seller', status: 'shipped' }, { write: false, ship: true }), true);
  assert.equal(canMarkPlotter({ role: 'seller', status: 'approved' }, { write: true, ship: false }), true);
  assert.equal(canMarkPlotter({ role: 'seller', status: 'approved' }, { write: false, ship: false }), false);
  assert.equal(canMarkPlotter({ role: 'seller', status: 'draft' }, both), false);
  assert.equal(canMarkPlotter({ role: 'seller', status: 'cancelled' }, both), false);
  assert.equal(canMarkPlotter({ role: 'buyer', status: 'approved' }, both), false);
});

test('stepParts: line changes and plotter marks of the timeline', () => {
  assert.equal(actionKey('plotter_marked'), 'step.plotter_marked');
  assert.deepEqual(stepParts({ action: 'plotter_marked', plotter_cut: true }), [{ key: 'plotter.marked_on' }]);
  assert.deepEqual(stepParts({ action: 'plotter_marked', plotter_cut: false }), [{ key: 'plotter.marked_off' }]);
  assert.deepEqual(
    stepParts({
      action: 'updated',
      changes: [
        { product: 'A', before: null, after: '3', plotter_cut: true },
        { product: 'B', before: '2', after: null },
        { product: 'C', before: '2', after: '4' },
        { product: 'D', before: '1', after: '1', plotter_cut: false },
      ],
    }),
    [
      { key: 'change.added', params: { product: 'A', after: '3' } },
      { key: 'plotter.change_on', params: { product: 'A' } },
      { key: 'change.removed', params: { product: 'B', before: '2' } },
      { key: 'change.changed', params: { product: 'C', before: '2', after: '4' } },
      { key: 'plotter.change_off', params: { product: 'D' } },
    ],
  );
  assert.deepEqual(stepParts({ action: 'approved' }), []);
});
