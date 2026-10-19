import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { adapterErrorKey, gaugeErrorKey } from './errors.ts';
import {
  appendValue,
  formatInputValue,
  fromMicrons,
  initialSampler,
  type SamplerState,
  samplerProgress,
  stepSampler,
  substrateOf,
} from './readings.ts';

function feed(values: (number | null)[], start: SamplerState = initialSampler, material: 'fe' | 'al' = 'fe') {
  let state = start;
  const commits: number[] = [];
  for (const um of values) {
    const r = stepSampler(state, { um, material });
    state = r.state;
    if (r.commit) commits.push(r.commit.um);
  }
  return { state, commits };
}

describe('units', () => {
  it('converts microns to the user unit', () => {
    assert.equal(fromMicrons(120.4, 'um'), 120);
    assert.equal(fromMicrons(127, 'mil'), 5);
    assert.equal(fromMicrons(115, 'mil'), 4.5);
  });

  it('formats field values', () => {
    assert.equal(formatInputValue(120, 'um'), '120');
    assert.equal(formatInputValue(127, 'mil'), '5.0');
    assert.equal(formatInputValue(115, 'mil', ','), '4,5');
  });

  it('appends to a part value list', () => {
    assert.equal(appendValue('', '120'), '120');
    assert.equal(appendValue(' 118 ', '120'), '118 120');
  });

  it('maps material to substrate', () => {
    assert.equal(substrateOf('fe'), 'Fe');
    assert.equal(substrateOf('al'), 'Al');
    assert.equal(substrateOf('unknown'), undefined);
  });
});

describe('stepSampler', () => {
  it('commits the mean once the probe is steady', () => {
    const { state, commits } = feed([118, 120, 122, 119, 121, 120, 118, 122]);
    assert.deepEqual(commits, [120]);
    assert.equal(state.phase, 'committed');
    assert.equal(samplerProgress(state), 1);
  });

  it('commits only once per contact, again after lifting', () => {
    const steady = Array<number>(8).fill(100);
    const { commits } = feed([...steady, 100, 100, 100, null, ...steady.map(() => 200)]);
    assert.deepEqual(commits, [100, 200]);
  });

  it('waits while the probe slides, using the last window', () => {
    const { commits } = feed([40, 400, 90, 300, 120, 120, 121, 119, 120, 120, 122, 118, 120, 121]);
    assert.deepEqual(commits, [120]);
  });

  it('restarts when the probe lifts mid-contact', () => {
    const { state, commits } = feed([100, 100, 100, null, 100]);
    assert.deepEqual(commits, []);
    assert.equal(state.samples.length, 1);
    assert.equal(samplerProgress(state), 1 / 8);
  });

  it('drops samples of another material', () => {
    const { state } = feed([100, 100, 100]);
    const next = stepSampler(state, { um: 100, material: 'al' });
    assert.deepEqual(next.state.samples, [100]);
  });
});

describe('errors', () => {
  it('maps BLE codes and own failures', () => {
    assert.equal(gaugeErrorKey({ errorCode: 102 }), 'ble.error.powered_off');
    assert.equal(gaugeErrorKey({ errorCode: 601 }), 'ble.error.location_off');
    assert.equal(gaugeErrorKey(new Error('curve')), 'ble.error.curve');
    assert.equal(gaugeErrorKey('weird'), 'ble.error.generic');
  });

  it('maps adapter states', () => {
    assert.equal(adapterErrorKey('PoweredOn'), null);
    assert.equal(adapterErrorKey('Unknown'), null);
    assert.equal(adapterErrorKey('PoweredOff'), 'ble.error.powered_off');
    assert.equal(adapterErrorKey('Unauthorized'), 'ble.error.unauthorized');
  });
});
