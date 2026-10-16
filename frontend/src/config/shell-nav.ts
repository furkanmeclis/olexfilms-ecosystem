import { cmsNav, platformNav, tenantNav } from "@/config/nav";
import { routes } from "@/config/routes";
import { platformSettingsNav, tenantSettingsNav } from "@/config/settings-nav";
import type { NavCatalog } from "@/features/nav-engine/types";

export type ShellVariant = "platform" | "cms" | "tenant";

export type ShellNav = {
  /** Sidebar menu. */
  main: NavCatalog;
  /** Settings area sub-navigation (TEC-771); null for the CMS shell. */
  settings: NavCatalog | null;
  homeHref: string;
  settingsHref: string | null;
};

/** Menus of one app shell; the sidebar, palette and route guard share it. */
export function shellNav(variant: ShellVariant, tenantSlug?: string): ShellNav {
  if (variant === "tenant" && tenantSlug) {
    return {
      main: tenantNav(tenantSlug),
      settings: tenantSettingsNav(tenantSlug),
      homeHref: routes.tenant.home(tenantSlug),
      settingsHref: routes.tenant.settings.root(tenantSlug),
    };
  }
  if (variant === "cms") {
    return {
      main: cmsNav,
      settings: null,
      homeHref: routes.cms.home,
      settingsHref: null,
    };
  }
  return {
    main: platformNav,
    settings: platformSettingsNav,
    homeHref: routes.platform.home,
    settingsHref: routes.platform.settings.root,
  };
}

/** Main menu plus settings, for lookups over every page of the shell. */
export function shellNavCatalogs(nav: ShellNav): NavCatalog[] {
  return nav.settings ? [nav.main, nav.settings] : [nav.main];
}
