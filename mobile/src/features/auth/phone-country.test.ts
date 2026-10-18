import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  DEFAULT_COUNTRY,
  detectCountry,
  examplePhone,
  filterCountries,
  listCountries,
  parsePhone,
} from './phone-country.ts';

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

describe('filterCountries', () => {
  const tr = listCountries('tr');
  const codes = (q: string) => filterCountries(tr, q).map((c) => c.code);

  it('returns everything for an empty query', () => {
    assert.equal(filterCountries(tr, '  ').length, tr.length);
  });

  it('matches names ignoring case and accents', () => {
    assert.ok(codes('turkiye').includes('TR'));
    assert.ok(codes('ALMAN').includes('DE'));
    assert.ok(codes('birlesik').includes('GB'));
    assert.ok(codes('isvec').includes('SE'));
  });

  it('matches the ISO code', () => {
    assert.ok(codes('gb').includes('GB'));
  });

  it('matches calling-code digits with or without +', () => {
    assert.ok(codes('44').includes('GB'));
    assert.ok(codes('+90').includes('TR'));
    assert.ok(!codes('+90').includes('GB'));
  });
});

describe('detectCountry', () => {
  it('reads the country of a complete international number', () => {
    assert.equal(detectCountry('+44 7400 123456'), 'GB');
  });

  it('ignores national and partial input', () => {
    assert.equal(detectCountry('0532 123 45 67'), undefined);
    assert.equal(detectCountry('+44 74'), undefined);
  });
});

describe('examplePhone', () => {
  it('gives a national-format sample', () => {
    assert.match(examplePhone('TR') ?? '', /^0?5\d\d \d{3} \d\d \d\d$/);
  });
});
