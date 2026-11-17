import assert from 'node:assert/strict';
import { test } from 'node:test';

import { canPartialShip, receiptDiscrepancies, receiptValid, shortLines, transferMissing } from './discrepancy-logic.ts';

const item = (name: string, quantity: number, assigned: string, units: string[] = []) =>
  ({
    uuid: name,
    product: { name },
    quantity,
    meters: null,
    assigned,
    units: units.map((u) => ({ unit_uuid: u, barcode: `B-${u}` })),
  }) as never;

test('TEC-1227: short lines and partial shipment availability', () => {
  const o = { can_partial_ship: true, items: [item('Film', 2, '1'), item('Plexi', 1, '1'), item('Kit', 1, '0')] };
  assert.deepEqual(shortLines(o), [
    { name: 'Film', picked: 1, rest: 1 },
    { name: 'Kit', picked: 0, rest: 1 },
  ]);
  assert.equal(canPartialShip(o), true);
  assert.equal(canPartialShip({ ...o, can_partial_ship: false }), false);
  assert.equal(canPartialShip({ can_partial_ship: true, items: [item('Kit', 1, '0')] }), false);
});

test('TEC-1227: receipt discrepancies need a reason and one received unit', () => {
  const o = { items: [item('Film', 2, '2', ['u1', 'u2'])] };
  const marks = { u1: { kind: 'missing' as const, reason: ' Yok ' }, u2: { kind: 'ok' as const, reason: '' } };
  assert.deepEqual(receiptDiscrepancies(marks), [{ unit_uuid: 'u1', kind: 'missing', reason: 'Yok' }]);
  assert.equal(receiptValid(o, marks), true);
  assert.equal(receiptValid(o, { u1: { kind: 'damaged', reason: '' } }), false);
  assert.equal(
    receiptValid(o, { u1: { kind: 'missing', reason: 'a' }, u2: { kind: 'missing', reason: 'b' } }),
    false,
  );
});

test('TEC-1227: transfer lines not ticked are missing', () => {
  assert.deepEqual(transferMissing([{ uuid: 'a' }, { uuid: 'b' }], new Set(['a'])), [{ line_uuid: 'b' }]);
});
