import type { components } from '@/core/api/schema';

/** Campaign rules shared by the mobile screens (same as the web's features/campaigns/lib). */
type S = components['schemas'];
export type Campaign = S['Campaign'];
export type CampaignStatus = S['CampaignStatus'];
export type CampaignChannel = S['CampaignChannel'];
export type CampaignTone = 'neutral' | 'progress' | 'done' | 'problem';
export type Decision = 'approve' | 'changes' | 'reject';

export const CAMPAIGN_STATUSES: readonly CampaignStatus[] = [
  'draft',
  'pending_approval',
  'approved',
  'scheduled',
  'sending',
  'sent',
  'partially_failed',
  'cancelled',
  'rejected',
];

export const CAMPAIGN_CHANNELS: readonly CampaignChannel[] = ['push', 'whatsapp', 'email'];

export function campaignTone(status: CampaignStatus): CampaignTone {
  switch (status) {
    case 'sent':
    case 'approved':
      return 'done';
    case 'pending_approval':
    case 'scheduled':
    case 'sending':
      return 'progress';
    case 'partially_failed':
    case 'rejected':
      return 'problem';
    default:
      return 'neutral';
  }
}

/** Before sending a campaign can still be cancelled (TEC-406). */
export function canCancel(status: CampaignStatus) {
  return ['draft', 'pending_approval', 'approved', 'scheduled', 'sending'].includes(status);
}

/** A draft goes to its approver (a center's own submission is approved at once). */
export function canSubmit(status: CampaignStatus) {
  return status === 'draft';
}

/** Approve / reject / request changes apply to a campaign waiting for approval. */
export function canDecide(status: CampaignStatus) {
  return status === 'pending_approval';
}

/** Pending recipients and the success rate of the sent ones (null: none attempted). */
export function campaignStats(c: Pick<Campaign, 'recipients_total' | 'recipients_sent' | 'recipients_failed' | 'recipients_skipped'>) {
  const done = c.recipients_sent + c.recipients_failed + c.recipients_skipped;
  const pending = Math.max(0, c.recipients_total - done);
  const attempted = c.recipients_sent + c.recipients_failed;
  const successRate = attempted > 0 ? Math.round((c.recipients_sent / attempted) * 100) : null;
  return { pending, successRate };
}

/** Rejecting or asking for changes needs a reason; approving does not. */
export function decisionReady(decision: Decision, reason: string) {
  return decision === 'approve' || reason.trim().length > 0;
}

/** Body of the decision request (approve sends the reason only when given). */
export function decisionBody(decision: Decision, reason: string): { reason?: string } {
  const r = reason.trim();
  if (decision === 'approve') return r ? { reason: r } : {};
  return { reason: r };
}

/** Locales the campaign reaches or has content for, preview order first, no duplicates. */
export function campaignLocales(previewLocales: readonly { locale: string }[], contentLocales: readonly string[]) {
  return [...new Set([...previewLocales.map((l) => l.locale), ...contentLocales])];
}
