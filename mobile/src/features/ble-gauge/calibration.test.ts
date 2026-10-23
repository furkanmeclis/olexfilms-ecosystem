import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { BUNDLED_CURVES } from './bundled-curves.ts';
import {
  CALIBRATION,
  type CalibrationOutcome,
  calibrationProgress,
  calibrationSteps,
  calibrationTargetFrequency,
  calibrationTargetUm,
  fromBackendCalibration,
  newerCalibration,
  offsetsOf,
  startCalibrationStep,
  type StoredCalibration,
  stepCalibration,
  toBackendCalibration,
  uuidV4,
} from './calibration.ts';
import { frequencyToMicrons, type GaugeCurve, parseCurve } from './curve.ts';
import { decodeFrame, encodeFrame } from './protocol.ts';

const curve = BUNDLED_CURVES[32475]!;
const frame = (type: number, frequency: number, serial = 32475) =>
  decodeFrame(encodeFrame({ type, frequency, serial, batteryVoltage: 2.9 }))!;

function feed(c: GaugeCurve, material: 'fe' | 'al', frames: [number, number][], times?: number[]): CalibrationOutcome {
  let step = startCalibrationStep(c, material, 0);
  let out: CalibrationOutcome = { kind: 'progress', step };
  frames.forEach(([type, f], i) => {
    if (out.kind !== 'progress') return;
    out = stepCalibration(step, frame(type, f), c, times?.[i] ?? i * 100);
    if (out.kind === 'progress') step = out.step;
  });
  return out;
}

describe('calibration target (§6.1)', () => {
  it('is 90 µm for cal1 / cal2 / cal3 curves, else 150 µm', () => {
    assert.equal(calibrationTargetUm(curve), 90);
    assert.equal(calibrationTargetUm({ calVersion: null }), 150);
  });

  it('takes the frequency of the curve point at the foil thickness', () => {
    assert.equal(calibrationTargetFrequency(curve, 'fe'), 18240);
    assert.equal(calibrationTargetFrequency(curve, 'al'), 26190);
    // 150 µm curve: Fe 169 µm is out of ±10, the 6th point is used; Al 159 µm is within.
    const c150 = { ...curve, calVersion: null };
    assert.equal(calibrationTargetFrequency(c150, 'fe'), 19415);
    assert.equal(calibrationTargetFrequency(c150, 'al'), 26026);
  });

  it('skips the Al step on Economic gauges', () => {
    assert.deepEqual(calibrationSteps(2), ['fe', 'al']);
    assert.deepEqual(calibrationSteps(3), ['fe']);
    assert.deepEqual(calibrationSteps(4), ['fe']);
  });
});

describe('stepCalibration', () => {
  it('needs 10 steady frames and gives offset = target − measured', () => {
    // 90 µm foil on steel read 35 Hz high: 18275 instead of 18240.
    const frames: [number, number][] = Array.from({ length: 11 }, (_, i) => [1, 18275 + (i % 2)]);
    const out = feed(curve, 'fe', frames);
    assert.equal(out.kind, 'done');
    if (out.kind !== 'done') return;
    assert.equal(out.measured, 18275);
    assert.equal(out.offset, -35);
    assert.equal(out.samples.length, CALIBRATION.stableSamples);
    // With the offset the foil reads its own thickness.
    assert.equal(frequencyToMicrons('fe', 18275, curve, { fe: out.offset, al: 0 }), 90);
  });

  it('calibrates the Al channel and ignores the other channel and serial', () => {
    const frames: [number, number][] = [];
    for (let i = 0; i < 11; i += 1) frames.push([2, 26160], [1, 24073]);
    frames.splice(3, 0, [2, 26160]);
    const out = feed(curve, 'al', frames);
    assert.equal(out.kind, 'done');
    if (out.kind === 'done') assert.equal(out.offset, 30);
    let step = startCalibrationStep(curve, 'al', 0);
    const foreign = stepCalibration(step, frame(2, 26160, 1355), curve, 10);
    assert.equal(foreign.kind, 'progress');
    if (foreign.kind === 'progress') step = foreign.step;
    assert.equal(step.last, null);
  });

  it('counts unsteady and off-curve frames, failing at 10', () => {
    const jumping: [number, number][] = Array.from({ length: 12 }, (_, i) => [1, 18000 + (i % 2) * 50]);
    assert.deepEqual(feed(curve, 'fe', jumping), { kind: 'error', reason: 'unstable' });
    const air: [number, number][] = Array.from({ length: 12 }, () => [1, 22290]);
    assert.deepEqual(feed(curve, 'fe', air), { kind: 'error', reason: 'unstable' });
  });

  it('times out after 5 s', () => {
    const frames: [number, number][] = [[1, 18275], [1, 18275], [1, 18276]];
    assert.deepEqual(feed(curve, 'fe', frames, [0, 100, 5_001]), { kind: 'error', reason: 'timeout' });
  });

  it('reports progress', () => {
    const out = feed(curve, 'fe', [[1, 18275], [1, 18275], [1, 18276]]);
    assert.equal(out.kind, 'progress');
    if (out.kind === 'progress') assert.equal(calibrationProgress(out.step), 0.2);
  });

  it('uses a 150 µm target without calX', () => {
    const c = parseCurve('22291,23490,2,1,7\n0,15836,0,26391\n150,19000,150,26000\n3200,22002,3205,23947\n', 7);
    assert.equal(calibrationTargetFrequency(c, 'fe'), 19000);
  });
});

describe('stored calibrations', () => {
  const local: StoredCalibration = {
    serial: 32475,
    fe: -35,
    al: null,
    targetUm: 90,
    calibratedAt: '2026-10-23T10:00:00.000Z',
    samples: { fe: [18275], al: [] },
    clientId: 'c1',
    synced: false,
    by: null,
  };

  it('turns a calibration into offsets (0 for a channel never calibrated)', () => {
    assert.deepEqual(offsetsOf(local), { fe: -35, al: 0 });
    assert.equal(offsetsOf(null), null);
  });

  it('prefers the newer one, the backend on a tie', () => {
    const remote = fromBackendCalibration({
      uuid: 'u1',
      serial: 32475,
      fe_offset_hz: -20,
      al_offset_hz: 15,
      target_um: 90,
      samples: { fe: [], al: [] },
      calibrated_at: '2026-10-23T11:00:00.000Z',
      created_by: { name: 'Ada Usta' },
    });
    assert.equal(remote.synced, true);
    assert.equal(remote.by, 'Ada Usta');
    assert.equal(newerCalibration(local, remote), remote);
    assert.equal(newerCalibration({ ...local, calibratedAt: '2026-10-23T12:00:00.000Z' }, remote)?.clientId, 'c1');
    assert.equal(newerCalibration({ ...local, calibratedAt: remote.calibratedAt }, remote), remote);
    assert.equal(newerCalibration(null, remote), remote);
    assert.equal(newerCalibration(local, null), local);
  });

  it('builds the backend body', () => {
    assert.deepEqual(toBackendCalibration(local), {
      serial: 32475,
      fe_offset_hz: -35,
      al_offset_hz: null,
      target_um: 90,
      samples: { fe: [18275], al: [] },
      client_id: 'c1',
      calibrated_at: '2026-10-23T10:00:00.000Z',
    });
  });

  it('makes v4 uuids for the idempotency key', () => {
    const re = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u;
    assert.match(uuidV4(), re);
    assert.match(uuidV4(() => 0.999), re);
    assert.notEqual(uuidV4(), uuidV4());
  });
});
