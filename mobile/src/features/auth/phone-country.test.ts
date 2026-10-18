import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { DEFAULT_COUNTRY, listCountries, parsePhone } from './phone-country.ts';

describe('parsePhone', () => {
  it('normalizes Turkish national formats', () => {
    assert.equal(parsePhone('0532 123 45 67', 'TR'), '+905321234567');
    assert.equal(parsePhone('532-123-45-67', 'TR'), '+905321234567');
    assert.equal(parsePhone('(532) 123 4567', 'TR'), '+905321234567');
  });

  it('lets a pasted international number win over the selected country', () => {
    assert.equal(parsePhone('+44 7400 123456', 'TR'), '+447400123456');
  });

  it('parses other countries', () => {
    assert.equal(parsePhone('030 1234567', 'DE'), '+49301234567');
  });

  it('returns null for invalid or empty input', () => {
    assert.equal(parsePhone('123', 'TR'), null);
    assert.equal(parsePhone('', 'TR'), null);
  });
});

describe('listCountries', () => {
  const list = listCountries('tr');

  it('puts the default country first', () => {
    assert.equal(DEFAULT_COUNTRY, 'TR');
    assert.equal(list[0].code, 'TR');
  });

  it('localizes name, dial code and flag', () => {
    const de = list.find((c) => c.code === 'DE')!;
    assert.equal(de.name, 'Almanya');
    assert.equal(de.dial, '+49');
    assert.equal(de.flag, '🇩🇪');
  });

  it('uses English names for en and for unknown locales', () => {
    assert.equal(listCountries('en').find((c) => c.code === 'DE')!.name, 'Germany');
    assert.equal(listCountries('xx').find((c) => c.code === 'DE')!.name, 'Germany');
  });

  it('maps zh_CN to zh', () => {
    const zh = listCountries('zh_CN');
    assert.ok(zh.length > 200);
    assert.notEqual(zh.find((c) => c.code === 'DE')!.name, 'Germany');
  });

  it('has no duplicate codes', () => {
    assert.equal(new Set(list.map((c) => c.code)).size, list.length);
  });
});
