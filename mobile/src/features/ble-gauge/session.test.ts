import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { createGeneration, createSerialConfirm, poweredOnTimeoutError, poweredOnWaitMs } from './session.ts';

describe('createGeneration', () => {
  it('marks a connect attempt stale after a disconnect', () => {
    const g = createGeneration();
    const ticket = g.next();
    assert.ok(g.isCurrent(ticket));
    g.bump(); // disconnect / screen closed while connectToDevice is pending
    assert.ok(!g.isCurrent(ticket));
  });

  it('marks an older attempt stale when a newer one starts', () => {
    const g = createGeneration();
    const first = g.next();
    const second = g.next();
    assert.ok(!g.isCurrent(first));
    assert.ok(g.isCurrent(second));
  });
});

describe('createSerialConfirm', () => {
  it('needs two consecutive frames with the same serial', () => {
    const c = createSerialConfirm(2);
    assert.equal(c.push(32475), null);
    assert.equal(c.push(32475), 32475);
    assert.equal(c.push(32475), 32475);
  });

  it('ignores a single corrupted serial between good frames', () => {
    const c = createSerialConfirm(2);
    c.push(32475);
    assert.equal(c.push(99999), null); // corrupted frame
    assert.equal(c.push(32475), null); // streak restarts
    assert.equal(c.push(32475), 32475);
  });

  it('starts over after reset', () => {
    const c = createSerialConfirm(2);
    c.push(1355);
    c.reset();
    assert.equal(c.push(1355), null);
  });
});

describe('adapter wait', () => {
  it('waits long while the adapter is Unknown (iOS permission prompt)', () => {
    assert.equal(poweredOnWaitMs(null), 60_000);
    assert.equal(poweredOnWaitMs('Unknown'), 60_000);
    assert.equal(poweredOnWaitMs('Resetting'), 60_000);
    assert.equal(poweredOnWaitMs('PoweredOff'), 6_000);
  });

  it('reports a stuck adapter as not ready, not as powered off', () => {
    assert.equal(poweredOnTimeoutError('Unknown'), 'not_ready');
    assert.equal(poweredOnTimeoutError(null), 'not_ready');
    assert.equal(poweredOnTimeoutError('PoweredOff'), 'powered_off');
  });
});
