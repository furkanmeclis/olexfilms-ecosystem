import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  filterStaff,
  listAllStaff,
  normalizeAmount,
  paymentTone,
  type StaffProfile,
  staffCreateInput,
  staffForm,
  staffFormErrors,
  staffPatchInput,
} from './lib.ts';

const card = (over: Partial<StaffProfile>): StaffProfile => ({
  uuid: 'u',
  user_uuid: null,
  name: 'Ayşe Kaya',
  title: 'Uygulayıcı',
  hired_on: null,
  monthly_salary: null,
  currency: 'TRY',
  active: true,
  created_at: '',
  updated_at: '',
  ...over,
});

describe('staff form', () => {
  it('normalizes amounts like the web', () => {
    assert.equal(normalizeAmount('25.000,5'), '25000.50');
    assert.equal(normalizeAmount('1500'), '1500.00');
    assert.equal(normalizeAmount('0'), null);
    assert.equal(normalizeAmount('abc'), null);
  });

  it('validates the fields', () => {
    assert.deepEqual(staffFormErrors({ ...staffForm(null), name: ' ', hired_on: '2026-13-01', monthly_salary: 'x' }), [
      'name',
      'hired_on',
      'monthly_salary',
    ]);
    assert.deepEqual(staffFormErrors({ ...staffForm(null), name: 'Ali', hired_on: '2026-01-31' }), []);
  });

  it('create omits empty optionals, patch clears them', () => {
    const v = { ...staffForm(null), name: ' Ali ', monthly_salary: '' };
    assert.deepEqual(staffCreateInput(v), { name: 'Ali', title: null, active: true });
    assert.deepEqual(staffPatchInput({ ...v, active: false }), {
      name: 'Ali',
      title: null,
      hired_on: null,
      monthly_salary: null,
      active: false,
    });
  });
});

describe('staff list', () => {
  it('reads every page', async () => {
    const calls: number[] = [];
    const items = await listAllStaff(async (limit, offset) => {
      calls.push(offset);
      const n = offset === 0 ? limit : 5;
      return { items: Array.from({ length: n }, (_, i) => card({ uuid: `s${offset + i}` })), total: limit + 5, limit, offset };
    });
    assert.equal(items.length, 105);
    assert.deepEqual(calls, [0, 100]);
  });

  it('filters by text and state', () => {
    const list = [card({ uuid: 'a' }), card({ uuid: 'b', name: 'Mehmet', title: null, active: false })];
    assert.deepEqual(filterStaff(list, 'meh', 'all').map((s) => s.uuid), ['b']);
    assert.deepEqual(filterStaff(list, '', 'active').map((s) => s.uuid), ['a']);
    assert.deepEqual(filterStaff(list, 'uygula', 'inactive'), []);
  });

  it('payment tones', () => {
    assert.equal(paymentTone('posted'), 'done');
    assert.equal(paymentTone('planned'), 'progress');
    assert.equal(paymentTone('cancelled'), 'neutral');
  });
});
