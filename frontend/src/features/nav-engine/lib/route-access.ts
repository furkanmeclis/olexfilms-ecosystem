import {
  isNavEntryVisible,
  type NavAccess,
} from "@/features/nav-engine/lib/access";
import type {
  NavCatalog,
  NavGroupDef,
  NavItemDef,
} from "@/features/nav-engine/types";

export type NavRouteMatch = { group: NavGroupDef; item: NavItemDef };

export type NavRouteAccess = "allowed" | "denied";

function normalizePath(path: string): string {
  const bare = path.split(/[?#]/, 1)[0] ?? "";
  const trimmed = bare.replace(/\/+$/, "");
  return trimmed || "/";
}

/**
 * Nav entries owning `pathname`: the items whose href is the longest prefix
 * of it. Several items can share one href (e.g. "My stock" and "Center
 * stock" differ only by orgTypes), so all of them are returned. The shell
 * home only matches itself.
 */
export function matchNavEntries(
  pathname: string,
  catalogs: NavCatalog[],
  homeHref?: string,
): NavRouteMatch[] {
  const current = normalizePath(pathname);
  const home = homeHref ? normalizePath(homeHref) : null;
  let best: NavRouteMatch[] = [];
  let bestLen = -1;

  for (const catalog of catalogs) {
    for (const group of catalog.groups) {
      for (const item of group.items) {
        const target = normalizePath(item.href);
        const matches =
          current === target ||
          (target !== home && current.startsWith(`${target}/`));
        if (!matches) continue;
        if (target.length > bestLen) {
          best = [{ group, item }];
          bestLen = target.length;
        } else if (target.length === bestLen) {
          best.push({ group, item });
        }
      }
    }
  }
  return best;
}

/**
 * TEC-771: a page is open when the menu would show one of its entries for
 * the user's permissions, organization type and member role. Module flags
 * are left out: a page behind an off module renders its own "module is
 * off" screen (FEATURE_DISABLED), which explains more than a 403. Pages
 * without a nav entry (details reached from lists, profile, the settings
 * index) stay open and rely on their own guards and the API.
 */
export function navRouteAccess(
  pathname: string,
  catalogs: NavCatalog[],
  access: NavAccess,
  homeHref?: string,
): NavRouteAccess {
  const matches = matchNavEntries(pathname, catalogs, homeHref);
  if (!matches.length) return "allowed";

  const ignoreFeature = <T extends { feature?: string }>(entry: T) => ({
    ...entry,
    feature: undefined,
  });
  const visible = ({ group, item }: NavRouteMatch) =>
    isNavEntryVisible(ignoreFeature(group), access) &&
    isNavEntryVisible(ignoreFeature(item), access);
  return matches.some(visible) ? "allowed" : "denied";
}
