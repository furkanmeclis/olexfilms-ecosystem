import { useQuery } from '@tanstack/react-query';
import { type Href, useSegments } from 'expo-router';
import { useMemo } from 'react';

import { api, unwrap } from '@/core/api/client';
import { useSession } from '@/core/auth/session';
import { activeMembership } from '@/core/spaces';

import { accountingAccess, canReadEinvoices } from './lib';

/** Permission check of the session (super admin passes every gate). */
function useCanFn() {
  const me = useSession((s) => s.me);
  return useMemo(
    () => (permission: string) => !!me && (!!me.user.is_super_admin || me.permissions.includes(permission)),
    [me],
  );
}

/** Accounting gates of the active organization (same rules as the web). */
export function useAccountingAccess() {
  const me = useSession((s) => s.me);
  const can = useCanFn();
  const org = activeMembership(me);
  const orgType = org?.type;
  const parentUuid = org?.parent?.uuid ?? null;
  return {
    ...accountingAccess(can, orgType, parentUuid),
    orgUuid: me?.active_organization_uuid,
    orgType,
    parentUuid,
    canReadEinvoices: canReadEinvoices(can, orgType),
    canManageEinvoices: can('einvoice.manage'),
  };
}

/** Routes inside the current space's menu (`/<space>/menu/...`). */
export function useAccountingLinks() {
  const space = (useSegments() as string[])[1] ?? 'bayi';
  return {
    cari: (uuid: string) => `/${space}/menu/accounting/${uuid}` as Href,
    disputes: `/${space}/menu/disputes` as Href,
    dispute: (uuid: string) => `/${space}/menu/disputes/${uuid}` as Href,
    einvoice: (uuid: string) => `/${space}/menu/einvoices/${uuid}` as Href,
  };
}

/** Every accounting query of the active organization starts with this key. */
export const accountingKey = (org: string | undefined) => ['accounting', org ?? ''] as const;

/** Category key → label in the request language (backend catalog). */
export function useCategoryLabels(org: string | undefined, enabled: boolean) {
  const q = useQuery({
    queryKey: [...accountingKey(org), 'categories'],
    queryFn: () => unwrap(api.GET('/v1/accounting/categories')),
    enabled,
    staleTime: 10 * 60 * 1000,
  });
  return useMemo(() => new Map((q.data?.items ?? []).map((c) => [c.key, c.label])), [q.data]);
}

/** Entry uuids that already carry an open dispute (the web reads the first 100). */
export function useOpenDisputeEntries(org: string | undefined, enabled: boolean) {
  const q = useQuery({
    queryKey: [...accountingKey(org), 'disputes', 'open-entries'],
    queryFn: () => unwrap(api.GET('/v1/accounting/disputes', { params: { query: { status: 'open', limit: 100 } } })),
    enabled,
  });
  return useMemo(() => new Set((q.data?.items ?? []).filter((d) => d.status === 'open').map((d) => d.entry.uuid)), [q.data]);
}
