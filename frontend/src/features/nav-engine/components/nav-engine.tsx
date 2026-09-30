"use client";

import { SidebarContent } from "@/components/ui/sidebar";
import { NavGroup } from "@/features/nav-engine/components/nav-group";
import type { NavCatalog } from "@/features/nav-engine/types";

export function NavEngine({
  catalog,
  homeHref,
}: {
  catalog: NavCatalog;
  homeHref: string;
}) {
  return (
    <SidebarContent>
      {catalog.groups.map((group) => (
        <NavGroup
          key={group.id}
          group={group}
          catalogId={catalog.id}
          homeHref={homeHref}
        />
      ))}
    </SidebarContent>
  );
}
