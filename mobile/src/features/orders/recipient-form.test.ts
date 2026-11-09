import assert from 'node:assert/strict';
import { test } from 'node:test';

import { EMPTY_RECIPIENT, toRecipientBody, validateRecipient } from './recipient-form.ts';

test('name and phone are required', () => {
  assert.deepEqual(validateRecipient(EMPTY_RECIPIENT, null), { name: 'required', phone: 'required' });
});

test('a typed phone that does not parse is invalid', () => {
  const v = { ...EMPTY_RECIPIENT, name: 'Oto Kaplama', phone: '123' };
  assert.deepEqual(validateRecipient(v, null), { phone: 'phone_invalid' });
});

test('tax number is 10 (VKN) or 11 (TCKN) digits when given', () => {
  const base = { ...EMPTY_RECIPIENT, name: 'A', phone: '05551112233' };
  assert.deepEqual(validateRecipient({ ...base, tax_no: '123 456 7890' }, '+905551112233'), {});
  assert.deepEqual(validateRecipient({ ...base, tax_no: '12345678901' }, '+905551112233'), {});
  assert.deepEqual(validateRecipient({ ...base, tax_no: '12345' }, '+905551112233'), { tax_no: 'tax_no_invalid' });
});

test('body is trimmed and leaves empty optional fields out', () => {
  const body = toRecipientBody(
    { ...EMPTY_RECIPIENT, kind: 'retail_point', name: '  Teslim Noktası ', phone: '0555', city: ' İzmir ', tax_no: ' ' },
    '+905551112233',
  );
  assert.deepEqual(body, { kind: 'retail_point', name: 'Teslim Noktası', phone: '+905551112233', city: 'İzmir' });
});
