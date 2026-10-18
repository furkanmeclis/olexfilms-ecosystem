import type { Href } from 'expo-router';
import { useSegments } from 'expo-router';

import { api, unwrap } from '@/core/api/client';
import { activeMembership } from '@/core/spaces';
import { useSession } from '@/core/auth/session';

/** Routes of the leads screens inside the current space (`/<space>/menu/leads`). */
export function useLeadLinks() {
  const space = (useSegments() as string[])[1] ?? 'bayi';
  const base = `/${space}/menu/leads`;
  return {
    detail: (uuid: string) => `${base}/${uuid}` as Href,
    create: () => `${base}/new` as Href,
  };
}

export function useOrgType() {
  return activeMembership(useSession((s) => s.me))?.type;
}

export const leadKey = (uuid: string) => ['leads', 'detail', uuid] as const;
export const leadEventsKey = (uuid: string) => ['leads', 'events', uuid] as const;

/** Members a lead can be assigned to (organization of the lead). */
export function fetchAssignees(organizationUuid?: string) {
  return unwrap(api.GET('/v1/leads/assignees', { params: { query: { organization_uuid: organizationUuid } } }));
}
