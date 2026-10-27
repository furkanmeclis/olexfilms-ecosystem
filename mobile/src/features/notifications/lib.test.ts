import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { notificationExternalUrl, notificationRoute, notificationTarget } from './lib.ts';

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

// TEC-1034: customer rows carry web portal paths.
describe('customer portal links', () => {
  it('opens the customer screens', () => {
    assert.equal(notificationRoute({ action_url: `/portal/services/${U}#review` }, 'musteri'), `/musteri/service/review/${U}`);
    assert.equal(notificationRoute({ action_url: `/portal/services/${U}` }, 'musteri'), `/musteri/service/${U}`);
    assert.equal(notificationRoute({ action_url: '/portal/appointments', payload: { appointment_uuid: U } }, 'musteri'), '/musteri/appointments');
    assert.equal(notificationRoute({ action_url: '/portal/warranties', payload: { claim_uuid: 'c', warranty_uuid: U } }, 'musteri'), '/musteri/profile/claims');
    assert.equal(notificationRoute({ action_url: '/portal/warranties' }, 'musteri'), '/musteri/warranties');
    assert.equal(notificationRoute({ action_url: '/portal' }, 'musteri'), '/musteri');
    assert.equal(notificationRoute({ action_url: 'https://olexfilms.app/portal' }, 'musteri'), '/musteri');
  });
  it('opens an unmapped web link in the browser for customers only', () => {
    const quote = { action_url: 'https://olexfilms.app/s/AbCdEfGhIj', payload: { quote_uuid: U } };
    assert.equal(notificationRoute(quote, 'musteri'), null);
    assert.equal(notificationExternalUrl(quote, 'musteri'), quote.action_url);
    assert.equal(notificationExternalUrl(quote, 'bayi'), null);
    assert.equal(notificationExternalUrl({ action_url: '/portal' }, 'musteri'), null);
  });
});
