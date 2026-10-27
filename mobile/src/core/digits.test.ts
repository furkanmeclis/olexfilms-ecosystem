import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { formatDigits, intlLocale, setDigitStyle, toLatinDigits } from './digits.ts';

describe('digits', () => {
  it('formats Arabic with Arabic-Indic digits by default and Latin by choice', () => {
    setDigitStyle('arab');
    assert.equal(new Intl.NumberFormat(intlLocale('ar')).format(1234), '١٬٢٣٤');
    assert.equal(intlLocale('tr'), 'tr');
    setDigitStyle('latn');
    assert.equal(new Intl.NumberFormat(intlLocale('ar')).format(12), '12');
    setDigitStyle('arab');
  });

  it('formats plain numbers for texts: years without grouping, Latin outside Arabic', () => {
    setDigitStyle('arab');
    assert.equal(formatDigits(2025, 'ar'), '٢٠٢٥');
    assert.equal(formatDigits(3, 'ar'), '٣');
    assert.equal(formatDigits(12345, 'ar'), '١٢٬٣٤٥');
    assert.equal(formatDigits(2025, 'tr'), '2025');
    setDigitStyle('latn');
    assert.equal(formatDigits(30, 'ar'), '30');
    setDigitStyle('arab');
  });

  it('normalizes typed digits to Latin', () => {
    assert.equal(toLatinDigits('٣٤ ABC ١٢'), '34 ABC 12');
    assert.equal(toLatinDigits('۱۲۳'), '123');
    assert.equal(toLatinDigits('WBA123'), 'WBA123');
  });
});
