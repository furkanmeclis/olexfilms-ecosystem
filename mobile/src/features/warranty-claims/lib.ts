export type ClaimStatus = 'open' | 'dealer_review' | 'center_review' | 'approved' | 'rejected' | 'reapplied' | 'closed';
export type ClaimTone = 'neutral' | 'progress' | 'done' | 'problem';

/** Flow order (open → closed), same as the backend status sort. */
export const CLAIM_STATUSES: readonly ClaimStatus[] = [
  'open',
  'dealer_review',
  'center_review',
  'approved',
  'rejected',
  'reapplied',
  'closed',
];

export function claimTone(s: ClaimStatus): ClaimTone {
  if (s === 'approved' || s === 'reapplied') return 'done';
  if (s === 'rejected') return 'problem';
  if (s === 'dealer_review' || s === 'center_review') return 'progress';
  return 'neutral';
}

/** A claim that still blocks a new one on the same warranty. */
export function isLiveClaim(s: ClaimStatus): boolean {
  return s !== 'rejected' && s !== 'closed';
}

/**
 * "Submit for review" needs at least one part, a description and at least
 * one photo (the backend answers 422 CLAIM_PHOTO_REQUIRED otherwise).
 */
export function canSubmitClaim(input: { parts: readonly string[]; description: string; photos: number }): boolean {
  return input.parts.length > 0 && input.description.trim() !== '' && input.photos > 0;
}

export type ClaimGrants = { write: boolean; review: boolean; decide: boolean; superAdmin?: boolean };

export type ClaimActions = {
  /** open → dealer_review (the dealer / distributor that owns the claim). */
  submitReview: boolean;
  /** dealer_review (or a distributor's own open claim) → center_review. */
  forward: boolean;
  /** center_review → approved / rejected (center only). */
  decide: boolean;
  /** closed → approved; platform super admin only. */
  reopen: boolean;
};

/**
 * Actions the caller may take on a claim, mirroring the web: dealer_review
 * needs warranty_claims.write, forwarding to the center warranty_claims.review,
 * the decision warranty_claims.decide. Dealers never get approve / reject.
 */
export function claimActions(status: ClaimStatus, grants: ClaimGrants, orgType: string | undefined): ClaimActions {
  return {
    submitReview: status === 'open' && grants.write,
    forward: grants.review && (status === 'dealer_review' || (status === 'open' && orgType === 'distributor')),
    decide: status === 'center_review' && grants.decide,
    reopen: status === 'closed' && grants.superAdmin === true,
  };
}

/** Photos may be added while the claim is still being prepared. */
export function canAddClaimPhoto(status: ClaimStatus, write: boolean): boolean {
  return write && status === 'open';
}

/** `status` query value of the list: undefined for "all". */
export function statusParam(filter: 'all' | ClaimStatus): string | undefined {
  return filter === 'all' ? undefined : filter;
}

/** Toggles a part key in the picked list, keeping the order of selection. */
export function togglePart(parts: readonly string[], key: string): string[] {
  return parts.includes(key) ? parts.filter((p) => p !== key) : [...parts, key];
}
