"use client";

import { Settings } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { AppSidebarLogo } from "@/components/brand";
import type { AppLayoutVariant } from "@/components/layout/app-layout";
import {
  Sidebar,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";
import { useShellNav } from "@/components/layout/use-shell-nav";
import { NavEngine } from "@/features/nav-engine";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import { useLocale } from "@/providers/locale-provider";

export function AppSidebar({
  variant = "platform",
  tenantSlug,
}: {
  variant?: AppLayoutVariant;
  tenantSlug?: string;
}) {
  const { t } = useLocale();
  const pathname = usePathname();
  const nav = useShellNav(variant, tenantSlug);

  // TEC-771: one entry into the settings area, shown when any of its
  // pages is open to the user.
  const settingsHref = nav.settingsHref;
  const hasSettings = Boolean(
    settingsHref &&
    nav.settings?.groups.some(
      (group) => visibleNavItems(group, nav.access).length > 0,
    ),
  );
  const settingsActive = Boolean(
    settingsHref &&
    (pathname === settingsHref || pathname.startsWith(`${settingsHref}/`)),
  );

  return (
    <Sidebar collapsible="icon" variant="inset">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link href={nav.homeHref}>
                <AppSidebarLogo />
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <NavEngine catalog={nav.main} homeHref={nav.homeHref} org={nav.org} />

      <SidebarFooter>
        {hasSettings && settingsHref ? (
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                asChild
                isActive={settingsActive}
                tooltip={t("layout.nav_settings")}
              >
                <Link href={settingsHref} data-testid="sidebar-settings">
                  <Settings />
                  <span>{t("layout.nav_settings")}</span>
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        ) : null}
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
