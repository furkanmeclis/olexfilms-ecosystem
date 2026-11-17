import { type Href, useSegments } from 'expo-router';

import { useSession } from '@/core/auth/session';
import { useModuleFilter } from '@/core/modules';
import { activeMembership, useResolvedSpace } from '@/core/spaces';

import { type EntityKind, entityPath } from './href';

/**
 * `(kind, id) => Href | null` for the screen the user is on (TEC-1219). Null
 * means "show the name as plain text": the app has no screen for the kind in
 * this space, the permission is missing or the module is switched off.
 */
export function useEntityHrefs(): (kind: EntityKind, id: string | null | undefined) => Href | null {
  const segments = useSegments() as string[];
  const fallbackSpace = useResolvedSpace();
  const me = useSession((s) => s.me);
  const routeOn = useModuleFilter();
  const orgType = activeMembership(me)?.type;
  return (kind, id) => {
    const can = (p: string) => !!me && (me.user.is_super_admin || me.permissions.includes(p));
    return entityPath({ segments, fallbackSpace, can, routeOn, orgType }, kind, id) as Href | null;
  };
}

/** Links of one kind: `useEntityHref('order')(uuid)`. */
export function useEntityHref(kind: EntityKind): (id: string | null | undefined) => Href | null {
  const hrefOf = useEntityHrefs();
  return (id) => hrefOf(kind, id);
}
