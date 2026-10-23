"use client";

import { useQuery } from "@tanstack/react-query";
import { LogOut } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import type { ReactNode } from "react";

import { AppSidebarLogo } from "@/components/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { Separator } from "@/components/ui/separator";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { routes } from "@/config/routes";
import { PortalNav } from "@/features/portal/components/portal-nav";
import { portalApi, portalSignOut } from "@/features/portal/lib/portal-client";
import {
  isPortalReadOnly,
  PORTAL_ME_KEY,
} from "@/features/portal/lib/portal-vehicles";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-868: portal frame in the panel's shell: the same sidebar (grouped
 * menu with icons, collapses to icons, drawer on a phone) and a slim header
 * with the menu trigger, language and theme. Portal sessions only; the
 * panel session (if any) is untouched by the sign-out here.
 */
export function PortalShell({ children }: { children: ReactNode }) {
  const { t } = useLocale();
  const router = useRouter();
  const me = useQuery({
    queryKey: PORTAL_ME_KEY,
    queryFn: () => portalApi.me(),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const fleet = isPortalReadOnly(me.data?.roles);

  const onLogout = async () => {
    await portalSignOut();
    router.replace(routes.portal.login);
    router.refresh();
  };

  return (
    <SidebarProvider defaultOpen>
      <Sidebar collapsible="icon" variant="inset" data-testid="portal-sidebar">
        <SidebarHeader>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton size="lg" asChild>
                <Link href={routes.portal.home}>
                  <AppSidebarLogo />
                </Link>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent>
          <PortalNav />
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                tooltip={t("portal.home.logout")}
                onClick={() => void onLogout()}
                data-testid="portal-sidebar-logout"
              >
                <LogOut className="rtl:rotate-180" />
                <span>{t("portal.home.logout")}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>
      <SidebarInset className="min-w-0">
        <header className="bg-background/95 sticky top-0 z-20 flex h-14 items-center gap-3 border-b px-4 backdrop-blur md:rounded-t-xl">
          <SidebarTrigger
            variant="outline"
            className="max-md:scale-110"
            aria-label={t("portal.nav.toggle")}
          />
          <Separator orientation="vertical" className="h-6" />
          <span className="text-muted-foreground min-w-0 flex-1 truncate text-sm font-medium">
            {t(fleet ? "portal.fleet.title" : "portal.nav.title")}
          </span>
          <div className="ms-auto flex items-center gap-1">
            <LocaleSwitch />
            <ThemeSwitch />
          </div>
        </header>
        {children}
      </SidebarInset>
    </SidebarProvider>
  );
}
