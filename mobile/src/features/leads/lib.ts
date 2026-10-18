export type LeadStatus = 'new' | 'contacted' | 'quoted' | 'won' | 'lost';
export type LeadTemperature = 'cold' | 'warm' | 'hot';
export type LeadTargetType = 'customer' | 'dealer_candidate' | 'distributor_candidate';
export type LeadSource =
  | 'incoming_call'
  | 'outgoing_call'
  | 'walk_in'
  | 'whatsapp'
  | 'social'
  | 'referral'
  | 'website'
  | 'application_form'
  | 'other';
export type LeadTone = 'neutral' | 'progress' | 'done' | 'problem';

export const LEAD_STATUSES: readonly LeadStatus[] = ['new', 'contacted', 'quoted', 'won', 'lost'];
export const LEAD_TEMPERATURES: readonly LeadTemperature[] = ['cold', 'warm', 'hot'];
export const LEAD_SOURCES: readonly LeadSource[] = [
  'incoming_call',
  'outgoing_call',
  'walk_in',
  'whatsapp',
  'social',
  'referral',
  'website',
  'application_form',
  'other',
];

/** A dealer never recruits a distributor, so that target is center / distributor only. */
export function leadTargetTypesFor(orgType: string | undefined): LeadTargetType[] {
  const all: LeadTargetType[] = ['customer', 'dealer_candidate', 'distributor_candidate'];
  return orgType === 'dealer' ? all.filter((x) => x !== 'distributor_candidate') : all;
}

export function leadStatusTone(s: LeadStatus): LeadTone {
  if (s === 'won') return 'done';
  if (s === 'lost') return 'problem';
  if (s === 'quoted' || s === 'contacted') return 'progress';
  return 'neutral';
}

export function leadTemperatureTone(t: LeadTemperature): LeadTone {
  if (t === 'hot') return 'problem';
  if (t === 'warm') return 'progress';
  return 'neutral';
}

/** Statuses a lead can move to from the current one. */
export function nextLeadStatuses(s: LeadStatus): LeadStatus[] {
  return LEAD_STATUSES.filter((x) => x !== s);
}

/** Lost needs a reason; the others send none. */
export function statusBody(status: LeadStatus, lostReason: string) {
  return { status, lost_reason: status === 'lost' ? lostReason.trim() : null };
}

export function canMarkLost(reason: string): boolean {
  return reason.trim().length > 0;
}

export function leadName(l: {
  target_type: LeadTargetType;
  candidate_company_name?: string | null;
  candidate_contact_name?: string | null;
}): string {
  if (l.target_type === 'customer') return l.candidate_contact_name || '';
  return l.candidate_company_name || l.candidate_contact_name || '';
}

export type LeadForm = {
  target_type: LeadTargetType;
  customer_uuid: string;
  company: string;
  contact: string;
  phone: string;
  email: string;
};

/** Field keys that fail validation (empty when the form can be sent). */
export function leadFormErrors(v: LeadForm): string[] {
  const out: string[] = [];
  if (v.target_type === 'customer') {
    if (!v.customer_uuid) out.push('customer');
  } else {
    if (!v.company.trim()) out.push('company');
    if (!v.contact.trim()) out.push('contact');
  }
  const phone = v.phone.trim();
  if (phone && !/^\+[1-9]\d{6,14}$/.test(phone)) out.push('phone');
  const email = v.email.trim();
  if (email && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) out.push('email');
  return out;
}

/** Follow-up chip → ISO date at 09:00 local, `null` for none. */
export function followUpIso(days: number | null, now: Date = new Date()): string | null {
  if (days === null) return null;
  const d = new Date(now);
  d.setDate(d.getDate() + days);
  d.setHours(9, 0, 0, 0);
  return d.toISOString();
}

/** A follow-up date that has passed while the lead is still open. */
export function isOverdue(followUp: string | undefined | null, status: LeadStatus, now: Date = new Date()): boolean {
  if (!followUp || status === 'won' || status === 'lost') return false;
  return new Date(followUp).getTime() < now.getTime();
}
