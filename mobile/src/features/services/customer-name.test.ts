import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { customerCompany, customerSubtitle, customerTitle } from './customer-name.ts';

describe('customer name', () => {
  it('shows an individual by name and phone', () => {
    const c = { name: 'Ayşe', surname: 'Yılmaz', phone: '+905551234567', type: 'individual', company_name: 'Eski' };
    assert.equal(customerCompany(c), null);
    assert.equal(customerTitle(c), 'Ayşe Yılmaz');
    assert.equal(customerSubtitle(c), '+905551234567');
  });

  it('shows a corporate customer under the company with the contact below', () => {
    const c = { name: 'Ali', surname: 'Kaya', phone: '+905557654321', type: 'corporate', company_name: ' Olex Ltd ' };
    assert.equal(customerTitle(c), 'Olex Ltd');
    assert.equal(customerSubtitle(c), 'Ali Kaya · +905557654321');
  });

  it('trusts company_name when the type is unknown', () => {
    assert.equal(customerTitle({ name: 'Ali', company_name: 'Olex Ltd' }), 'Olex Ltd');
    assert.equal(customerTitle({ name: '', company_name: '  ' }), '—');
  });
});
