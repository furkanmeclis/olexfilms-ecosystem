import type {
  NavGroupDef,
  NavItemDef,
  NavOrgContext,
} from "@/features/nav-engine/types";

export type NavAccess = {
  can: (permission: string | string[]) => boolean;
  canAny: (permissions: string[]) => boolean;
  /** Active organization; entries with orgTypes/orgRoles need it. */
  org?: NavOrgContext | null;
};

export function isNavEntryVisible(
  entry: Pick<
    NavItemDef,
    "permission" | "anyPermission" | "orgTypes" | "orgRoles"
  >,
  access: NavAccess,
): boolean {
  if (entry.permission && !access.can(entry.permission)) return false;
  if (entry.anyPermission?.length && !access.canAny(entry.anyPermission)) {
    return false;
  }
  if (entry.orgTypes?.length) {
    const type = access.org?.type;
    if (!type || !(entry.orgTypes as string[]).includes(type)) return false;
  }
  if (entry.orgRoles?.length) {
    const role = access.org?.role;
    if (!role || !entry.orgRoles.includes(role)) return false;
  }
  return true;
}

export function visibleNavItems(group: NavGroupDef, access: NavAccess) {
  if (!isNavEntryVisible(group, access)) return [];
  return group.items.filter((item) => isNavEntryVisible(item, access));
}
