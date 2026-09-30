import type { AppLayoutVariant } from "@/components/layout/app-layout";
import { cmsNav, platformNav, tenantNav } from "@/config/nav";
import { isNavEntryVisible } from "@/features/nav-engine/lib/access";
import { resolveSearchIcon } from "@/features/search-engine/lib/icons";
import type { PaletteItem } from "@/features/search-engine/types";
import type { NavCatalog } from "@/features/nav-engine/types";
import { createElement } from "react";

type AccessFns = {
  can: (permission: string | string[]) => boolean;
  canAny: (permissions: string[]) => boolean;
};

export function buildNavPageItems(
  variant: AppLayoutVariant,
  access: AccessFns,
  t: (key: string) => string,
  tenantSlug?: string,
  hiddenItemIds: ReadonlySet<string> = new Set(),
): PaletteItem[] {
  const catalog: NavCatalog =
    variant === "tenant" && tenantSlug
      ? tenantNav(tenantSlug)
      : variant === "cms"
        ? cmsNav
        : platformNav;
  const items: PaletteItem[] = [];

  for (const group of catalog.groups) {
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
