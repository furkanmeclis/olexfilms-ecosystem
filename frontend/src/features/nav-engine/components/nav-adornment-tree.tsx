"use client";

import type { ReactNode } from "react";

import { EMPTY_NAV_ADORNMENT } from "@/features/nav-engine/define";
import type {
  NavAdornment,
  NavAdornmentComponent,
  NavItemDef,
  ResolvedNavItem,
} from "@/features/nav-engine/types";

function NavAdornmentGate({
  Adornment,
  children,
}: {
  Adornment?: NavAdornmentComponent;
  children: (adornment: NavAdornment) => ReactNode;
}) {
  if (!Adornment) return children(EMPTY_NAV_ADORNMENT);
  return <Adornment>{children}</Adornment>;
}

/**
 * Mounts each item's adornment hook (badges/info) so group-level aggregates
 * stay live even when the accordion or icon flyout is closed.
 */
export function NavAdornmentTree({
  items,
  children,
}: {
  items: NavItemDef[];
  children: (resolved: ResolvedNavItem[]) => ReactNode;
}) {
  return (
    <NavAdornmentTreeInner items={items} index={0} acc={[]}>
      {children}
    </NavAdornmentTreeInner>
  );
}

function NavAdornmentTreeInner({
  items,
  index,
  acc,
  children,
}: {
  items: NavItemDef[];
  index: number;
  acc: ResolvedNavItem[];
  children: (resolved: ResolvedNavItem[]) => ReactNode;
}) {
  if (index >= items.length) return children(acc);

  const item = items[index];
  return (
    <NavAdornmentGate Adornment={item.Adornment}>
      {(adornment) => (
        <NavAdornmentTreeInner
          items={items}
          index={index + 1}
          acc={[...acc, { item, adornment }]}
        >
          {children}
        </NavAdornmentTreeInner>
      )}
    </NavAdornmentGate>
  );
}
