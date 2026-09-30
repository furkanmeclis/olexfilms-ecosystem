import type { ReactNode } from "react";

import type {
  NavAdornment,
  NavAdornmentComponent,
  NavCatalog,
  NavGroupDef,
  NavItemDef,
} from "@/features/nav-engine/types";

export const EMPTY_NAV_ADORNMENT: NavAdornment = {};

export function defineNav(catalog: NavCatalog): NavCatalog {
  return catalog;
}

export function defineNavGroup(group: NavGroupDef): NavGroupDef {
  return group;
}

export function defineNavItem(item: NavItemDef): NavItemDef {
  return item;
}

/**
 * Wrap a hook so feature modules can attach live badges/info without
 * violating the rules of hooks. Mounted only while the item is visible.
 */
export function createNavAdornment(
  useAdornment: () => NavAdornment,
): NavAdornmentComponent {
  function Bound({
    children,
  }: {
    children: (adornment: NavAdornment) => ReactNode;
  }) {
    return children(useAdornment());
  }
  Bound.displayName = useAdornment.name || "NavAdornment";
  return Bound;
}
