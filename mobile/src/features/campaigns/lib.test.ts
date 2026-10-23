import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  CAMPAIGN_STATUSES,
  campaignLocales,
  campaignStats,
  campaignTone,
  canCancel,
  canDecide,
  canSubmit,
  decisionBody,
  decisionReady,
} from './lib.ts';

describe('campaign status rules', () => {
  it('colors every status', () => {
    assert.deepEqual(
      CAMPAIGN_STATUSES.map(campaignTone),
      ['neutral', 'progress', 'done', 'progress', 'progress', 'done', 'problem', 'neutral', 'problem'],
    );
  });
  it('allows cancel until sent', () => {
    assert.deepEqual(
      CAMPAIGN_STATUSES.filter(canCancel),
      ['draft', 'pending_approval', 'approved', 'scheduled', 'sending'],
    );
  });
  it('submits drafts and decides pending approvals only', () => {
    assert.deepEqual(CAMPAIGN_STATUSES.filter(canSubmit), ['draft']);
    assert.deepEqual(CAMPAIGN_STATUSES.filter(canDecide), ['pending_approval']);
  });
});

describe('campaignStats', () => {
  it('counts pending and the success rate', () => {
    assert.deepEqual(
      campaignStats({ recipients_total: 10, recipients_sent: 6, recipients_failed: 2, recipients_skipped: 1 }),
      { pending: 1, successRate: 75 },
    );
    assert.deepEqual(
      campaignStats({ recipients_total: 0, recipients_sent: 0, recipients_failed: 0, recipients_skipped: 0 }),
      { pending: 0, successRate: null },
    );
  });
});

describe('approval decision', () => {
  it('needs a reason to reject or request changes', () => {
    assert.equal(decisionReady('approve', ''), true);
    assert.equal(decisionReady('reject', '  '), false);
    assert.equal(decisionReady('changes', 'Görsel eksik'), true);
  });
  it('builds the body', () => {
    assert.deepEqual(decisionBody('approve', ' '), {});
    assert.deepEqual(decisionBody('approve', ' ok '), { reason: 'ok' });
    assert.deepEqual(decisionBody('reject', ' spam '), { reason: 'spam' });
  });
  it('lists locales once, preview first', () => {
    assert.deepEqual(campaignLocales([{ locale: 'tr' }, { locale: 'en' }], ['en', 'de']), ['tr', 'en', 'de']);
  });
});
