import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { deleteErrorKey, isLegalKind, keepsCodeStep, LEGAL_KINDS, legalTextUrl, pendingConsent } from './lib.ts';

describe('legal texts (TEC-1257)', () => {
  it('knows only the public kinds', () => {
    assert.deepEqual([...LEGAL_KINDS], ['privacy_policy', 'kvkk_notice', 'terms_of_use']);
    assert.ok(isLegalKind('kvkk_notice'));
    assert.ok(!isLegalKind('ai_guidelines'));
    assert.ok(!isLegalKind(undefined));
  });

  it('builds the public URL with the locale', () => {
    assert.equal(
      legalTextUrl('https://x.test/api/', 'terms_of_use', 'zh-CN'),
      'https://x.test/api/v1/public/legal-texts/terms_of_use?locale=zh-CN',
    );
  });
});

describe('consent after sign-in', () => {
  it('asks the AI guidelines only', () => {
    const ai = { kind: 'ai_guidelines', locale: 'tr', version: 2, body: 'x' };
    assert.equal(pendingConsent([{ kind: 'marketing_consent', locale: 'tr', version: 1, body: 'y' }, ai]), ai);
    assert.equal(pendingConsent([{ kind: 'marketing_consent', locale: 'tr', version: 1, body: 'y' }]), null);
    assert.equal(pendingConsent([]), null);
    assert.equal(pendingConsent(undefined), null);
  });
});

describe('account deletion errors', () => {
  it('maps the API codes to messages', () => {
    assert.equal(deleteErrorKey('INVALID_OTP_CODE'), 'invalid_code');
    assert.equal(deleteErrorKey('OTP_LOCKED'), 'otp_locked');
    assert.equal(deleteErrorKey('RATE_LIMITED'), 'rate_limited');
    assert.equal(deleteErrorKey('OTP_DELIVERY_FAILED'), 'delivery_failed');
    assert.equal(deleteErrorKey('ACCOUNT_HAS_NO_PHONE'), 'no_phone');
    assert.equal(deleteErrorKey('CUSTOMER_HAS_PANEL_ACCOUNT'), 'panel_account');
    assert.equal(deleteErrorKey('INTERNAL_ERROR'), 'failed');
    assert.equal(deleteErrorKey(undefined), 'failed');
  });

  it('keeps the code step only for a wrong code', () => {
    assert.ok(keepsCodeStep('INVALID_OTP_CODE'));
    assert.ok(!keepsCodeStep('OTP_LOCKED'));
  });
});
