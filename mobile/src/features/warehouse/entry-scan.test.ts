import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { entryLineBarcode } from './entry-scan.ts';

describe('entryLineBarcode (TEC-1224)', () => {
  const lines = [{ barcode: 'OLX-00000012' }, { barcode: 'GZ-ab-7' }];
  it('matches a scanned code case-insensitively and returns the line spelling', () => {
    assert.equal(entryLineBarcode(lines, 'olx-00000012'), 'OLX-00000012');
    assert.equal(entryLineBarcode(lines, ' GZ-AB-7 '), 'GZ-ab-7');
  });
  it('finds nothing for an unknown or empty code', () => {
    assert.equal(entryLineBarcode(lines, 'OLX-1'), undefined);
    assert.equal(entryLineBarcode(lines, '  '), undefined);
  });
});
