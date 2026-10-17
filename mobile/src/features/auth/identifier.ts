import { parsePhoneNumberFromString } from 'libphonenumber-js/min';

export type Identifier = { kind: 'email'; email: string } | { kind: 'phone'; e164: string };

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * One box for every user: an e-mail address or a phone number. Phones are
 * normalized to E.164 (K29); a number without a country code is read as
 * Turkish until the backend's identify step tells us the user's country.
 */
export function parseIdentifier(raw: string): Identifier | null {
  const v = raw.trim();
  if (EMAIL.test(v)) return { kind: 'email', email: v.toLowerCase() };
  const phone = parsePhoneNumberFromString(v, 'TR');
  if (phone?.isValid()) return { kind: 'phone', e164: phone.number };
  return null;
}
