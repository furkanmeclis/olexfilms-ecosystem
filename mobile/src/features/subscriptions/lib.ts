import type { components } from '@/core/api/schema';
import type { StatusTone } from '@/ui/status-pill';

type S = components['schemas'];
export type Subscription = S['ServiceSubscription'];
export type SubscriptionStatus = S['ServiceSubscriptionStatus'];
export type CancelQueueItem = S['ServiceSubscriptionCancelQueueItem'];
export type CancelRequestStatus = CancelQueueItem['status'];

export const SUBSCRIPTION_STATUSES: SubscriptionStatus[] = ['scheduled', 'active', 'cancel_requested', 'cancelled', 'expired'];
export const CANCEL_REQUEST_STATUSES: CancelRequestStatus[] = ['pending', 'approved', 'rejected'];

export function subscriptionTone(status: SubscriptionStatus): StatusTone {
  if (status === 'active') return 'done';
  if (status === 'cancel_requested') return 'progress';
  if (status === 'cancelled') return 'problem';
  return 'neutral';
}

export function cancelRequestTone(status: CancelRequestStatus): StatusTone {
  if (status === 'pending') return 'progress';
  if (status === 'approved') return 'done';
  return 'problem';
}

/**
 * Early cancellation is requested by the subscriber organization for its own
 * active subscription (web: canRequestCancel; the API answers 404 / 409 otherwise).
 */
export function canRequestCancel(
  sub: Pick<Subscription, 'organization_uuid' | 'status'>,
  activeOrgUuid: string | undefined,
  hasPermission: boolean,
) {
  return hasPermission && !!activeOrgUuid && sub.organization_uuid === activeOrgUuid && sub.status === 'active';
}

/** A cancellation reason is required (1–5000 characters); a rejection needs a note. */
export function validReason(reason: string) {
  const r = reason.trim();
  return r.length > 0 && r.length <= 5000;
}

/** Decision body: an approval note is optional (null when empty). */
export function decisionBody(note: string) {
  return { note: note.trim() || null };
}
