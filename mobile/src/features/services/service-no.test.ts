import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { extractServiceNo, isValidServiceNo } from './service-no.ts';

describe('service number', () => {
  it('takes the number out of the warranty card URL', () => {
    assert.equal(extractServiceNo('https://olexfilms.app/warranty/1EVMEEI9'), '1EVMEEI9');
    assert.equal(extractServiceNo(' https://olexfilms.app/warranty/DS12AB34CD/pdf?x=1 '), 'DS12AB34CD');
    assert.equal(extractServiceNo('olexfilms.app/warranty/AB-12_cd'), 'AB-12_cd');
    assert.equal(extractServiceNo(' DS12AB34CD '), 'DS12AB34CD');
  });

  it('checks the shape', () => {
    assert.ok(isValidServiceNo('2025D2333D014-CC-210'));
    assert.ok(!isValidServiceNo('abc'));
    assert.ok(!isValidServiceNo('2025 3207'));
    assert.ok(!isValidServiceNo('1EVMEEİ9'));
  });
});
