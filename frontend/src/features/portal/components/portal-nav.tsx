"use client";

import {
  BellRing,
  Bot,
  CalendarClock,
  Car,
  FileSignature,
  FileText,
  House,
  Landmark,
  ShieldCheck,
  Store,
  Wrench,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";

import { routes } from "@/config/routes";
import { usePortalAssistantVisible } from "@/features/ai-assistant/hooks/use-portal-assistant-visible";
import { portalApi } from "@/features/portal/lib/portal-client";
import {
  isPortalReadOnly,
  PORTAL_ME_KEY,
} from "@/features/portal/lib/portal-vehicles";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const ITEMS = [
  { href: routes.portal.home, key: "portal.nav.home", icon: House },
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
  {
    href: routes.portal.contracts,
    key: "portal.nav.contracts",
    icon: FileSignature,
  },
  {
    href: routes.portal.preferences,
    key: "portal.nav.preferences",
    icon: BellRing,
  },
] as const;

const FLEET_ITEMS = [
  { href: routes.portal.home, key: "portal.nav.home", icon: House },
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
    href: routes.portal.preferences,
    key: "portal.nav.preferences",
    icon: BellRing,
  },
] as const;

/** TEC-390: shown only while the assistant is on for the customer. */
const ASSISTANT_ITEM = {
  href: routes.portal.assistant,
  key: "ai.nav_title",
  icon: Bot,
} as const;

/** Whether a nav item is the current section (home matches only itself). */
export function isPortalNavActive(href: string, pathname: string): boolean {
  if (href === routes.portal.home) return pathname === href;
  if (href === routes.portal.vehicles) {
    // A service detail is reached from a vehicle.
    return (
      pathname.startsWith(href) || pathname.startsWith("/portal/services/")
    );
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

/**
 * Portal top navigation (TEC-241): home, my vehicles, my warranties; TEC-245
 * adds my contracts and notification preferences, TEC-327 my appointments.
 */
export function PortalNav() {
  const { t } = useLocale();
  const pathname = usePathname() ?? "";
  const assistant = usePortalAssistantVisible();
  const me = useQuery({
    queryKey: PORTAL_ME_KEY,
    queryFn: () => portalApi.me(),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const fleet = isPortalReadOnly(me.data?.roles);
  const baseItems = fleet ? FLEET_ITEMS : ITEMS;
  const items =
    !fleet && assistant ? [...baseItems, ASSISTANT_ITEM] : baseItems;
  return (
    <nav
      aria-label={t("portal.nav.label")}
      className="bg-background/95 sticky top-0 z-10 border-b backdrop-blur"
      data-testid="portal-nav"
    >
      <ul className="mx-auto flex w-full max-w-3xl gap-1 overflow-x-auto px-4 py-2">
        {items.map(({ href, key, icon: Icon }) => {
          const active = isPortalNavActive(href, pathname);
          return (
            <li key={href}>
              <Link
                href={href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "inline-flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium whitespace-nowrap transition-colors",
                  active
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                <Icon className="size-4" />
                {t(key)}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
