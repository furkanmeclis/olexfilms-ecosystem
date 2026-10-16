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

/** "pending": a gate depends on data that is still loading (modules). */
export type NavRouteAccess = "allowed" | "denied" | "pending";

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
 * TEC-771: a page is open when the menu would show one of its entries;
 * pages without a nav entry (details reached from lists, profile, the
 * settings index) stay open and rely on their own guards and the API.
 */
export function navRouteAccess(
  pathname: string,
  catalogs: NavCatalog[],
  access: NavAccess,
  homeHref?: string,
): NavRouteAccess {
  const matches = matchNavEntries(pathname, catalogs, homeHref);
  if (!matches.length) return "allowed";

  const visible = ({ group, item }: NavRouteMatch) =>
    isNavEntryVisible(group, access) && isNavEntryVisible(item, access);
  if (matches.some(visible)) return "allowed";

  // Module flags are fetched after the session; until they arrive a
  // feature gated entry cannot be judged yet.
  const waitsForFeatures =
    access.org?.features == null &&
    matches.some(({ group, item }) => Boolean(group.feature || item.feature));
  if (waitsForFeatures) {
    const withoutFeatures = {
      ...access,
      org: { ...access.org, features: undefined },
    };
    const strip = <T extends { feature?: string }>(entry: T) => ({
      ...entry,
      feature: undefined,
    });
    const couldBeVisible = matches.some(
      ({ group, item }) =>
        isNavEntryVisible(strip(group), withoutFeatures) &&
        isNavEntryVisible(strip(item), withoutFeatures),
    );
    if (couldBeVisible) return "pending";
  }
  return "denied";
}
