import type { components } from '@/core/api/schema';

type S = components['schemas'];
export type CariAccount = S['CariAccount'];
export type FinanceEntry = S['FinanceEntry'];
export type AccountingDispute = S['AccountingDispute'];
export type DisputeStatus = S['AccountingDisputeStatus'];
export type ResolveInput = S['AccountingDisputeResolveInput'];
export type Resolution = ResolveInput['resolution'];
export type Einvoice = S['Einvoice'];
export type EinvoiceStatus = Einvoice['status'];
export type EinvoiceValidation = Einvoice['validation_status'];
type Tone = 'neutral' | 'progress' | 'done' | 'problem';

export const DISPUTE_STATUSES: readonly DisputeStatus[] = ['open', 'resolved_reversal', 'resolved_revision', 'rejected'];
export const RESOLUTIONS: readonly Resolution[] = ['reversal', 'revision', 'reject'];
export const EINVOICE_STATUSES: readonly EinvoiceStatus[] = ['draft', 'failed', 'archived', 'voided'];
export const DISPUTE_TEXT_MAX = 2000;

/** Source types the web labels (accounting.sources.*); others show their raw key. */
export const ENTRY_SOURCE_TYPES: readonly string[] = [
  'manual',
  'order',
  'service',
  'transfer',
  'opening_balance',
  'account_opening',
  'staff_payment',
  'stock_transfer',
  'stock_return',
  'cari_transfer',
  'service_subscription_period',
  'service_subscription_cancel',
  'warranty_claim',
  'product_sale',
  'purchase_external',
  'service_income',
];

export type AccountingAccess = {
  canRead: boolean;
  /** accounting.dispute and a parent to dispute against (K24): only a child disputes. */
  canDispute: boolean;
  /** accounting.resolve in a center or distributor; the per-dispute check is canResolveDispute. */
  canResolve: boolean;
};

/** Same gates as the web's resolveAccountingAccess (writes stay on the web). */
export function accountingAccess(
  can: (permission: string) => boolean,
  orgType: string | null | undefined,
  parentUuid: string | null | undefined,
): AccountingAccess {
  const canRead = can('accounting.read');
  return {
    canRead,
    canDispute: canRead && !!parentUuid && can('accounting.dispute'),
    canResolve: canRead && (orgType === 'center' || orgType === 'distributor') && can('accounting.resolve'),
  };
}

/** The e-invoice screens open only in the brand center with einvoice.read (backend requireCenter). */
export function canReadEinvoices(can: (permission: string) => boolean, orgType: string | null | undefined) {
  return orgType === 'center' && can('einvoice.read');
}

/** Counterparty label key: organization type, or "customer" for a user cari. */
export function counterpartyKind(c: Pick<CariAccount, 'counterparty'>): string {
  if (c.counterparty.type === 'user') return 'customer';
  return c.counterparty.org_type ?? 'organization';
}

export type BalanceKind = 'receivable' | 'payable' | 'settled';

export function balanceKind(balance: string): BalanceKind {
  const v = Number(balance);
  if (v > 0) return 'receivable';
  if (v < 0) return 'payable';
  return 'settled';
}

export function balanceTone(kind: BalanceKind): Tone {
  if (kind === 'receivable') return 'done';
  if (kind === 'payable') return 'problem';
  return 'neutral';
}

function sourced(sourceType: string | null, sourceUuid: string | null) {
  return !!sourceType && sourceType !== 'manual' && !!sourceUuid;
}

/**
 * Mirrors OpenDispute (TEC-174): a sourced, original and still open row on the
 * active organization's cari with its parent.
 */
export function isEntryDisputable(entry: FinanceEntry, parentUuid: string | null | undefined): boolean {
  if (!parentUuid || !entry.cari_uuid) return false;
  if (entry.counterparty_organization?.uuid !== parentUuid) return false;
  if (entry.reversal_of_uuid || entry.voided || entry.reversed_by_uuid) return false;
  return sourced(entry.source_type, entry.source_uuid);
}

/** The parent the row came from resolves an open dispute, with accounting.resolve. */
export function canResolveDispute(d: AccountingDispute, orgUuid: string | undefined, canResolve: boolean): boolean {
  return canResolve && d.status === 'open' && !!orgUuid && d.counterparty_organization.uuid === orgUuid;
}

export function disputeTone(status: DisputeStatus): Tone {
  if (status === 'open') return 'progress';
  if (status === 'rejected') return 'problem';
  return 'done';
}

/**
 * Amount typed with a comma or a dot as the decimal mark; other separators are
 * grouping. Null when it is not a positive amount with at most two fraction digits.
 */
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

export type ResolveForm = { resolution: Resolution; corrected_amount: string; note: string };
export type ResolveErrors = { corrected_amount?: 'required' | 'amount'; note?: 'note_required' | 'too_long' };

/** reversal: optional note; revision: a positive corrected amount; reject: a note is required. */
export function validateResolve(v: ResolveForm): ResolveErrors {
  const errors: ResolveErrors = {};
  if (v.resolution === 'revision') {
    if (!v.corrected_amount.trim()) errors.corrected_amount = 'required';
    else if (normalizeAmount(v.corrected_amount) === null) errors.corrected_amount = 'amount';
  }
  if (v.note.length > DISPUTE_TEXT_MAX) errors.note = 'too_long';
  else if (v.resolution === 'reject' && !v.note.trim()) errors.note = 'note_required';
  return errors;
}

export function resolveInput(v: ResolveForm): ResolveInput {
  const note = v.note.trim();
  return {
    resolution: v.resolution,
    ...(note ? { note } : {}),
    ...(v.resolution === 'revision'
      ? { corrected_amount: normalizeAmount(v.corrected_amount) ?? v.corrected_amount.trim() }
      : {}),
  };
}

export function einvoiceTone(status: EinvoiceStatus): Tone {
  if (status === 'archived') return 'done';
  if (status === 'failed') return 'problem';
  if (status === 'voided') return 'progress';
  return 'neutral';
}

export function validationTone(status: EinvoiceValidation): Tone {
  if (status === 'valid') return 'done';
  return status === 'invalid' ? 'problem' : 'progress';
}

/** Archive and void stay on the web (step-up); true when the invoice still has one of them. */
export function hasWebOnlyActions(inv: Pick<Einvoice, 'status' | 'validation_status'>) {
  const archivable = (inv.status === 'draft' || inv.status === 'failed') && inv.validation_status !== 'invalid';
  return archivable || inv.status !== 'voided';
}

export type TimelineStep = { id: 'created' | 'failed' | 'archived' | 'voided'; at: string; note?: string | null };

/** Status history derived from the invoice, as on the web. */
export function einvoiceTimeline(
  inv: Pick<Einvoice, 'status' | 'number' | 'created_at' | 'updated_at' | 'issue_date' | 'error' | 'voided_at' | 'void_reason'>,
): TimelineStep[] {
  const steps: TimelineStep[] = [{ id: 'created', at: inv.created_at }];
  if (inv.status === 'failed') steps.push({ id: 'failed', at: inv.updated_at, note: inv.error });
  if (inv.number) steps.push({ id: 'archived', at: inv.issue_date, note: inv.number });
  if (inv.status === 'voided' && inv.voided_at) steps.push({ id: 'voided', at: inv.voided_at, note: inv.void_reason });
  return steps;
}

export function formatMoney(amount: string | number, currency: string, locale: string) {
  const v = typeof amount === 'number' ? amount : Number(amount);
  try {
    return new Intl.NumberFormat(locale, { style: 'currency', currency }).format(v);
  } catch {
    return `${v.toFixed(2)} ${currency}`;
  }
}
