import type { StatusTone } from '@/ui/status-pill';

export type LocalizedText = Record<string, string>;

export type Certificate = {
  uuid: string;
  user_name?: string;
  user_surname?: string;
  type_name?: LocalizedText | string | null;
  organization_name?: string;
  status: string;
  issued_at?: string | null;
  expires_at?: string | null;
  verified_at?: string | null;
  reject_reason?: string | null;
  created_at: string;
};

export type CertificateWarning = {
  uuid: string;
  service_no?: string;
  user_name?: string;
  user_surname?: string;
  type_name?: LocalizedText | string | null;
  organization_name?: string;
  reason: string;
  decision: string;
  note?: string | null;
  created_at: string;
  decided_at?: string | null;
};

/** The API sends a jsonb column as base64 bytes in some rows; accept both shapes. */
function parseName(value: string): LocalizedText | string {
  const tryJson = (s: string): LocalizedText | null => {
    try {
      const v: unknown = JSON.parse(s);
      return v && typeof v === 'object' ? (v as LocalizedText) : null;
    } catch {
      return null;
    }
  };
  const direct = tryJson(value);
  if (direct) return direct;
  if (/^[A-Za-z0-9+/]+={0,2}$/.test(value) && value.length % 4 === 0) {
    try {
      const decoded = tryJson(atob(value));
      if (decoded) return decoded;
    } catch {
      // not base64: plain text name
    }
  }
  return value;
}

/** Name in the locale, else tr, en or the first one. */
export function localizedName(value: LocalizedText | string | null | undefined, locale: string): string {
  if (!value) return '';
  const names = typeof value === 'string' ? parseName(value) : value;
  if (typeof names === 'string') return names;
  return names[locale] ?? names[locale.replace('-', '_')] ?? names.tr ?? names.en ?? Object.values(names)[0] ?? '';
}

export function personName(row: { user_name?: string | null; user_surname?: string | null }) {
  return [row.user_name, row.user_surname].filter(Boolean).join(' ');
}

export function certificateTone(status: string): StatusTone {
  if (status === 'valid') return 'done';
  if (status === 'pending') return 'progress';
  if (status === 'rejected' || status === 'revoked' || status === 'expired') return 'problem';
  return 'neutral';
}

export function warningTone(decision: string): StatusTone {
  if (decision === 'approved') return 'done';
  if (decision === 'pending_approval') return 'progress';
  if (decision === 'rejected') return 'problem';
  return 'neutral';
}

/** A reject needs a reason (the API refuses an empty one). */
export const hasReason = (reason: string) => reason.trim().length > 0;

export const isPendingCertificate = (c: Pick<Certificate, 'status'>) => c.status === 'pending';
export const isPendingWarning = (w: Pick<CertificateWarning, 'decision'>) => w.decision === 'pending_approval';
