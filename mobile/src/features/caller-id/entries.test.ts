import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { buildPhoneBook, toDigits } from './entries.ts';

describe('toDigits', () => {
  it('drops the plus sign of an E.164 number', () => {
    assert.equal(toDigits('+905321112233'), '905321112233');
    assert.equal(toDigits(' +4915112345678 '), '4915112345678');
  });
  it('refuses anything that is not E.164', () => {
    for (const bad of ['', '+', '05321112233', '+90 532 111 22 33', '+90532111223344556', '+12345', 'abc']) {
      assert.equal(toDigits(bad), null, bad);
    }
  });
});

describe('buildPhoneBook', () => {
  it('writes unique lines in ascending numeric order', () => {
    const { tsv, count } = buildPhoneBook([
      { phone: '+905321112233', label: 'Ahmet Yılmaz' },
      { phone: '+4915112345678', label: 'Hans Müller' },
      { phone: '+12025550123', label: 'Jane Doe' },
      { phone: '+905321112233', label: 'Duplicate' },
    ]);
    assert.equal(count, 3);
    // 11 digits before 12 before 13: numeric order, not text order.
    assert.equal(tsv, '12025550123\tJane Doe\n905321112233\tAhmet Yılmaz\n4915112345678\tHans Müller\n');
  });
  it('keeps a label on one line', () => {
    const { tsv } = buildPhoneBook([{ phone: '+905321112233', label: ' Ahmet\tYılmaz\n(ABC) ' }]);
    assert.equal(tsv, '905321112233\tAhmet Yılmaz (ABC)\n');
  });
  it('leaves out entries without a number or a label', () => {
    const { tsv, count } = buildPhoneBook([
      { phone: '0532', label: 'Bad number' },
      { phone: '+905321112233', label: '   ' },
    ]);
    assert.equal(count, 0);
    assert.equal(tsv, '');
  });
});
