import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, it } from 'node:test';

import { BUNDLED_CURVES } from './bundled-curves.ts';
import { encryptCurveFile, resolveCurve } from './curve.ts';

const REAL_32475 = readFileSync(new URL('./fixtures/32475.dat', import.meta.url), 'latin1');

const CSV_1355 = `22291,23490,1,1,1355,0,0,0,0
0,15836,0,26391
90,18240,90,26190
3200,22002,3205,23947
`;

function fetcher(answers: (string | null | Error)[]) {
  const calls: number[] = [];
  const fn = async (serial: number) => {
    calls.push(serial);
    const a = answers.shift();
    if (a instanceof Error) throw a;
    return a ?? null;
  };
  return { fn, calls };
}

describe('resolveCurve (TEC-934)', () => {
  it('uses the backend file first', async () => {
    const f = fetcher([encryptCurveFile(CSV_1355, '0123456789abcdef')]);
    const r = await resolveCurve(1355, f.fn, BUNDLED_CURVES);
    assert.equal(r.source, 'backend');
    assert.equal(r.curve.serial, 1355);
    assert.equal(r.curve.deviceType, 1);
    assert.deepEqual(f.calls, [1355]);
  });

  it('decodes the real factory file served by the backend', async () => {
    const r = await resolveCurve(32475, fetcher([REAL_32475]).fn, {});
    assert.equal(r.source, 'backend');
    assert.deepEqual(r.curve, BUNDLED_CURVES[32475]);
  });

  it('falls back to the bundled curve on 404', async () => {
    const f = fetcher([null]);
    const r = await resolveCurve(32475, f.fn, BUNDLED_CURVES);
    assert.equal(r.source, 'bundled');
    assert.deepEqual(r.curve, BUNDLED_CURVES[32475]);
    assert.equal(f.calls.length, 1);
  });

  it('retries a network error once, then falls back', async () => {
    const f = fetcher([new Error('Network request failed'), new Error('Network request failed')]);
    const r = await resolveCurve(32475, f.fn, BUNDLED_CURVES);
    assert.equal(r.source, 'bundled');
    assert.equal(f.calls.length, 2);
  });

  it('recovers when the retry succeeds', async () => {
    const f = fetcher([new Error('timeout'), encryptCurveFile(CSV_1355, '0123456789abcdef')]);
    assert.equal((await resolveCurve(1355, f.fn, {})).source, 'backend');
  });

  it("refuses another serial's file and falls back", async () => {
    const f = fetcher([encryptCurveFile(CSV_1355, '0123456789abcdef')]);
    const r = await resolveCurve(32475, f.fn, BUNDLED_CURVES);
    assert.equal(r.source, 'bundled');
    assert.equal(f.calls.length, 1);
  });

  it('throws when neither source has the serial', async () => {
    await assert.rejects(resolveCurve(1355, fetcher([null]).fn, BUNDLED_CURVES), /curve_not_found/);
    await assert.rejects(resolveCurve(1355, fetcher(['garbage']).fn, BUNDLED_CURVES), /curve_format/);
  });
});
