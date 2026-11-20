import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { createScanQueue, ScanTimeoutError, type ScanSettled } from './scan-queue.ts';

type Deferred = { resolve: (v: string) => void; reject: (e: unknown) => void; signal: AbortSignal };

function harness(opts: { timeoutMs?: number; maxPending?: number } = {}) {
  let clock = 1_000;
  const calls: { code: string; d: Deferred }[] = [];
  const settled: { code: string; r: ScanSettled<string> }[] = [];
  const states: { active: string | null; pending: number }[] = [];
  const q = createScanQueue<string>({
    run: (code, signal) =>
      new Promise<string>((resolve, reject) => {
        calls.push({ code, d: { resolve, reject, signal } });
      }),
    onSettled: (code, r) => settled.push({ code, r }),
    onChange: (s) => states.push(s),
    now: () => clock,
    timeoutMs: opts.timeoutMs ?? 0,
    maxPending: opts.maxPending,
  });
  return { q, calls, settled, states, tick: (ms: number) => (clock += ms) };
}

const flush = () => new Promise((r) => setImmediate(r));

describe('createScanQueue (TEC-1255)', () => {
  it('keeps codes that arrive while one is running and handles them in order', async () => {
    const h = harness();
    assert.equal(h.q.push('A'), 'queued');
    assert.equal(h.q.push('B'), 'queued');
    assert.equal(h.q.push('C'), 'queued');
    assert.deepEqual(h.q.state(), { active: 'A', pending: 2 });
    assert.deepEqual(h.calls.map((c) => c.code), ['A']);

    h.calls[0].d.resolve('a');
    await flush();
    assert.deepEqual(h.calls.map((c) => c.code), ['A', 'B']);
    assert.deepEqual(h.q.state(), { active: 'B', pending: 1 });

    h.calls[1].d.reject(new Error('nope'));
    await flush();
    h.calls[2].d.resolve('c');
    await flush();
    assert.deepEqual(h.settled.map((s) => [s.code, s.r.ok]), [
      ['A', true],
      ['B', false],
      ['C', true],
    ]);
    assert.deepEqual(h.q.state(), { active: null, pending: 0 });
    assert.deepEqual(h.states.at(-1), { active: null, pending: 0 });
  });

  it('drops the same code within 2.5 s unless repeat is off, and accepts it after', async () => {
    const h = harness();
    h.q.push('A');
    h.calls[0].d.resolve('a');
    await flush();
    h.tick(1000);
    assert.equal(h.q.push('A'), 'repeat');
    assert.equal(h.q.push('A', { repeat: false }), 'queued');
    h.calls[1].d.resolve('a');
    await flush();
    h.tick(2600);
    assert.equal(h.q.push('A'), 'queued');
    assert.equal(h.q.push('  '), 'empty');
    h.calls[2].d.resolve('a');
    await flush();
    h.tick(100);
    assert.equal(h.q.push('A'), 'repeat');
    h.q.resetRepeat();
    assert.equal(h.q.push('A'), 'queued');
    assert.equal(h.calls.length, 4);
  });

  it('does not queue a camera repeat of a code still running or waiting', () => {
    const h = harness();
    h.q.push('A');
    h.q.push('B');
    h.tick(5000);
    assert.equal(h.q.push('A'), 'repeat');
    assert.equal(h.q.push('B'), 'repeat');
    assert.equal(h.q.push('B', { repeat: false }), 'queued');
  });

  it('gives up on a request that does not answer and moves on', async () => {
    const h = harness({ timeoutMs: 20 });
    h.q.push('A');
    h.q.push('B');
    await new Promise((r) => setTimeout(r, 40));
    assert.equal(h.settled[0].code, 'A');
    assert.equal(h.settled[0].r.ok, false);
    assert.ok(!h.settled[0].r.ok && h.settled[0].r.error instanceof ScanTimeoutError);
    assert.equal(h.calls[0].d.signal.aborted, true);
    assert.equal(h.q.state().active, 'B');
    // A late answer for A is ignored.
    h.calls[0].d.resolve('late');
    h.calls[1].d.resolve('b');
    await flush();
    assert.deepEqual(h.settled.map((s) => s.code), ['A', 'B']);
  });

  it('holdTimeout lets a code wait for the user past the limit', async () => {
    const h = harness({ timeoutMs: 20 });
    h.q.push('A');
    h.q.holdTimeout();
    await new Promise((r) => setTimeout(r, 40));
    assert.equal(h.settled.length, 0);
    h.calls[0].d.resolve('a');
    await flush();
    assert.equal(h.settled.length, 1);
  });

  it('clear drops waiting codes and the running result', async () => {
    const h = harness();
    h.q.push('A');
    h.q.push('B');
    h.q.clear();
    assert.equal(h.calls[0].d.signal.aborted, true);
    assert.deepEqual(h.q.state(), { active: null, pending: 0 });
    h.calls[0].d.resolve('a');
    await flush();
    assert.equal(h.settled.length, 0);
    assert.equal(h.q.push('A'), 'queued');
    assert.deepEqual(h.calls.map((c) => c.code), ['A', 'A']);
  });

  it('refuses codes beyond the waiting limit', () => {
    const h = harness({ maxPending: 2 });
    h.q.push('A');
    h.q.push('B');
    h.q.push('C');
    assert.equal(h.q.push('D'), 'full');
  });

  it('keeps going when the result handler throws or the runner throws synchronously', async () => {
    let n = 0;
    const seen: string[] = [];
    const q = createScanQueue<string>({
      run: (code) => {
        if (code === 'X') throw new Error('sync');
        return Promise.resolve(code);
      },
      onSettled: (code) => {
        seen.push(code);
        n += 1;
        if (n === 1) throw new Error('handler');
      },
      timeoutMs: 0,
    });
    const error = console.error;
    console.error = () => {};
    q.push('X');
    q.push('Y');
    await flush();
    await flush();
    console.error = error;
    assert.deepEqual(seen, ['X', 'Y']);
  });
});
