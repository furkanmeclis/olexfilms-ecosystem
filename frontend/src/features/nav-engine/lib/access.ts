import type { NavGroupDef, NavItemDef } from "@/features/nav-engine/types";

type AccessFns = {
  can: (permission: string | string[]) => boolean;
  canAny: (permissions: string[]) => boolean;
};

export function isNavEntryVisible(
  entry: Pick<NavItemDef, "permission" | "anyPermission">,
  access: AccessFns,
): boolean {
  if (entry.permission && !access.can(entry.permission)) return false;
  if (entry.anyPermission?.length && !access.canAny(entry.anyPermission)) {
    return false;
  }
  return true;
}

export function visibleNavItems(group: NavGroupDef, access: AccessFns) {
  if (!isNavEntryVisible(group, access)) return [];
  return group.items.filter((item) => isNavEntryVisible(item, access));
}
