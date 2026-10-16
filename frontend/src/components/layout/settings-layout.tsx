"use client";

import { ChevronLeft } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { useShellNav } from "@/components/layout/use-shell-nav";
import type { ShellVariant } from "@/config/shell-nav";
import { visibleNavItems } from "@/features/nav-engine/lib/access";
import { resolveActiveNavHref } from "@/features/nav-engine/lib/active";
import type { NavGroupDef, NavItemDef } from "@/features/nav-engine/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type SettingsSection = { group: NavGroupDef; items: NavItemDef[] };

/** Settings groups and items open to the user, in catalog order. */
function useSettingsSections(variant: ShellVariant, tenantSlug?: string) {
  const nav = useShellNav(variant, tenantSlug);
  const sections: SettingsSection[] = (nav.settings?.groups ?? [])
    .map((group) => ({ group, items: visibleNavItems(group, nav.access) }))
    .filter((section) => section.items.length > 0);
  return { nav, sections };
}

/**
 * TEC-771: settings area shell. Desktop shows the sub-navigation in a left
 * column; on phones the index page is the menu and sub pages get a back
 * link, so the narrow screen never carries two menus.
 */
export function SettingsLayout({
  variant,
  tenantSlug,
  children,
}: {
  variant: ShellVariant;
  tenantSlug?: string;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const pathname = usePathname() ?? "";
  const { nav, sections } = useSettingsSections(variant, tenantSlug);
  const root = nav.settingsHref ?? "";
  const onIndex = pathname.replace(/\/+$/, "") === root;
  const hrefs = sections.flatMap((s) => s.items.map((item) => item.href));
  const activeHref = resolveActiveNavHref(pathname, hrefs);

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-4 md:flex-row md:gap-8">
      <aside className="hidden w-56 shrink-0 md:block">
        <nav
          aria-label={t("layout.settings_menu")}
          className="sticky top-20 space-y-5"
          data-testid="settings-nav"
        >
          <Link
            href={root}
            className={cn(
              "font-display block px-2 text-lg font-semibold tracking-tight",
              onIndex
                ? "text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {t("layout.nav_settings")}
          </Link>
          {sections.map(({ group, items }) => (
            <div key={group.id} className="space-y-1">
              <p className="text-muted-foreground px-2 text-xs font-medium tracking-wide uppercase">
                {t(group.labelKey)}
              </p>
              <ul className="space-y-0.5">
                {items.map((item) => {
                  const Icon = item.icon;
                  const active = item.href === activeHref;
                  return (
                    <li key={item.id}>
                      <Link
                        href={item.href}
                        aria-current={active ? "page" : undefined}
                        className={cn(
                          "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors",
                          active
                            ? "bg-accent text-accent-foreground font-medium"
                            : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
                        )}
                      >
                        <Icon className="size-4 shrink-0" />
                        <span className="truncate">{t(item.titleKey)}</span>
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col gap-4">
        {!onIndex ? (
          <Link
            href={root}
            className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1 text-sm md:hidden"
          >
            <ChevronLeft className="size-4 rtl:rotate-180" />
            {t("layout.nav_settings")}
          </Link>
        ) : null}
        {children}
      </div>
    </div>
  );
}

/** Settings index: every open settings page, grouped like the side menu. */
export function SettingsIndexPage({
  variant,
  tenantSlug,
}: {
  variant: ShellVariant;
  tenantSlug?: string;
}) {
  const { t } = useLocale();
  const { sections } = useSettingsSections(variant, tenantSlug);

  return (
    <div className="space-y-6" data-testid="settings-index">
      <PageHeader
        title={t("layout.nav_settings")}
        description={t("layout.settings_description")}
      />
      {sections.length === 0 ? (
        <EmptyState title={t("layout.settings_empty")} />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {sections.map(({ group, items }) => {
            const GroupIcon = group.icon;
            return (
              <section
                key={group.id}
                className="bg-card rounded-xl border p-4"
                aria-labelledby={`settings-group-${group.id}`}
              >
                <h2
                  id={`settings-group-${group.id}`}
                  className="mb-2 flex items-center gap-2 text-sm font-semibold"
                >
                  {GroupIcon ? (
                    <GroupIcon className="text-muted-foreground size-4" />
                  ) : null}
                  {t(group.labelKey)}
                </h2>
                <ul className="-mx-2">
                  {items.map((item) => {
                    const Icon = item.icon;
                    return (
                      <li key={item.id}>
                        <Link
                          href={item.href}
                          className="hover:bg-accent flex min-h-10 items-center gap-2 rounded-md px-2 py-2 text-sm"
                        >
                          <Icon className="text-muted-foreground size-4 shrink-0" />
                          <span>{t(item.titleKey)}</span>
                        </Link>
                      </li>
                    );
                  })}
                </ul>
              </section>
            );
          })}
        </div>
      )}
    </div>
  );
}
