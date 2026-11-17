import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { hasUnsavedMeasurement, leaveChoices, type MeasurementInput } from './leave-guard.ts';

const empty: MeasurementInput = { plate: '', vin: '', bodyType: null, values: {}, points: {} };

describe('hasUnsavedMeasurement (TEC-1222)', () => {
  it('is false for an untouched new measurement', () => {
    assert.equal(hasUnsavedMeasurement(empty, null), false);
    assert.equal(hasUnsavedMeasurement({ ...empty, plate: '  ', values: { hood: ' ' } }, null), false);
  });
  it('is true once a plate, VIN, body type or reading is entered', () => {
    assert.equal(hasUnsavedMeasurement({ ...empty, plate: '34ABC123' }, null), true);
    assert.equal(hasUnsavedMeasurement({ ...empty, vin: 'WVW' }, null), true);
    assert.equal(hasUnsavedMeasurement({ ...empty, bodyType: 'sedan' }, null), true);
    assert.equal(hasUnsavedMeasurement({ ...empty, values: { hood: '120' } }, null), true);
    assert.equal(hasUnsavedMeasurement({ ...empty, points: { 'HOOD#1': {} } }, null), true);
  });
  it('compares a resumed draft with what was stored', () => {
    const stored: MeasurementInput = { ...empty, plate: '34ABC123', values: { hood: '120', roof: '95' }, points: { 'HOOD#1': {} } };
    assert.equal(hasUnsavedMeasurement({ ...stored, values: { roof: '95', hood: '120' } }, stored), false);
    assert.equal(hasUnsavedMeasurement({ ...stored, values: { ...stored.values, door: '88' } }, stored), true);
    assert.equal(hasUnsavedMeasurement({ ...stored, points: { 'HOOD#1': {}, 'HOOD#2': {} } }, stored), true);
  });
});

describe('leaveChoices (TEC-1222)', () => {
  it('offers Save only when the draft can be stored', () => {
    assert.deepEqual(leaveChoices({ plateOk: true, vinOk: true }), ['cancel', 'save', 'discard']);
    assert.deepEqual(leaveChoices({ plateOk: false, vinOk: true }), ['cancel', 'discard']);
    assert.deepEqual(leaveChoices({ plateOk: true, vinOk: false }), ['cancel', 'discard']);
  });
});
