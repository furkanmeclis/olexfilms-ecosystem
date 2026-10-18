import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { canAddClaimPhoto, canSubmitClaim, claimActions, claimTone, isLiveClaim, statusParam, togglePart } from './lib.ts';

const none = { write: false, review: false, decide: false };

describe('claimActions', () => {
  it('lets the dealer submit an open claim', () => {
    assert.deepEqual(claimActions('open', { ...none, write: true }, 'dealer'), {
      submitReview: true,
      forward: false,
      decide: false,
      reopen: false,
    });
  });
  it('lets a distributor forward its own open claim, a dealer not', () => {
    assert.equal(claimActions('open', { ...none, review: true }, 'distributor').forward, true);
    assert.equal(claimActions('open', { ...none, review: true }, 'dealer').forward, false);
  });
  it('forwards dealer_review with the review grant', () => {
    assert.equal(claimActions('dealer_review', { ...none, review: true }, 'distributor').forward, true);
    assert.equal(claimActions('dealer_review', none, 'distributor').forward, false);
  });
  it('decides only in center_review', () => {
    assert.equal(claimActions('center_review', { ...none, decide: true }, 'center').decide, true);
    assert.equal(claimActions('dealer_review', { ...none, decide: true }, 'center').decide, false);
  });
  it('reopens a closed claim for super admins only', () => {
    assert.equal(claimActions('closed', { ...none, superAdmin: true }, 'center').reopen, true);
    assert.equal(claimActions('closed', { ...none, decide: true }, 'center').reopen, false);
    assert.equal(claimActions('approved', { ...none, superAdmin: true }, 'center').reopen, false);
  });
});

describe('claim helpers', () => {
  it('maps statuses to tones', () => {
    assert.equal(claimTone('approved'), 'done');
    assert.equal(claimTone('rejected'), 'problem');
    assert.equal(claimTone('center_review'), 'progress');
    assert.equal(claimTone('open'), 'neutral');
  });
  it('knows live claims', () => {
    assert.equal(isLiveClaim('open'), true);
    assert.equal(isLiveClaim('rejected'), false);
    assert.equal(isLiveClaim('closed'), false);
  });
  it('needs parts, description and a photo to submit', () => {
    assert.equal(canSubmitClaim({ parts: ['hood'], description: ' x ', photos: 1 }), true);
    assert.equal(canSubmitClaim({ parts: [], description: 'x', photos: 1 }), false);
    assert.equal(canSubmitClaim({ parts: ['hood'], description: '  ', photos: 1 }), false);
    assert.equal(canSubmitClaim({ parts: ['hood'], description: 'x', photos: 0 }), false);
  });
  it('adds photos only to open claims with write', () => {
    assert.equal(canAddClaimPhoto('open', true), true);
    assert.equal(canAddClaimPhoto('open', false), false);
    assert.equal(canAddClaimPhoto('dealer_review', true), false);
  });
  it('builds the status filter and toggles parts', () => {
    assert.equal(statusParam('all'), undefined);
    assert.equal(statusParam('approved'), 'approved');
    assert.deepEqual(togglePart(['a'], 'b'), ['a', 'b']);
    assert.deepEqual(togglePart(['a', 'b'], 'a'), ['b']);
  });
});
