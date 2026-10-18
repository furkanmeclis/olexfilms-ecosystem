import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  canVerify,
  cleanCode,
  isTransferCode,
  isTransferPhone,
  normalizePhone,
  pendingTransfer,
  transferErrorKey,
  verifyBody,
} from './transfer-lib.ts';

const open = { status: 'pending', from_verified: false, to_verified: false, new_owner_known: true };

describe('transfer codes', () => {
  it('accepts exactly six digits', () => {
    assert.equal(isTransferCode('123456'), true);
    assert.equal(isTransferCode(' 123456 '), true);
    assert.equal(isTransferCode('12345'), false);
    assert.equal(isTransferCode('12a456'), false);
  });
  it('cleans typed input', () => {
    assert.equal(cleanCode('12-34 56789'), '123456');
  });
});

describe('phone', () => {
  it('validates and normalizes', () => {
    assert.equal(isTransferPhone('+90 555 123 45 67'), true);
    assert.equal(isTransferPhone('12'), false);
    assert.equal(normalizePhone('+90 (555) 123-45 67'), '+905551234567');
  });
});

describe('pendingTransfer', () => {
  it('finds the open transfer', () => {
    assert.equal(pendingTransfer([{ status: 'completed' }, { status: 'pending' }])?.status, 'pending');
    assert.equal(pendingTransfer([{ status: 'cancelled' }]), undefined);
    assert.equal(pendingTransfer(undefined), undefined);
  });
});

describe('verifyBody', () => {
  it('sends only the open sides and the name for unknown owners', () => {
    const t = { ...open, from_verified: true, new_owner_known: false };
    assert.deepEqual(verifyBody(t, { fromCode: '111111', toCode: '222222', name: ' Ada ', surname: '' }), {
      to_code: '222222',
      new_owner_name: 'Ada',
    });
  });
});

describe('canVerify', () => {
  it('needs a valid code', () => {
    assert.equal(canVerify(open, { fromCode: '', toCode: '', name: '' }), false);
    assert.equal(canVerify(open, { fromCode: '123456', toCode: '', name: '' }), true);
  });
  it('needs the name once both sides are ready for an unknown owner', () => {
    const t = { ...open, new_owner_known: false };
    assert.equal(canVerify(t, { fromCode: '123456', toCode: '654321', name: '' }), false);
    assert.equal(canVerify(t, { fromCode: '123456', toCode: '654321', name: 'Ada' }), true);
    assert.equal(canVerify(t, { fromCode: '123456', toCode: '', name: '' }), true);
  });
});

describe('transferErrorKey', () => {
  it('carries the attempts left', () => {
    assert.deepEqual(transferErrorKey({ code: 'VEHICLE_TRANSFER_INVALID_CODE', details: [{ code: '2' }] }), {
      key: 'transfer_error.invalid_code',
      params: { count: 2 },
    });
  });
  it('maps codes and statuses', () => {
    assert.equal(transferErrorKey({ code: 'VEHICLE_TRANSFER_LOCKED' }).key, 'transfer_error.locked');
    assert.equal(transferErrorKey({ code: 'VALIDATION_ERROR', details: [{ field: 'phone' }] }).key, 'transfer_error.phone');
    assert.equal(transferErrorKey({ status: 429 }).key, 'transfer_error.rate');
    assert.equal(transferErrorKey({ status: 500 }).key, 'transfer_error.generic');
  });
});
