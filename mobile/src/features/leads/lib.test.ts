import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  canMarkLost,
  followUpIso,
  isOverdue,
  leadFormErrors,
  leadName,
  leadStatusTone,
  leadTargetTypesFor,
  nextLeadStatuses,
  statusBody,
} from './lib.ts';

const base = { target_type: 'customer' as const, customer_uuid: '', company: '', contact: '', phone: '', email: '' };

describe('lead form', () => {
  it('needs a customer for customer leads', () => {
    assert.deepEqual(leadFormErrors(base), ['customer']);
    assert.deepEqual(leadFormErrors({ ...base, customer_uuid: 'x' }), []);
  });
  it('needs company and contact for candidates', () => {
    assert.deepEqual(leadFormErrors({ ...base, target_type: 'dealer_candidate' }), ['company', 'contact']);
    assert.deepEqual(leadFormErrors({ ...base, target_type: 'dealer_candidate', company: 'A', contact: 'B' }), []);
  });
  it('validates phone and e-mail when given', () => {
    const ok = { ...base, customer_uuid: 'x' };
    assert.deepEqual(leadFormErrors({ ...ok, phone: '0532' }), ['phone']);
    assert.deepEqual(leadFormErrors({ ...ok, phone: '+905321234567' }), []);
    assert.deepEqual(leadFormErrors({ ...ok, email: 'nope' }), ['email']);
    assert.deepEqual(leadFormErrors({ ...ok, email: 'a@b.co' }), []);
  });
  it('hides the distributor target from dealers', () => {
    assert.equal(leadTargetTypesFor('dealer').includes('distributor_candidate'), false);
    assert.equal(leadTargetTypesFor('center').includes('distributor_candidate'), true);
  });
});

describe('lead status', () => {
  it('lists every other status', () => {
    assert.deepEqual(nextLeadStatuses('quoted'), ['new', 'contacted', 'won', 'lost']);
  });
  it('sends the reason for lost only', () => {
    assert.deepEqual(statusBody('lost', '  price '), { status: 'lost', lost_reason: 'price' });
    assert.deepEqual(statusBody('won', 'x'), { status: 'won', lost_reason: null });
    assert.equal(canMarkLost('  '), false);
    assert.equal(canMarkLost('a'), true);
  });
  it('maps tones', () => {
    assert.equal(leadStatusTone('won'), 'done');
    assert.equal(leadStatusTone('lost'), 'problem');
    assert.equal(leadStatusTone('new'), 'neutral');
  });
  it('names a lead', () => {
    assert.equal(leadName({ target_type: 'customer', candidate_contact_name: 'Ali' }), 'Ali');
    assert.equal(
      leadName({ target_type: 'dealer_candidate', candidate_company_name: 'Acme', candidate_contact_name: 'Ali' }),
      'Acme',
    );
    assert.equal(leadName({ target_type: 'customer' }), '');
  });
});

describe('follow-up', () => {
  const now = new Date(2026, 9, 19, 15, 30);
  it('builds the iso date at 09:00 local', () => {
    const d = new Date(followUpIso(1, now) as string);
    assert.equal(d.getDate(), 20);
    assert.equal(d.getHours(), 9);
    assert.equal(followUpIso(null, now), null);
  });
  it('flags overdue open leads', () => {
    assert.equal(isOverdue(new Date(2026, 9, 18).toISOString(), 'new', now), true);
    assert.equal(isOverdue(new Date(2026, 9, 18).toISOString(), 'won', now), false);
    assert.equal(isOverdue(new Date(2026, 9, 25).toISOString(), 'new', now), false);
    assert.equal(isOverdue(undefined, 'new', now), false);
  });
});
