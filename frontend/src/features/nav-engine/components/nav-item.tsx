"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import {
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { NavBadges } from "@/features/nav-engine/components/nav-badges";
import {
  hasNavInfo,
  NavInfoButton,
  NavInfoSummary,
} from "@/features/nav-engine/components/nav-info";
import { isNavHrefActive } from "@/features/nav-engine/lib/active";
import { mergeNavAdornment } from "@/features/nav-engine/lib/adornment";
import type { NavAdornment, NavItemDef } from "@/features/nav-engine/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export function NavSidebarItem({
  item,
  adornment,
  homeHref,
  peerHrefs,
}: {
  item: NavItemDef;
  adornment: NavAdornment;
  homeHref: string;
  peerHrefs?: string[];
}) {
  const { t } = useLocale();
  const pathname = usePathname();
  const { isMobile, setOpenMobile } = useSidebar();
  const merged = mergeNavAdornment(item, adornment, t("common.soon"));
  const title = t(item.titleKey);
  const active = isNavHrefActive(pathname, item.href, homeHref, peerHrefs);
  const Icon = item.icon;
  const showInfo = hasNavInfo(merged.info);
  const tooltip = item.soon ? `${title} (${t("common.soon")})` : title;

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        asChild
        isActive={active}
        tooltip={tooltip}
        className={cn(
          "transition-[background-color,box-shadow,transform] duration-200",
          active && "shadow-sm",
        )}
      >
        <Link
          href={item.href}
          className={cn(item.soon && "opacity-90")}
          onClick={() => {
            if (isMobile) setOpenMobile(false);
          }}
        >
          <Icon />
          <span>{title}</span>
          <NavBadges
            badges={merged.badges}
            className="group-data-[collapsible=icon]:hidden"
          />
        </Link>
      </SidebarMenuButton>
      {showInfo && merged.info ? (
        <NavInfoButton info={merged.info} label={t("layout.nav_item_info")} />
      ) : null}
    </SidebarMenuItem>
  );
}

export function NavDropdownItem({
  item,
  adornment,
  homeHref,
  peerHrefs,
}: {
  item: NavItemDef;
  adornment: NavAdornment;
  homeHref: string;
  peerHrefs?: string[];
}) {
  const { t } = useLocale();
  const pathname = usePathname();
  const { isMobile, setOpenMobile } = useSidebar();
  const merged = mergeNavAdornment(item, adornment, t("common.soon"));
  const title = t(item.titleKey);
  const active = isNavHrefActive(pathname, item.href, homeHref, peerHrefs);
  const Icon = item.icon;
  const showInfo = hasNavInfo(merged.info);

  return (
    <DropdownMenuItem asChild>
      <Link
        href={item.href}
        data-active={active}
        className={cn(
          "flex flex-col items-stretch gap-1",
          active && "bg-accent",
        )}
        onClick={() => {
          if (isMobile) setOpenMobile(false);
        }}
      >
        <span className="flex w-full items-center gap-2">
          <Icon />
          <span className="min-w-0 flex-1 truncate">{title}</span>
          <NavBadges badges={merged.badges} compact />
        </span>
        {showInfo && merged.info ? <NavInfoSummary info={merged.info} /> : null}
      </Link>
    </DropdownMenuItem>
  );
}
