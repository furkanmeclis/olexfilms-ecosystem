import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { notificationRoute, notificationTarget } from './lib.ts';

const U = '3f2b8c1e-5a4d-4e6f-9a7b-1c2d3e4f5a6b';

describe('notificationTarget', () => {
  it('reads the record from the web action link', () => {
    assert.deepEqual(notificationTarget({ action_url: `https://app.example.com/t/acme/orders/${U}` }), { kind: 'orders', uuid: U });
  });
  it('falls back to payload ids', () => {
    assert.deepEqual(notificationTarget({ payload: { service_uuid: U } }), { kind: 'services', uuid: U });
  });
  it('finds nothing in plain messages', () => {
    assert.equal(notificationTarget({ action_url: '/t/acme/settings' }), null);
    assert.equal(notificationTarget({}), null);
  });
});

describe('notificationRoute', () => {
  it('maps to the screens of the space', () => {
    assert.equal(notificationRoute({ action_url: `/t/acme/appointments/${U}` }, 'bayi'), `/bayi/menu/appointments/${U}`);
    assert.equal(notificationRoute({ action_url: `/t/acme/orders/${U}` }, 'depo'), `/depo/jobs/order/${U}`);
  });
  it('returns null when the space has no screen for it', () => {
    assert.equal(notificationRoute({ action_url: `/t/acme/appointments/${U}` }, 'admin'), null);
    assert.equal(notificationRoute({ action_url: `/t/acme/orders/${U}` }, null), null);
  });
});
