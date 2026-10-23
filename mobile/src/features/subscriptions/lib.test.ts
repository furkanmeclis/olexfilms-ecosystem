import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { cancelRequestTone, canRequestCancel, decisionBody, subscriptionTone, validReason } from './lib.ts';

describe('service subscriptions', () => {
  it('only the subscriber requests the cancellation of an active subscription', () => {
    const sub = { organization_uuid: 'o1', status: 'active' as const };
    assert.equal(canRequestCancel(sub, 'o1', true), true);
    assert.equal(canRequestCancel(sub, 'o2', true), false);
    assert.equal(canRequestCancel(sub, 'o1', false), false);
    assert.equal(canRequestCancel({ ...sub, status: 'cancel_requested' }, 'o1', true), false);
    assert.equal(canRequestCancel(sub, undefined, true), false);
  });

  it('validates reasons and builds decisions', () => {
    assert.equal(validReason('  '), false);
    assert.equal(validReason('x'.repeat(5001)), false);
    assert.equal(validReason('too expensive'), true);
    assert.deepEqual(decisionBody('  '), { note: null });
    assert.deepEqual(decisionBody(' ok '), { note: 'ok' });
  });

  it('tones', () => {
    assert.equal(subscriptionTone('active'), 'done');
    assert.equal(subscriptionTone('cancel_requested'), 'progress');
    assert.equal(subscriptionTone('cancelled'), 'problem');
    assert.equal(subscriptionTone('expired'), 'neutral');
    assert.equal(cancelRequestTone('pending'), 'progress');
    assert.equal(cancelRequestTone('rejected'), 'problem');
  });
});
