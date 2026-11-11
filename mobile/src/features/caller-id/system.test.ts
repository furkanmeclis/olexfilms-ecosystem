import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { toSystem } from './system.ts';

describe('toSystem', () => {
  it('iOS is ready when the extension is switched on', () => {
    assert.deepEqual(toSystem('ios', { supported: true, extension: 'enabled' }), {
      supported: true,
      steps: [{ id: 'ios_extension', done: true }],
      ready: true,
    });
    assert.equal(toSystem('ios', { supported: true, extension: 'disabled' }).ready, false);
    assert.equal(toSystem('ios', { supported: true, extension: 'unknown' }).ready, false);
  });
  it('Android needs the role and the overlay permission', () => {
    const half = toSystem('android', { supported: true, role: true, overlay: false });
    assert.deepEqual(half.steps, [
      { id: 'android_role', done: true },
      { id: 'android_overlay', done: false },
    ]);
    assert.equal(half.ready, false);
    assert.equal(toSystem('android', { supported: true, role: true, overlay: true }).ready, true);
  });
  it('an unsupported device has no steps', () => {
    assert.deepEqual(toSystem('android', { supported: false }), { supported: false, steps: [], ready: false });
  });
});
