import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { awaitingSignature, canCreateContract, contractState, contractWebUrl, showContract } from './contract.ts';

describe('contract (TEC-1083)', () => {
  it('maps the summary to a display state', () => {
    assert.equal(contractState(null), 'none');
    assert.equal(contractState({ status: 'pending' }), 'pending');
    assert.equal(contractState({ status: 'executed' }), 'executed');
  });

  it('shows the card when a contract exists or is required', () => {
    assert.equal(showContract({ contract: null, contract_required: false }), false);
    assert.equal(showContract({ contract: null, contract_required: true }), true);
    assert.equal(showContract({ contract: { status: 'executed' } }), true);
  });

  it('creates a contract only for a draft or pending service without a live one', () => {
    assert.equal(canCreateContract({ status: 'draft', contract: null }), true);
    assert.equal(canCreateContract({ status: 'pending', contract: { status: 'voided' } }), true);
    assert.equal(canCreateContract({ status: 'pending', contract: { status: 'pending' } }), false);
    assert.equal(canCreateContract({ status: 'processing', contract: null }), false);
  });

  it('waits for signatures on draft and pending contracts', () => {
    assert.equal(awaitingSignature({ status: 'pending' }), true);
    assert.equal(awaitingSignature({ status: 'executed' }), false);
    assert.equal(awaitingSignature(null), false);
  });

  it('links the web wizard of the service', () => {
    assert.equal(contractWebUrl('https://x.tr', 'afm florya', 'u-1'), 'https://x.tr/t/afm%20florya/services/u-1/wizard');
  });
});
