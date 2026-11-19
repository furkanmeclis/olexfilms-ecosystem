import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  assignTarget,
  cutAssignBody,
  cutWarnings,
  isCovered,
  isCutItem,
  isMeterItem,
  pieceCredits,
  pieceMeters,
  restBelowMinCut,
} from './order-assign.ts';

const box = { uuid: 'box', product: { uuid: 'ppf', unit_type: 'piece' }, quantity: 1, meters: null, assigned: '0', sale_measure: 'box', line_credits: null };
const meter = { uuid: 'm', product: { uuid: 'ppf', unit_type: 'piece' }, quantity: null, meters: '3.00', assigned: '0', sale_measure: 'meter', line_credits: 5 };

const preview = {
  closed_box: false,
  whole: false,
  remnant: false,
  partial: false,
  source_meters: '12.00',
  cut_meters: '3.00',
  remaining_meters: '9.00',
  missing_meters: '3.00',
  min_cut_meters: '2.00',
};

describe('TEC-1241 meter lines of box products', () => {
  it('counts a meter line in meters and knows it is cut', () => {
    assert.equal(isMeterItem(meter), true);
    assert.equal(isCutItem(meter), true);
    assert.equal(isCutItem(box), false);
    // Before TEC-1241 the quantity (null) made the line look covered.
    assert.equal(isCovered(meter), false);
    assert.equal(isCovered({ ...meter, assigned: '3.00' }), true);
  });

  it('sends a unit with meters to the meter line, a closed box to the box line', () => {
    const items = [box, meter];
    assert.deepEqual(assignTarget(items, 'ppf', true), { kind: 'line', item: meter });
    assert.deepEqual(assignTarget(items, 'ppf', false), { kind: 'line', item: box });
    // The box line is full: the closed box is opened for the meter line.
    assert.deepEqual(assignTarget([{ ...box, assigned: '1' }, meter], 'ppf', false), { kind: 'line', item: meter });
    assert.deepEqual(assignTarget(items, 'ppf'), { kind: 'line', item: box });
  });

  it('names the box opening, the remnant and a short piece', () => {
    assert.deepEqual(cutWarnings(preview), []);
    assert.deepEqual(
      cutWarnings({ ...preview, closed_box: true, remnant: true, partial: true }).map((w) => w.key),
      ['orders.cut.box_open', 'orders.cut.remnant', 'orders.cut.partial_short'],
    );
    assert.equal(cutWarnings({ ...preview, closed_box: true, whole: true })[0].key, 'orders.cut.box_whole');
  });

  it('reads the typed piece meters and credits', () => {
    assert.equal(pieceMeters('3,5'), '3.5');
    assert.equal(pieceMeters('0'), '');
    assert.equal(pieceMeters('1.234'), '');
    assert.equal(pieceCredits(''), null);
    assert.equal(pieceCredits('3'), 3);
    assert.ok(Number.isNaN(pieceCredits('x') as number));
    assert.ok(Number.isNaN(pieceCredits('1001') as number));
  });

  it('builds the assign body of a confirmed cut', () => {
    assert.deepEqual(cutAssignBody('B1', { closed_box: true, partial: false }, null, 'k'), {
      barcode: 'B1',
      idempotency_key: 'k',
      open_box: true,
    });
    assert.deepEqual(
      cutAssignBody('B1', { closed_box: false, partial: true }, { meters: '2,5', note: ' iki parça ', credits: '2' }, 'k'),
      { barcode: 'B1', idempotency_key: 'k', meters: '2.5', credits: 2, partial: true, note: 'iki parça' },
    );
  });

  it('warns when a partial shipment leaves less than the minimum cut', () => {
    assert.equal(restBelowMinCut({ ...meter, meters: '6.00', assigned: '4.50', min_cut_meters: '2.00' }), true);
    assert.equal(restBelowMinCut({ ...meter, meters: '6.00', assigned: '3', min_cut_meters: '2.00' }), false);
    assert.equal(restBelowMinCut({ ...box, min_cut_meters: '2.00' }), false);
  });
});
