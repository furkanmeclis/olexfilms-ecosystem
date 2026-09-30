"use client";

import { ChevronDown } from "lucide-react";
import { usePathname } from "next/navigation";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { NavAdornmentTree } from "@/features/nav-engine/components/nav-adornment-tree";
import { NavCollapsedIndicator } from "@/features/nav-engine/components/nav-badges";
import {
  NavDropdownItem,
  NavSidebarItem,
} from "@/features/nav-engine/components/nav-item";
import { useNavGroupOpen } from "@/features/nav-engine/hooks/use-nav-group-open";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import {
  formatNavCount,
  resolveActiveNavHref,
} from "@/features/nav-engine/lib/active";
import {
  hasVisibleNavBadge,
  mergeNavAdornment,
  visibleCountTotal,
} from "@/features/nav-engine/lib/adornment";
import type { NavGroupDef, ResolvedNavItem } from "@/features/nav-engine/types";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export function NavGroup({
  group,
  catalogId,
  homeHref,
}: {
  group: NavGroupDef;
  catalogId: string;
  homeHref: string;
}) {
  const { can, canAny } = usePermission();
  const items = visibleNavItems(group, { can, canAny });
  if (!items.length) return null;

  return (
    <NavAdornmentTree items={items}>
      {(resolved) => (
        <NavGroupView
          group={group}
          catalogId={catalogId}
          homeHref={homeHref}
          resolved={resolved}
        />
      )}
    </NavAdornmentTree>
  );
}

function NavGroupView({
  group,
  catalogId,
  homeHref,
  resolved,
}: {
  group: NavGroupDef;
  catalogId: string;
  homeHref: string;
  resolved: ResolvedNavItem[];
}) {
  const { t, dir } = useLocale();
  const pathname = usePathname();
  const { isMobile, state } = useSidebar();
  const collapsible = group.collapsible ?? true;
  const peerHrefs = resolved.map(({ item }) => item.href);
  const hasActive =
    resolveActiveNavHref(pathname, peerHrefs, homeHref) !== null;
  const [open, onOpenChange] = useNavGroupOpen(
    catalogId,
    group.id,
    group.defaultOpen ?? true,
  );

  const iconMode = !isMobile && state === "collapsed";
  const label = t(group.labelKey);
  const GroupIcon = group.icon ?? resolved[0]?.item.icon;
  const collapsedCount = resolved.reduce((sum, entry) => {
    const merged = mergeNavAdornment(
      entry.item,
      entry.adornment,
      t("common.soon"),
    );
    return sum + visibleCountTotal(merged.badges);
  }, 0);
  const collapsedHasExtra = resolved.some((entry) => {
    const merged = mergeNavAdornment(
      entry.item,
      entry.adornment,
      t("common.soon"),
    );
    return hasVisibleNavBadge(merged.badges);
  });

  if (iconMode) {
    return (
      <CollapsedNavGroup
        label={label}
        GroupIcon={GroupIcon}
        resolved={resolved}
        homeHref={homeHref}
        peerHrefs={peerHrefs}
        hasActive={hasActive}
        dir={dir}
        count={collapsedCount}
        hasExtra={collapsedHasExtra}
      />
    );
  }

  const itemList = (
    <SidebarGroupContent>
      <SidebarMenu>
        {resolved.map(({ item, adornment }) => (
          <NavSidebarItem
            key={item.id}
            item={item}
            adornment={adornment}
            homeHref={homeHref}
            peerHrefs={peerHrefs}
          />
        ))}
      </SidebarMenu>
    </SidebarGroupContent>
  );

  if (!collapsible) {
    return (
      <SidebarGroup>
        <SidebarGroupLabel>{label}</SidebarGroupLabel>
        {itemList}
      </SidebarGroup>
    );
  }

  return (
    <Collapsible
      open={open}
      onOpenChange={onOpenChange}
      className="group/collapsible"
    >
      <SidebarGroup>
        <SidebarGroupLabel asChild>
          <CollapsibleTrigger className="hover:bg-sidebar-accent hover:text-sidebar-accent-foreground w-full cursor-pointer">
            {label}
            <span className="ms-auto flex items-center gap-1">
              {!open && collapsedCount > 0 ? (
                <span className="text-sidebar-foreground/70 text-[10px] font-medium tabular-nums">
                  {formatNavCount(collapsedCount)}
                </span>
              ) : null}
              <ChevronDown className="size-4 transition-transform duration-200 group-data-[state=closed]/collapsible:-rotate-90 rtl:group-data-[state=closed]/collapsible:rotate-90" />
            </span>
          </CollapsibleTrigger>
        </SidebarGroupLabel>
        <CollapsibleContent className="data-[state=closed]:animate-accordion-up data-[state=open]:animate-accordion-down overflow-hidden">
          {itemList}
        </CollapsibleContent>
      </SidebarGroup>
    </Collapsible>
  );
}

function CollapsedNavGroup({
  label,
  GroupIcon,
  resolved,
  homeHref,
  peerHrefs,
  hasActive,
  dir,
  count,
  hasExtra,
}: {
  label: string;
  GroupIcon?: ResolvedNavItem["item"]["icon"];
  resolved: ResolvedNavItem[];
  homeHref: string;
  peerHrefs: string[];
  hasActive: boolean;
  dir: "ltr" | "rtl";
  count: number;
  hasExtra: boolean;
}) {
  const single = resolved.length === 1;

  if (single) {
    const { item, adornment } = resolved[0];
    return (
      <SidebarGroup className="px-2 py-1">
        <SidebarGroupContent>
          <SidebarMenu>
            <NavSidebarItem
              item={item}
              adornment={adornment}
              homeHref={homeHref}
              peerHrefs={peerHrefs}
            />
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    );
  }

  const Icon = GroupIcon;

  return (
    <SidebarGroup className="px-2 py-1">
      <SidebarGroupContent>
        <SidebarMenu>
          <SidebarMenuItem>
            <DropdownMenu>
              <div className="relative">
                <SidebarMenuButton asChild tooltip={label} isActive={hasActive}>
                  <DropdownMenuTrigger>
                    {Icon ? <Icon /> : null}
                    <span className="sr-only">{label}</span>
                  </DropdownMenuTrigger>
                </SidebarMenuButton>
                <NavCollapsedIndicator
                  count={count}
                  hasExtra={hasExtra && count <= 0}
                />
              </div>
              <DropdownMenuContent
                side={dir === "rtl" ? "left" : "right"}
                align="start"
                sideOffset={8}
                className="min-w-56"
              >
                <DropdownMenuLabel>{label}</DropdownMenuLabel>
                <DropdownMenuSeparator />
                {resolved.map(({ item, adornment }) => (
                  <NavDropdownItem
                    key={item.id}
                    item={item}
                    adornment={adornment}
                    homeHref={homeHref}
                    peerHrefs={peerHrefs}
                  />
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}
