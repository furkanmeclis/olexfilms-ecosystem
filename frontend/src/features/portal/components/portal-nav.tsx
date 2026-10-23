"use client";

import {
  Bot,
  CalendarClock,
  Car,
  FileSignature,
  FileText,
  House,
  Landmark,
  ShieldCheck,
  Store,
  UserRound,
  Wrench,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";

import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { routes } from "@/config/routes";
import { usePortalAssistantVisible } from "@/features/ai-assistant/hooks/use-portal-assistant-visible";
import { portalApi } from "@/features/portal/lib/portal-client";
import {
  isPortalReadOnly,
  PORTAL_ME_KEY,
} from "@/features/portal/lib/portal-vehicles";
import { useLocale } from "@/providers/locale-provider";

type NavItem = { href: string; key: string; icon: LucideIcon };
type NavGroup = { key: string; labelKey: string; items: readonly NavItem[] };

/**
 * TEC-868: the customer menu in sections (overview, vehicles and services,
 * account). The link order stays the one of TEC-241 / TEC-245 / TEC-327.
 */
const GROUPS: readonly NavGroup[] = [
  {
    key: "general",
    labelKey: "portal.nav.group.general",
    items: [{ href: routes.portal.home, key: "portal.nav.home", icon: House }],
  },
  {
    key: "vehicles",
    labelKey: "portal.nav.group.vehicles",
    items: [
      { href: routes.portal.vehicles, key: "portal.nav.vehicles", icon: Car },
      {
        href: routes.portal.warranties,
        key: "portal.nav.warranties",
        icon: ShieldCheck,
      },
      {
        href: routes.portal.appointments,
        key: "portal.nav.appointments",
        icon: CalendarClock,
      },
      { href: routes.portal.dealers, key: "portal.nav.dealers", icon: Store },
    ],
  },
  {
    key: "account",
    labelKey: "portal.nav.group.account",
    items: [
      {
        href: routes.portal.contracts,
        key: "portal.nav.contracts",
        icon: FileSignature,
      },
      // TEC-771: preferences and contracts are linked from the profile too.
      {
        href: routes.portal.profile,
        key: "portal.nav.profile",
        icon: UserRound,
      },
    ],
  },
];

/**
 * TEC-868: the fleet menu in sections (overview, fleet, account and
 * reports); routes and order unchanged.
 */
const FLEET_GROUPS: readonly NavGroup[] = [
  {
    key: "general",
    labelKey: "portal.nav.group.general",
    items: [{ href: routes.portal.home, key: "portal.nav.home", icon: House }],
  },
  {
    key: "fleet",
    labelKey: "portal.fleet.nav.group.fleet",
    items: [
      {
        href: routes.portal.fleet.vehicles,
        key: "portal.fleet.nav.vehicles",
        icon: Car,
      },
      {
        href: routes.portal.fleet.services,
        key: "portal.fleet.nav.services",
        icon: Wrench,
      },
      {
        href: routes.portal.fleet.warranties,
        key: "portal.fleet.nav.warranties",
        icon: ShieldCheck,
      },
    ],
  },
  {
    key: "account",
    labelKey: "portal.fleet.nav.group.account",
    items: [
      {
        href: routes.portal.fleet.account,
        key: "portal.fleet.nav.account",
        icon: Landmark,
      },
      {
        href: routes.portal.fleet.reports,
        key: "portal.fleet.nav.reports",
        icon: FileText,
      },
      {
        href: routes.portal.profile,
        key: "portal.nav.profile",
        icon: UserRound,
      },
    ],
  },
];

/** TEC-390: shown only while the assistant is on for the customer. */
const ASSISTANT_ITEM: NavItem = {
  href: routes.portal.assistant,
  key: "ai.nav_title",
  icon: Bot,
};

/** Whether a nav item is the current section (home matches only itself). */
export function isPortalNavActive(href: string, pathname: string): boolean {
  if (href === routes.portal.home) return pathname === href;
  if (href === routes.portal.vehicles) {
    // A service detail is reached from a vehicle.
    return (
      pathname === href ||
      pathname.startsWith(`${href}/`) ||
      pathname.startsWith("/portal/services/")
    );
  }
  if (href === routes.portal.profile) {
    // Notification preferences are part of the profile.
    return (
      pathname === href ||
      pathname.startsWith(`${href}/`) ||
      pathname === routes.portal.preferences
    );
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

/** The menu groups of the session (fleet-only or customer). */
export function portalNavGroups(
  fleet: boolean,
  assistant: boolean,
): readonly NavGroup[] {
  if (fleet) return FLEET_GROUPS;
  if (!assistant) return GROUPS;
  return GROUPS.map((group) =>
    group.key === "account"
      ? { ...group, items: [...group.items, ASSISTANT_ITEM] }
      : group,
  );
}

/**
 * Portal navigation (TEC-241, TEC-245, TEC-327), shown in the portal
 * sidebar (TEC-868): grouped sections with icons and the current page
 * marked; collapses to icons on desktop and lives in a drawer on a phone.
 * Must render inside the portal SidebarProvider.
 */
export function PortalNav() {
  const { t } = useLocale();
  const pathname = usePathname() ?? "";
  const assistant = usePortalAssistantVisible();
  const { isMobile, setOpenMobile } = useSidebar();
  const me = useQuery({
    queryKey: PORTAL_ME_KEY,
    queryFn: () => portalApi.me(),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const fleet = isPortalReadOnly(me.data?.roles);
  const groups = portalNavGroups(fleet, assistant);
  const count = groups.reduce((n, g) => n + g.items.length, 0);
  // Keep the current section in view in a long (or scrolled) menu.
  const navRef = useRef<HTMLElement>(null);
  useEffect(() => {
    navRef.current
      ?.querySelector<HTMLElement>('[aria-current="page"]')
      ?.scrollIntoView?.({ block: "nearest", inline: "center" });
  }, [pathname, count]);
  return (
    <nav
      ref={navRef}
      aria-label={t("portal.nav.label")}
      data-testid="portal-nav"
      className="flex min-h-0 flex-col"
    >
      {groups.map((group) => (
        <SidebarGroup key={group.key} data-group={group.key}>
          <SidebarGroupLabel>{t(group.labelKey)}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {group.items.map(({ href, key, icon: Icon }) => {
                const active = isPortalNavActive(href, pathname);
                const label = t(key);
                return (
                  <SidebarMenuItem key={href}>
                    <SidebarMenuButton
                      asChild
                      isActive={active}
                      tooltip={label}
                    >
                      <Link
                        href={href}
                        aria-current={active ? "page" : undefined}
                        onClick={() => {
                          if (isMobile) setOpenMobile(false);
                        }}
                      >
                        <Icon />
                        <span>{label}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      ))}
    </nav>
  );
}
