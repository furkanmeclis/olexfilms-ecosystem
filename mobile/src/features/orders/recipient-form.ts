/**
 * New order recipient (TEC-1062 / TEC-1063): the center opens a dealer candidate
 * or a retail / delivery point in the waiting pool from the new order form.
 * Pure rules mirrored from POST /v1/tenant/pending-recipients. No `@/` imports:
 * node --test runs this file directly.
 */

export type RecipientKind = 'dealer_candidate' | 'retail_point';

export type RecipientForm = {
  kind: RecipientKind;
  name: string;
  phone: string;
  city: string;
  district: string;
  address: string;
  tax_no: string;
  tax_office: string;
};

export type RecipientField = 'name' | 'phone' | 'tax_no';
export type RecipientError = 'required' | 'phone_invalid' | 'tax_no_invalid' | 'too_long';
export type RecipientErrors = Partial<Record<RecipientField, RecipientError>>;

export type RecipientBody = {
  kind: RecipientKind;
  name: string;
  phone: string;
  city?: string;
  district?: string;
  address?: string;
  tax_no?: string;
  tax_office?: string;
};

export const NAME_MAX = 200;

export const EMPTY_RECIPIENT: RecipientForm = {
  kind: 'dealer_candidate',
  name: '',
  phone: '',
  city: '',
  district: '',
  address: '',
  tax_no: '',
  tax_office: '',
};

/** 10 digit VKN or 11 digit TCKN; spaces are ignored. */
export function normalizeTaxNo(raw: string): string {
  return raw.replace(/\s+/g, '');
}

/**
 * Validates the form. `phoneE164` is the phone already parsed by the caller
 * (libphonenumber lives outside this file): null when the typed number is not valid.
 */
export function validateRecipient(v: RecipientForm, phoneE164: string | null): RecipientErrors {
  const errors: RecipientErrors = {};
  const name = v.name.trim();
  if (!name) errors.name = 'required';
  else if (name.length > NAME_MAX) errors.name = 'too_long';
  if (!v.phone.trim()) errors.phone = 'required';
  else if (!phoneE164) errors.phone = 'phone_invalid';
  const tax = normalizeTaxNo(v.tax_no);
  if (tax && !/^(\d{10}|\d{11})$/.test(tax)) errors.tax_no = 'tax_no_invalid';
  return errors;
}

/** Request body: trimmed, empty optional fields left out. */
export function toRecipientBody(v: RecipientForm, phoneE164: string): RecipientBody {
  const body: RecipientBody = { kind: v.kind, name: v.name.trim(), phone: phoneE164 };
  const opt = (s: string) => s.trim() || undefined;
  const city = opt(v.city);
  const district = opt(v.district);
  const address = opt(v.address);
  const tax = normalizeTaxNo(v.tax_no) || undefined;
  const office = opt(v.tax_office);
  if (city) body.city = city;
  if (district) body.district = district;
  if (address) body.address = address;
  if (tax) body.tax_no = tax;
  if (office) body.tax_office = office;
  return body;
}
