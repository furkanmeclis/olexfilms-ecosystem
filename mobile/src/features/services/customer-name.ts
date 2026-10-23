/** Customer fields the pickers and summaries show (TEC-867). */
export type NamedCustomer = {
  name: string;
  surname?: string | null;
  phone?: string | null;
  type?: string | null;
  company_name?: string | null;
};

const person = (c: NamedCustomer) => [c.name, c.surname].filter(Boolean).join(' ');

/** Company of a corporate customer, or null. */
export function customerCompany(c: NamedCustomer): string | null {
  const company = c.company_name?.trim();
  if (!company) return null;
  return !c.type || c.type === 'corporate' ? company : null;
}

/** Title: the company name for a corporate customer, otherwise the person. */
export function customerTitle(c: NamedCustomer): string {
  return customerCompany(c) ?? (person(c) || '—');
}

/** Subtitle: the contact person (corporate) and the phone. */
export function customerSubtitle(c: NamedCustomer): string {
  const contact = customerCompany(c) ? person(c) : '';
  return [contact, c.phone].filter(Boolean).join(' · ');
}
