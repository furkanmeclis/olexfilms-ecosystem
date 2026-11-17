import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, it } from 'node:test';

import { awaitingSignature, canCreateContract, CONTRACT_MODULE, contractState, contractWebUrl, showContract } from './contract.ts';

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

describe('contract module key', () => {
  it('is the backend intake_contracts module, not contracts (the card never showed)', () => {
    assert.equal(CONTRACT_MODULE, 'intake_contracts');
    const catalog = readFileSync(new URL('../../../../backend/internal/platform/features/catalog.go', import.meta.url), 'utf8');
    assert.match(catalog, /ModuleIntakeContracts\s*=\s*"intake_contracts"/);
  });

  it('is what the card asks useModule for', () => {
    const card = readFileSync(new URL('./contract-card.tsx', import.meta.url), 'utf8');
    assert.match(card, /useModule\(CONTRACT_MODULE\)/);
    assert.doesNotMatch(card, /useModule\('contracts'\)/);
  });
});
