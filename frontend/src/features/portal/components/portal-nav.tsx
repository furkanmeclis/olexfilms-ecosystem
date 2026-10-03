"use client";

import { Car, House, ShieldCheck, Store } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { routes } from "@/config/routes";
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
  { href: routes.portal.dealers, key: "portal.nav.dealers", icon: Store },
] as const;

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

/** Portal top navigation (TEC-241): home, my vehicles, my warranties. */
export function PortalNav() {
  const { t } = useLocale();
  const pathname = usePathname() ?? "";
  return (
    <nav
      aria-label={t("portal.nav.label")}
      className="bg-background/95 sticky top-0 z-10 border-b backdrop-blur"
      data-testid="portal-nav"
    >
      <ul className="mx-auto flex w-full max-w-3xl gap-1 overflow-x-auto px-4 py-2">
        {ITEMS.map(({ href, key, icon: Icon }) => {
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
