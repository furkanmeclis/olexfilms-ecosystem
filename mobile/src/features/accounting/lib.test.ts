import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  accountingAccess,
  balanceKind,
  canReadEinvoices,
  canResolveDispute,
  counterpartyKind,
  einvoiceTimeline,
  hasWebOnlyActions,
  isEntryDisputable,
  normalizeAmount,
  resolveInput,
  validateResolve,
  type AccountingDispute,
  type FinanceEntry,
} from './lib.ts';

const PARENT = 'p-1';

function entry(over: Partial<FinanceEntry> = {}): FinanceEntry {
  return {
    uuid: 'e-1',
    direction: 'charge',
    category: 'sale',
    category_label_key: 'accounting.category.sale',
    account: null,
    cari_uuid: 'c-1',
    counterparty_organization: { uuid: PARENT, name: 'Merkez' },
    orig_currency: 'TRY',
    orig_amount: '100.00',
    currency: 'TRY',
    amount: '100.00',
    rate: '1',
    rate_date: '2026-10-01',
    source_type: 'order',
    source_uuid: 's-1',
    revision: 1,
    reversal_of_uuid: null,
    reversed_by_uuid: null,
    voided: false,
    description: null,
    created_at: '2026-10-01T08:00:00Z',
    ...over,
  };
}

function dispute(over: Partial<AccountingDispute> = {}): AccountingDispute {
  return {
    uuid: 'd-1',
    status: 'open',
    organization: { uuid: 'child', name: 'Bayi' },
    counterparty_organization: { uuid: PARENT, name: 'Merkez' },
    entry: {
      uuid: 'e-1',
      direction: 'charge',
      category: 'sale',
      category_label_key: 'accounting.category.sale',
      orig_currency: 'TRY',
      orig_amount: '100.00',
      currency: 'TRY',
      amount: '100.00',
      revision: 1,
      created_at: '2026-10-01T08:00:00Z',
    },
    source_type: 'order',
    source_uuid: 's-1',
    reason: 'Tutar yanlış',
    corrected_amount: null,
    resolution_note: null,
    reversal_entry_uuid: null,
    revision_entry_uuid: null,
    created_at: '2026-10-02T08:00:00Z',
    resolved_at: null,
    ...over,
  };
}

const grants = (...p: string[]) => (k: string) => p.includes(k);

describe('accountingAccess', () => {
  it('lets a child with a parent dispute, never the center', () => {
    const can = grants('accounting.read', 'accounting.dispute');
    assert.equal(accountingAccess(can, 'dealer', PARENT).canDispute, true);
    assert.equal(accountingAccess(can, 'center', null).canDispute, false);
    assert.equal(accountingAccess(grants('accounting.dispute'), 'dealer', PARENT).canDispute, false);
  });
  it('lets only a center or distributor resolve', () => {
    const can = grants('accounting.read', 'accounting.resolve');
    assert.equal(accountingAccess(can, 'distributor', PARENT).canResolve, true);
    assert.equal(accountingAccess(can, 'dealer', PARENT).canResolve, false);
  });
  it('opens e-invoices only in the center', () => {
    assert.equal(canReadEinvoices(grants('einvoice.read'), 'center'), true);
    assert.equal(canReadEinvoices(grants('einvoice.read'), 'distributor'), false);
    assert.equal(canReadEinvoices(grants(), 'center'), false);
  });
});

describe('cari', () => {
  it('labels the counterparty and the balance side', () => {
    assert.equal(counterpartyKind({ counterparty: { type: 'user', uuid: null, name: 'A' } }), 'customer');
    assert.equal(counterpartyKind({ counterparty: { type: 'organization', uuid: 'x', name: 'B', org_type: 'dealer' } }), 'dealer');
    assert.equal(balanceKind('12.50'), 'receivable');
    assert.equal(balanceKind('-1'), 'payable');
    assert.equal(balanceKind('0.00'), 'settled');
  });
});

describe('isEntryDisputable', () => {
  it('accepts a sourced open row of the parent cari', () => {
    assert.equal(isEntryDisputable(entry(), PARENT), true);
  });
  it('rejects manual, reversed, foreign or parentless rows', () => {
    assert.equal(isEntryDisputable(entry({ source_type: 'manual' }), PARENT), false);
    assert.equal(isEntryDisputable(entry({ voided: true }), PARENT), false);
    assert.equal(isEntryDisputable(entry({ reversal_of_uuid: 'r' }), PARENT), false);
    assert.equal(isEntryDisputable(entry({ counterparty_organization: { uuid: 'other', name: 'X' } }), PARENT), false);
    assert.equal(isEntryDisputable(entry(), null), false);
  });
});

describe('disputes', () => {
  it('lets the addressed parent resolve an open dispute', () => {
    assert.equal(canResolveDispute(dispute(), PARENT, true), true);
    assert.equal(canResolveDispute(dispute(), 'child', true), false);
    assert.equal(canResolveDispute(dispute({ status: 'rejected' }), PARENT, true), false);
    assert.equal(canResolveDispute(dispute(), PARENT, false), false);
  });
  it('validates the resolve form like the web', () => {
    assert.deepEqual(validateResolve({ resolution: 'reversal', corrected_amount: '', note: '' }), {});
    assert.deepEqual(validateResolve({ resolution: 'reject', corrected_amount: '', note: ' ' }), { note: 'note_required' });
    assert.deepEqual(validateResolve({ resolution: 'revision', corrected_amount: '', note: '' }), { corrected_amount: 'required' });
    assert.deepEqual(validateResolve({ resolution: 'revision', corrected_amount: 'abc', note: '' }), { corrected_amount: 'amount' });
  });
  it('builds the resolve body', () => {
    assert.deepEqual(resolveInput({ resolution: 'revision', corrected_amount: '1.200,5', note: ' ok ' }), {
      resolution: 'revision',
      note: 'ok',
      corrected_amount: '1200.50',
    });
    assert.deepEqual(resolveInput({ resolution: 'reversal', corrected_amount: '9', note: '' }), { resolution: 'reversal' });
  });
  it('normalizes amounts', () => {
    assert.equal(normalizeAmount('1.234,56'), '1234.56');
    assert.equal(normalizeAmount('0'), null);
    assert.equal(normalizeAmount('12.345'), '12345.00');
  });
});

describe('einvoice', () => {
  const base = {
    status: 'draft' as const,
    number: null,
    created_at: '2026-10-01T08:00:00Z',
    updated_at: '2026-10-01T08:00:00Z',
    issue_date: '2026-10-01',
    error: null,
    voided_at: null,
    void_reason: null,
  };
  it('derives the status history', () => {
    assert.deepEqual(einvoiceTimeline(base).map((s) => s.id), ['created']);
    const voided = { ...base, status: 'voided' as const, number: 'EAR2026000000001', voided_at: '2026-10-02T09:00:00Z', void_reason: 'x' };
    assert.deepEqual(einvoiceTimeline(voided).map((s) => s.id), ['created', 'archived', 'voided']);
  });
  it('flags the web-only actions', () => {
    assert.equal(hasWebOnlyActions({ status: 'voided', validation_status: 'valid' }), false);
    assert.equal(hasWebOnlyActions({ status: 'archived', validation_status: 'valid' }), true);
  });
});
