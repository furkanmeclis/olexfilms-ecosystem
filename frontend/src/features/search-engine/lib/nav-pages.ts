import type { AppLayoutVariant } from "@/components/layout/app-layout";
import { shellNav, shellNavCatalogs } from "@/config/shell-nav";
import {
  isNavEntryVisible,
  type NavAccess,
} from "@/features/nav-engine/lib/access";
import { resolveSearchIcon } from "@/features/search-engine/lib/icons";
import type { PaletteItem } from "@/features/search-engine/types";
import { createElement } from "react";

export function buildNavPageItems(
  variant: AppLayoutVariant,
  access: NavAccess,
  t: (key: string) => string,
  tenantSlug?: string,
  hiddenItemIds: ReadonlySet<string> = new Set(),
): PaletteItem[] {
  // TEC-771: settings pages are searchable like the sidebar ones.
  const catalogs = shellNavCatalogs(shellNav(variant, tenantSlug));
  const items: PaletteItem[] = [];

  for (const group of catalogs.flatMap((catalog) => catalog.groups)) {
    if (!isNavEntryVisible(group, access)) continue;
    const groupLabel = t(group.labelKey);
    for (const item of group.items) {
      if (hiddenItemIds.has(item.id)) continue;
      if (!isNavEntryVisible(item, access)) continue;
      if (item.soon) continue;
      items.push({
        id: `page:${item.id}`,
        spec: "pages",
        label: t(item.titleKey),
        description: groupLabel,
        href: item.href,
        iconKey: item.searchIcon ?? "pages",
        icon: createElement(resolveSearchIcon(item.searchIcon ?? "pages"), {
          className: "size-4 shrink-0 opacity-80",
        }),
        group: t("search.group_pages"),
      });
    }
  }

  return items;
}
