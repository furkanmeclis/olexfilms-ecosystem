import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  buildRows,
  EMPTY_VEHICLE,
  frequentList,
  isComplete,
  pickBrand,
  pickModel,
  previousStep,
  reachableSteps,
  startStep,
  vehicleTitle,
  yearOptions,
  yearSpan,
  type VehicleValue,
} from './picker.ts';

const NOW = new Date('2026-06-01T00:00:00Z');
const bmw = { uuid: 'b1', name: 'BMW' };
const audi = { uuid: 'b2', name: 'Audi' };
const s3 = { uuid: 'm1', name: '3 Serisi', yearStart: 2012, yearStop: 2019 };
const full: VehicleValue = { brand: bmw, model: s3, year: 2015 };

describe('vehicle picker value', () => {
  it('needs brand and model to be complete', () => {
    assert.equal(isComplete(EMPTY_VEHICLE), false);
    assert.equal(isComplete({ ...EMPTY_VEHICLE, brand: bmw }), false);
    assert.equal(isComplete({ brand: bmw, model: s3, year: null }), true);
    assert.equal(isComplete(null), false);
  });

  it('titles brand and model', () => {
    assert.equal(vehicleTitle(full), 'BMW 3 Serisi');
    assert.equal(vehicleTitle({ ...EMPTY_VEHICLE, brand: bmw }), 'BMW');
    assert.equal(vehicleTitle(undefined), '');
  });

  it('opens on the first missing step', () => {
    assert.equal(startStep(null), 'brand');
    assert.equal(startStep({ ...EMPTY_VEHICLE, brand: bmw }), 'model');
    assert.equal(startStep(full), 'year');
  });

  it('allows jumping back only to reached steps', () => {
    assert.deepEqual([...reachableSteps(EMPTY_VEHICLE, 'brand')].sort(), ['brand']);
    assert.deepEqual([...reachableSteps({ ...EMPTY_VEHICLE, brand: bmw }, 'model')].sort(), ['brand', 'model']);
    assert.deepEqual([...reachableSteps(full, 'brand')].sort(), ['brand', 'model', 'year']);
    assert.equal(previousStep('brand'), null);
    assert.equal(previousStep('year'), 'model');
  });

  it('resets model and year on another brand only', () => {
    assert.deepEqual(pickBrand(full, audi), { brand: audi, model: null, year: null });
    assert.deepEqual(pickBrand(full, { ...bmw }), full);
  });

  it('keeps the year only inside the new model span', () => {
    assert.equal(pickModel(full, { uuid: 'm2', name: 'X1', yearStart: 2014, yearStop: 2022 }, NOW).year, 2015);
    assert.equal(pickModel(full, { uuid: 'm3', name: 'i3', yearStart: 2016, yearStop: null }, NOW).year, null);
  });
});

describe('yearOptions', () => {
  it('lists the production span newest first', () => {
    assert.deepEqual(yearOptions({ yearStart: 2017, yearStop: 2019 }, NOW), [2019, 2018, 2017]);
  });
  it('clamps an open or future end to next year', () => {
    assert.equal(yearOptions({ yearStart: 2024, yearStop: null }, NOW)[0], 2027);
    assert.equal(yearOptions({ yearStart: 2024, yearStop: 2040 }, NOW)[0], 2027);
  });
  it('falls back to the full range without a start or with a reversed span', () => {
    const all = yearOptions(null, NOW);
    assert.equal(all[0], 2027);
    assert.equal(all.at(-1), 1975);
    assert.deepEqual(yearOptions({ yearStart: 2030, yearStop: 2031 }, NOW), all);
  });
  it('formats the span', () => {
    assert.equal(yearSpan(2015, 2020), '2015–2020');
    assert.equal(yearSpan(2015, null), '2015–');
    assert.equal(yearSpan(null, null), '');
  });
});

describe('buildRows', () => {
  const all = [audi, bmw, { uuid: 'b3', name: 'Citroën' }];
  const from = (f: { uuid: string; name: string }) => ({ uuid: f.uuid, name: f.name });

  it('puts frequent items first under their own header, then every item', () => {
    const rows = buildRows(all, [{ uuid: 'b2', name: 'BMW' }, { uuid: 'b1', name: 'Audi' }].reverse(), false, from);
    assert.deepEqual(
      rows.map((r) => r.key),
      ['h:frequent', 'top:b1', 'top:b2', 'h:all', 'all:b2', 'all:b1', 'all:b3'],
    );
  });

  it('uses the frequent row itself when its catalog page is not loaded', () => {
    const rows = buildRows([audi], [{ uuid: 'b9', name: 'Togg' }], false, from);
    const top = rows.find((r) => r.key === 'top:b9');
    assert.ok(top && top.kind === 'item' && top.item.name === 'Togg' && top.frequent);
  });

  it('is a plain list while searching or without frequent data', () => {
    for (const rows of [buildRows(all, [{ uuid: 'b1', name: 'BMW' }], true, from), buildRows(all, [], false, from), buildRows(all, undefined, false, from)]) {
      assert.deepEqual(rows.map((r) => r.key), ['all:b2', 'all:b1', 'all:b3']);
    }
  });

  it('drops duplicate frequent rows', () => {
    const rows = buildRows(all, [{ uuid: 'b1', name: 'BMW' }, { uuid: 'b1', name: 'BMW' }], false, from);
    assert.equal(rows.filter((r) => r.key === 'top:b1').length, 1);
  });
});

describe('frequentList', () => {
  it('tolerates an older backend', () => {
    assert.deepEqual(frequentList(undefined), []);
    assert.deepEqual(frequentList({ items: [] }), []);
    assert.deepEqual(frequentList([null, { name: 'x' }, { uuid: 'a', name: 'A' }]), [{ uuid: 'a', name: 'A' }]);
  });
});
