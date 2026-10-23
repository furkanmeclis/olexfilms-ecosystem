import type { components } from '@/core/api/schema';
import type { StatusTone } from '@/ui/status-pill';

type S = components['schemas'];
export type StaffProfile = S['StaffProfile'];
export type StaffCreateInput = S['StaffProfileCreateInput'];
export type StaffPatchInput = S['StaffProfilePatchInput'];
export type StaffPayment = S['StaffPayment'];

type Page<T> = { items: T[]; total: number; limit: number; offset: number };

/** Largest page the API serves (apiquery.MaxLimit). */
export const STAFF_PAGE_LIMIT = 100;
const MAX_PAGES = 100;

/**
 * Every staff card of the book (web: listAll). There is no single-card GET;
 * the list has no server search, so the screens filter it in memory.
 */
export async function listAllStaff(fetchPage: (limit: number, offset: number) => Promise<Page<StaffProfile>>) {
  const items: StaffProfile[] = [];
  for (let page = 0; page < MAX_PAGES; page++) {
    const res = await fetchPage(STAFF_PAGE_LIMIT, page * STAFF_PAGE_LIMIT);
    items.push(...res.items);
    if (res.items.length < STAFF_PAGE_LIMIT || items.length >= res.total) break;
  }
  return items;
}

/** Case-insensitive name / title search of the in-memory list. */
export function filterStaff(items: readonly StaffProfile[], q: string, active: 'all' | 'active' | 'inactive') {
  const needle = q.trim().toLocaleLowerCase();
  return items.filter(
    (s) =>
      (active === 'all' || s.active === (active === 'active')) &&
      (!needle || `${s.name} ${s.title ?? ''}`.toLocaleLowerCase().includes(needle)),
  );
}

export function paymentTone(status: StaffPayment['status']): StatusTone {
  if (status === 'posted') return 'done';
  if (status === 'planned') return 'progress';
  return 'neutral';
}

/** Amount text to the API form `1234.50`, or null when empty / invalid / not positive (web: normalizeAmount). */
export function normalizeAmount(raw: string): string | null {
  const value = raw.replace(/[\s  ']/g, '');
  if (!/^[0-9.,]+$/.test(value)) return null;
  const match = /^(.*?)(?:[.,](\d{1,2}))?$/.exec(value);
  if (!match) return null;
  const intPart = (match[1] ?? '').replace(/[.,]/g, '');
  const frac = match[2] ?? '';
  if (!/^\d{1,16}$/.test(intPart)) return null;
  const amount = `${intPart.replace(/^0+(?=\d)/, '')}.${frac.padEnd(2, '0')}`;
  return Number(amount) > 0 ? amount : null;
}

export type StaffForm = { name: string; title: string; hired_on: string; monthly_salary: string; active: boolean };

export function staffForm(s: StaffProfile | null | undefined): StaffForm {
  return {
    name: s?.name ?? '',
    title: s?.title ?? '',
    hired_on: s?.hired_on ?? '',
    monthly_salary: s?.monthly_salary ?? '',
    active: s?.active ?? true,
  };
}

const DATE_RE = /^\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])$/;

export type StaffFormError = 'name' | 'title' | 'hired_on' | 'monthly_salary';

export function staffFormErrors(v: StaffForm): StaffFormError[] {
  const errors: StaffFormError[] = [];
  const name = v.name.trim();
  if (!name || name.length > 200) errors.push('name');
  if (v.title.trim().length > 100) errors.push('title');
  if (v.hired_on.trim() && !DATE_RE.test(v.hired_on.trim())) errors.push('hired_on');
  if (v.monthly_salary.trim() && !normalizeAmount(v.monthly_salary)) errors.push('monthly_salary');
  return errors;
}

export function staffCreateInput(v: StaffForm): StaffCreateInput {
  const salary = normalizeAmount(v.monthly_salary);
  const hired = v.hired_on.trim();
  return {
    name: v.name.trim(),
    title: v.title.trim() || null,
    ...(hired ? { hired_on: hired } : {}),
    ...(salary ? { monthly_salary: salary } : {}),
    active: v.active,
  };
}

/** PATCH body: cleared optional fields are sent as null. */
export function staffPatchInput(v: StaffForm): StaffPatchInput {
  return {
    name: v.name.trim(),
    title: v.title.trim() || null,
    hired_on: v.hired_on.trim() || null,
    monthly_salary: normalizeAmount(v.monthly_salary),
    active: v.active,
  };
}
