import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { ensureDigitSupport, formatDigits, intlLocale, setDigitStyle, toLatinDigits } from './digits.ts';

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

  it('converts formatter output when the engine ignores the -u-nu- extension', () => {
    class LatinOnly {
      format(n: number) {
        return n.toLocaleString('en-US');
      }
      formatToParts(n: number) {
        return [{ type: 'integer', value: String(n) }];
      }
    }
    const intl = { NumberFormat: LatinOnly, DateTimeFormat: LatinOnly } as unknown as typeof Intl;
    ensureDigitSupport(intl);
    assert.equal(new intl.NumberFormat('ar-u-nu-arab').format(1422), '١٬٤٢٢');
    assert.equal(new intl.NumberFormat('ar-u-nu-latn').format(1422), '1,422');
    assert.equal(new intl.NumberFormat('tr').format(1422), '1,422');
    assert.equal(new intl.DateTimeFormat('ar-u-nu-arab').format(2026 as never), '٢,٠٢٦');
    assert.deepEqual(new intl.NumberFormat('ar-u-nu-arab').formatToParts(12), [{ type: 'integer', value: '١٢' }]);
  });

  it('leaves engines that honor the extension alone', () => {
    const before = Intl.NumberFormat;
    ensureDigitSupport();
    assert.equal(Intl.NumberFormat, before);
  });

  it('normalizes typed digits to Latin', () => {
    assert.equal(toLatinDigits('٣٤ ABC ١٢'), '34 ABC 12');
    assert.equal(toLatinDigits('۱۲۳'), '123');
    assert.equal(toLatinDigits('WBA123'), 'WBA123');
  });
});
