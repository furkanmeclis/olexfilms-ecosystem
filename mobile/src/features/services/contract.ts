/**
 * Vehicle intake contract on mobile (TEC-1083, option 3). The service carries
 * a contract summary (`contract`) and whether one is required
 * (`contract_required`); creating one goes through POST
 * /v1/services/{uuid}/contract (mirrored with `services`). The signing calls
 * (/v1/contracts/{uuid}/signers/*: customer OTP, customer and staff
 * signatures) are not on the mobile mirror yet, so signing opens the web
 * wizard. Pure, no `@/` imports, so node --test runs it.
 */

export type ContractState = 'none' | 'draft' | 'pending' | 'executed' | 'voided';

/** Display state; a voided contract counts as none when a new one may be made. */
export function contractState(contract: { status: string } | null | undefined): ContractState {
  if (!contract) return 'none';
  const s = contract.status;
  return s === 'draft' || s === 'pending' || s === 'executed' || s === 'voided' ? s : 'pending';
}

/** Whether the card shows at all: a contract exists or the service needs one. */
export function showContract(s: { contract?: { status: string } | null; contract_required?: boolean }): boolean {
  return !!s.contract || !!s.contract_required;
}

/** The backend makes a contract only for a draft or pending service, none or voided before. */
export function canCreateContract(s: { status: string; contract?: { status: string } | null }): boolean {
  const state = contractState(s.contract);
  return (state === 'none' || state === 'voided') && (s.status === 'draft' || s.status === 'pending');
}

/** Signing is still open: a contract that is neither executed nor voided. */
export function awaitingSignature(contract: { status: string } | null | undefined): boolean {
  const state = contractState(contract);
  return state === 'draft' || state === 'pending';
}

/** Web wizard of the service, where the contract is signed (OTP + both signatures). */
export function contractWebUrl(origin: string, orgSlug: string, serviceUuid: string): string {
  return `${origin}/t/${encodeURIComponent(orgSlug)}/services/${encodeURIComponent(serviceUuid)}/wizard`;
}
