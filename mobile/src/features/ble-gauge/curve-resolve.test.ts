import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, it } from 'node:test';

import { BUNDLED_CURVES } from './bundled-curves.ts';
import { type CurveResponse, encryptCurveFile, parseCurve, resolveCurve } from './curve.ts';

const REAL_32475 = readFileSync(new URL('./fixtures/32475.dat', import.meta.url), 'latin1');

const CSV_1355 = `22291,23490,1,1,1355,0,0,0,0
0,15836,0,26391
90,18240,90,26190
3200,22002,3205,23947
`;

type Answer = string | null | Error | 304;

/** Backend stub: a string is a 200 (ETag `e-<length>`), null a 404, 304 not modified, an Error a network failure. */
function fetcher(answers: Answer[]) {
  const calls: number[] = [];
  const etags: (string | null)[] = [];
  const fn = async (serial: number, etag: string | null): Promise<CurveResponse> => {
    calls.push(serial);
    etags.push(etag);
    const a = answers.shift();
    if (a instanceof Error) throw a;
    if (a === 304) return { status: 'not_modified' };
    if (a == null) return { status: 'not_found' };
    return { status: 'ok', text: a, etag: `e-${a.length}` };
  };
  return { fn, calls, etags };
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

  // TEC-939: the stored copy is revalidated with its ETag on every connect.
  it('sends the stored ETag and keeps the copy on 304', async () => {
    const stored = { curve: BUNDLED_CURVES[32475]!, etag: 'abc' };
    const f = fetcher([304]);
    const r = await resolveCurve(32475, f.fn, {}, stored);
    assert.equal(r.source, 'cache');
    assert.equal(r.etag, 'abc');
    assert.deepEqual(f.etags, ['abc']);
  });

  it('replaces the stored copy when the factory file changed', async () => {
    const changed = CSV_1355.replace('90,18240', '90,18250');
    const stored = { curve: parseCurve(CSV_1355, 1355), etag: 'old' };
    const r = await resolveCurve(1355, fetcher([encryptCurveFile(changed, '0123456789abcdef')]).fn, {}, stored);
    assert.equal(r.source, 'backend');
    assert.notEqual(r.etag, 'old');
    assert.equal(r.curve.fe[1]!.frequency, 18250);
  });

  it('uses the stored copy offline, on 404 and when the backend fails', async () => {
    const stored = { curve: parseCurve(CSV_1355, 1355), etag: 'x' };
    const off = fetcher([]);
    assert.equal((await resolveCurve(1355, off.fn, {}, stored, false)).source, 'cache');
    assert.equal(off.calls.length, 0);
    assert.equal((await resolveCurve(1355, fetcher([null]).fn, {}, stored)).source, 'cache');
    assert.equal((await resolveCurve(1355, fetcher([new Error('a'), new Error('b')]).fn, {}, stored)).source, 'cache');
    assert.equal((await resolveCurve(1355, fetcher(['garbage']).fn, {}, stored)).source, 'cache');
  });

  it("ignores a stored copy of another serial and asks without an ETag", async () => {
    const f = fetcher([encryptCurveFile(CSV_1355, '0123456789abcdef')]);
    const r = await resolveCurve(1355, f.fn, {}, { curve: BUNDLED_CURVES[32475]!, etag: 'other' });
    assert.equal(r.curve.serial, 1355);
    assert.deepEqual(f.etags, [null]);
  });

  it('offline without a stored copy uses the bundled curve', async () => {
    assert.equal((await resolveCurve(32475, fetcher([]).fn, BUNDLED_CURVES, null, false)).source, 'bundled');
  });
});
