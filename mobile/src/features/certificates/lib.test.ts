import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { certificateTone, hasReason, isPendingCertificate, localizedName, personName, warningTone } from './lib.ts';

describe('localizedName', () => {
  it('prefers the locale, then tr, then en', () => {
    assert.equal(localizedName({ tr: 'Uygulayıcı', en: 'Installer' }, 'en'), 'Installer');
    assert.equal(localizedName({ tr: 'Uygulayıcı', en: 'Installer' }, 'de'), 'Uygulayıcı');
    assert.equal(localizedName({ en: 'Installer' }, 'de'), 'Installer');
  });
  it('reads a json string and a base64 json string', () => {
    const json = JSON.stringify({ tr: 'Usta', en: 'Master' });
    assert.equal(localizedName(json, 'en'), 'Master');
    assert.equal(localizedName(Buffer.from(json).toString('base64'), 'tr'), 'Usta');
  });
  it('keeps a plain name and handles empty', () => {
    assert.equal(localizedName('Plain', 'tr'), 'Plain');
    assert.equal(localizedName(null, 'tr'), '');
  });
});

describe('helpers', () => {
  it('joins the person name', () => {
    assert.equal(personName({ user_name: 'Ali', user_surname: 'Veli' }), 'Ali Veli');
    assert.equal(personName({ user_name: 'Ali' }), 'Ali');
  });
  it('maps states to tones', () => {
    assert.equal(certificateTone('valid'), 'done');
    assert.equal(certificateTone('pending'), 'progress');
    assert.equal(certificateTone('expired'), 'problem');
    assert.equal(warningTone('pending_approval'), 'progress');
    assert.equal(warningTone('none'), 'neutral');
  });
  it('validates the reject reason and finds the pending queue', () => {
    assert.equal(hasReason('  '), false);
    assert.equal(hasReason('blurry file'), true);
    assert.equal(isPendingCertificate({ status: 'pending' }), true);
  });
});
