import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { BUNDLED_CURVES } from './bundled-curves.ts';
import type { GaugeCurve, GaugeReading } from './curve.ts';
import { initialMeasure, type MeasureState, stepMeasure } from './measure.ts';
import { decodeFrame, encodeFrame } from './protocol.ts';
import { type Commit, initialSampler, stepSampler } from './readings.ts';

const curve = BUNDLED_CURVES[32475]!;
const frame = (type: number, frequency: number, serial = 32475) =>
  decodeFrame(encodeFrame({ type, frequency, serial, batteryVoltage: 2.9 }))!;

/** Feeds frames through stepMeasure and the sampler, like manager.ts onLine. */
function run(frames: [number, number][], c: GaugeCurve | null = curve, offsets = null as { fe: number; al: number } | null) {
  let m: MeasureState = initialMeasure;
  let s = initialSampler;
  const readings: GaugeReading[] = [];
  const commits: Commit[] = [];
  for (const [type, freq] of frames) {
    const step = stepMeasure(m, frame(type, freq), c, offsets, 0);
    m = step.state;
    if (!step.reading) continue;
    readings.push(step.reading);
    const sampled = stepSampler(s, step.reading);
    s = sampled.state;
    if (sampled.commit) commits.push(sampled.commit);
  }
  return { readings, commits, state: m };
}

/** Mode 3: groups of 8 frames per channel. */
function alternate(fe: number, al: number, groups: number): [number, number][] {
  const out: [number, number][] = [];
  for (let g = 0; g < groups; g += 1) for (let i = 0; i < 8; i += 1) out.push(g % 2 === 0 ? [1, fe] : [2, al]);
  return out;
}

describe('stepMeasure (§4)', () => {
  it('measures steel on the Fe channel and holds the value on Al frames', () => {
    // ~230 µm on steel (REVERSE_ENGINEERING §3.3): Fe ≈ 19190, Al ≈ 20820.
    const { readings, commits } = run(alternate(19190, 20820, 4));
    const fe = readings.filter((r) => r.channel === 'fe');
    const al = readings.filter((r) => r.channel === 'al');
    assert.ok(fe.every((r) => r.material === 'fe' && !r.held && r.um === 228));
    assert.ok(al.every((r) => r.material === 'fe' && r.held && r.um === 228));
    // One contact, one commit, not reset by the Al groups.
    assert.equal(commits.length, 1);
    assert.deepEqual([commits[0]!.um, commits[0]!.material, commits[0]!.channel, commits[0]!.frequency], [228, 'fe', 'fe', 19190]);
  });

  it('measures aluminium on the Al channel and holds the value on Fe frames', () => {
    // On bare aluminium: Fe ≈ 24073 (> K), Al ≈ 26475 (≥ L, above the first Al point: 0 µm).
    const { readings, commits } = run(alternate(24073, 26475, 4));
    assert.ok(readings.every((r) => r.material === 'al'));
    assert.ok(readings.filter((r) => r.channel === 'fe').every((r) => r.held));
    assert.equal(commits.length, 1);
    assert.equal(commits[0]!.um, 0);
    assert.equal(commits[0]!.material, 'al');

    // 90 µm of paint on aluminium.
    const painted = run(alternate(24073, 26190, 4));
    assert.equal(painted.commits[0]!.um, 90);
  });

  it('reads the air as no value on both channels and forgets the Fe frequency', () => {
    const { readings, state } = run(alternate(22290, 23512, 2));
    assert.ok(readings.every((r) => r.um == null && !r.held));
    assert.equal(state.feFrequency, null);
  });

  it('starts a new contact after the probe lifted', () => {
    const frames = [...alternate(19190, 20820, 2), ...alternate(22290, 23512, 2), ...alternate(18240, 20000, 2)];
    assert.deepEqual(run(frames).commits.map((c) => c.um), [228, 90]);
  });

  it('uses the first Al frame against L when no Fe frame came yet', () => {
    const { readings } = run([[2, 26190], [2, 26190]]);
    assert.ok(readings.every((r) => r.material === 'al' && r.um === 90 && !r.held));
    const steel = run([[2, 20820]]);
    assert.equal(steel.readings[0]!.material, 'fe');
    assert.equal(steel.readings[0]!.held, true);
    assert.equal(steel.readings[0]!.um, null);
  });

  it('applies the calibration offset of the substrate', () => {
    const { commits } = run(alternate(18140, 20000, 2), curve, { fe: 100, al: -999 });
    assert.equal(commits[0]!.um, 90);
  });

  it('applies the device rules (Standard rounds to 10 µm, Economic has no Al)', () => {
    const standard = { ...curve, deviceType: 1 };
    assert.equal(run(alternate(19190, 20820, 2), standard).commits[0]!.um, 230);
    const economic = { ...curve, deviceType: 3 };
    const al = run(alternate(24073, 26190, 2), economic);
    assert.ok(al.readings.every((r) => r.um == null));
    assert.equal(al.commits.length, 0);
  });

  // §3.3: type 3 / 4 = the channel with the gauge's button pressed.
  it('turns the button frame of the substrate channel into a commit', () => {
    const { commits, readings } = run([[1, 19190], [2, 20820], [4, 20820], [3, 19190]]);
    assert.equal(readings[2]!.button, false); // other channel: held, no button
    assert.equal(readings[3]!.button, true);
    assert.equal(commits.length, 1);
    assert.equal(commits[0]!.trigger, 'button');
  });

  it('has no value without the serial curve', () => {
    assert.ok(run(alternate(19190, 20820, 2), null).readings.every((r) => r.um == null && r.material == null));
    const other = run([[1, 19190]], { ...curve, serial: 1355 });
    assert.equal(other.readings[0]!.um, null);
  });

  it('ignores frames of an unknown type', () => {
    assert.equal(stepMeasure(initialMeasure, frame(7, 19190), curve, null, 0).reading, null);
  });
});
