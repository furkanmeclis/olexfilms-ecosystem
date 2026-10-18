import type { Href } from 'expo-router';
import { useSegments } from 'expo-router';

import { useSession } from '@/core/auth/session';
import { activeMembership, useCan } from '@/core/spaces';

import type { ClaimGrants } from './lib';

/** Routes of the claims screens inside the current space (`/<space>/menu/warranty_claims`). */
export function useClaimLinks() {
  const space = (useSegments() as string[])[1] ?? 'bayi';
  const base = `/${space}/menu/warranty_claims`;
  return {
    detail: (uuid: string) => `${base}/${uuid}` as Href,
    create: (warrantyUuid?: string) => (warrantyUuid ? `${base}/new?warranty_uuid=${warrantyUuid}` : `${base}/new`) as Href,
  };
}

/** Permissions and organization type that decide which claim actions are shown. */
export function useClaimGrants(): { grants: ClaimGrants; orgType: string | undefined } {
  const me = useSession((s) => s.me);
  const write = useCan('warranty_claims.write');
  const review = useCan('warranty_claims.review');
  const decide = useCan('warranty_claims.decide');
  return {
    grants: { write, review, decide, superAdmin: !!me?.user.is_super_admin },
    orgType: activeMembership(me)?.type,
  };
}
