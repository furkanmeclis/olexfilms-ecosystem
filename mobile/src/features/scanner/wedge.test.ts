import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { splitWedgeInput } from './wedge.ts';

describe('splitWedgeInput (TEC-1230)', () => {
  it('takes the codes before each terminator and keeps the rest', () => {
    assert.deepEqual(splitWedgeInput('OLEX-00000001\n'), {
      codes: ['OLEX-00000001'],
      rest: '',
    });
    assert.deepEqual(splitWedgeInput('A1\r\nB2\tC3'), {
      codes: ['A1', 'B2'],
      rest: 'C3',
    });
    assert.deepEqual(splitWedgeInput('PART'), { codes: [], rest: 'PART' });
  });

  it('drops empty and blank codes', () => {
    assert.deepEqual(splitWedgeInput('\n \n'), { codes: [], rest: '' });
  });
});
