import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { createShakeDetector, type Sample, SHAKE_SAMPLE_INTERVAL_MS } from './shake.ts';

const DT = SHAKE_SAMPLE_INTERVAL_MS;

/** Samples at 20 Hz from a magnitude function (all on the z axis). */
function series(ms: number, mag: (t: number) => number, t0 = 0): Sample[] {
  const out: Sample[] = [];
  for (let t = t0; t < t0 + ms; t += DT) out.push({ x: 0, y: 0, z: mag(t), t });
  return out;
}

function fires(samples: Sample[]) {
  const d = createShakeDetector();
  return samples.filter((s) => d.push(s)).map((s) => s.t);
}

/** A vigorous shake: ~5 Hz, peaks around 3 g (troughs near 0 g). */
const shake = (t: number) => 1.5 + 1.6 * Math.sin((2 * Math.PI * 5 * t) / 1000);

describe('shake detection', () => {
  it('fires on a vigorous shake', () => {
    assert.ok(fires(series(1200, shake)).length >= 1);
  });

  it('stays quiet at rest', () => {
    assert.deepEqual(fires(series(5000, () => 1)), []);
  });

  it('ignores walking (about 2 steps/s, up to 1.6 g)', () => {
    const walk = (t: number) => 1 + 0.6 * Math.max(0, Math.sin((2 * Math.PI * 2 * t) / 1000));
    assert.deepEqual(fires(series(10_000, walk)), []);
  });

  it('ignores brisk walking with occasional jolts up to 2 g', () => {
    const walk = (t: number) => 1 + 1.0 * Math.max(0, Math.sin((2 * Math.PI * 2.5 * t) / 1000));
    assert.deepEqual(fires(series(10_000, walk)), []);
  });

  it('ignores putting the phone on a table (impact and one bounce)', () => {
    const table = (t: number) => (t === 1000 ? 3.2 : t === 1100 ? 2.6 : t === 1150 ? 0.4 : 1);
    assert.deepEqual(fires(series(4000, table)), []);
  });

  it('needs separate peaks: one long high reading is a single peak', () => {
    assert.deepEqual(fires(series(3000, (t) => (t >= 500 && t < 2500 ? 3 : 1))), []);
  });

  it('does not count peaks spread over more than the window', () => {
    // A 2.8 g spike every 500 ms: never 4 within 1.2 s.
    assert.deepEqual(fires(series(10_000, (t) => (t % 500 === 0 ? 2.8 : 1))), []);
  });

  it('waits for the cooldown before firing again', () => {
    const d = createShakeDetector({ cooldownMs: 3000 });
    const hits = series(6000, shake).filter((s) => d.push(s)).map((s) => s.t);
    assert.ok(hits.length >= 2, 'fires again after the cooldown');
    for (let i = 1; i < hits.length; i++) assert.ok(hits[i] - hits[i - 1] >= 3000);
  });

  it('ignores non-finite samples', () => {
    const d = createShakeDetector();
    assert.equal(d.push({ x: NaN, y: 0, z: 0, t: 0 }), false);
    assert.equal(d.push({ x: Infinity, y: 0, z: 0, t: 50 }), false);
  });
});
