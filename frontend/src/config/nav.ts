import {
  Activity,
  Bell,
  Building2,
  Download,
  FileText,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  LogIn,
  ScrollText,
  Settings2,
  Shield,
  Upload,
  UserRound,
} from "lucide-react";

import { appleNavIcon } from "@/components/icons/apple-icon";
import { facebookNavIcon } from "@/components/icons/facebook-icon";
import { githubNavIcon } from "@/components/icons/github-icon";
import { googleNavIcon } from "@/components/icons/google-icon";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNav } from "@/features/nav-engine";
import { usersNavItem } from "@/features/users/nav";

export const platformNav = defineNav({
  id: "platform",
  groups: [
    {
      id: "general",
      labelKey: "layout.nav_platform",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.platform.home,
          icon: LayoutDashboard,
        },
      ],
    },
    {
      id: "manage",
      labelKey: "layout.section_manage",
      icon: Shield,
      defaultOpen: true,
      items: [
        usersNavItem,
        {
          id: "organizations",
          titleKey: "layout.nav_organizations",
          href: routes.platform.organizations.root,
          icon: Building2,
          permission: permissions.organizations.read,
        },
        {
          id: "roles",
          titleKey: "layout.nav_roles",
          href: routes.platform.roles.root,
          icon: Shield,
          permission: permissions.roles.read,
        },
        {
          id: "notifications",
          titleKey: "layout.nav_notifications",
          href: routes.platform.notifications.root,
          icon: Bell,
          permission: permissions.notifications.platformRead,
        },
        {
          id: "activity",
          titleKey: "layout.nav_activity",
          href: routes.platform.activity.root,
          icon: Activity,
          permission: permissions.activity.read,
        },
        {
          id: "document-templates",
          titleKey: "layout.nav_document_templates",
          href: routes.platform.documentTemplates.root,
          icon: FileText,
          permission: permissions.documentTemplates.read,
        },
        {
          id: "logs",
          titleKey: "layout.nav_logs",
          href: routes.platform.logs.root,
          icon: ScrollText,
          permission: permissions.logs.read,
        },
        {
          id: "storage",
          titleKey: "layout.nav_storage",
          href: routes.platform.storage.root,
          icon: HardDrive,
          permission: permissions.storage.read,
        },
        {
          id: "access",
          titleKey: "layout.nav_access",
          href: routes.platform.access.root,
          icon: KeyRound,
          permission: permissions.access.read,
        },
      ],
    },
    {
      id: "auth-methods",
      labelKey: "layout.section_auth_methods",
      icon: LogIn,
      defaultOpen: true,
      items: [
        {
          id: "auth-settings",
          titleKey: "layout.nav_auth_settings",
          href: routes.platform.authSettings.root,
          icon: Settings2,
          permission: permissions.authSettings.read,
        },
        {
          id: "github-integration",
          titleKey: "layout.nav_github_integration",
          href: routes.platform.integrations.github,
          icon: githubNavIcon,
          searchIcon: "github",
          permission: permissions.integrations.github.read,
        },
        {
          id: "google-integration",
          titleKey: "layout.nav_google_integration",
          href: routes.platform.integrations.google,
          icon: googleNavIcon,
          permission: permissions.integrations.google.read,
        },
        {
          id: "facebook-integration",
          titleKey: "layout.nav_facebook_integration",
          href: routes.platform.integrations.facebook,
          icon: facebookNavIcon,
          permission: permissions.integrations.facebook.read,
        },
        {
          id: "apple-integration",
          titleKey: "layout.nav_apple_integration",
          href: routes.platform.integrations.apple,
          icon: appleNavIcon,
          permission: permissions.integrations.apple.read,
        },
      ],
    },
    {
      id: "io",
      labelKey: "layout.section_io",
      icon: Settings2,
      defaultOpen: true,
      items: [
        {
          id: "imports",
          titleKey: "layout.nav_imports",
          href: routes.platform.imports.root,
          icon: Upload,
          permission: permissions.imports.read,
        },
        {
          id: "exports",
          titleKey: "layout.nav_exports",
          href: routes.platform.exports.root,
          icon: Download,
          permission: permissions.exports.read,
        },
        {
          id: "export-settings",
          titleKey: "layout.nav_export_settings",
          href: routes.platform.settings.root,
          icon: Settings2,
          permission: permissions.settings.read,
        },
      ],
    },
  ],
});

export const cmsNav = defineNav({
  id: "cms",
  groups: [
    {
      id: "cms",
      labelKey: "layout.section_cms",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.cms.home,
          icon: LayoutDashboard,
        },
      ],
    },
  ],
});

/**
 * Tenant menu. Items may narrow by organization type (orgTypes: center |
 * distributor | dealer) and member role (orgRoles); tenant I/O routes are
 * owner-only on the API (RequireOrgRole).
 */
export function tenantNav(slug: string) {
  return defineNav({
    id: "tenant",
    groups: [
      {
        id: "tenant",
        labelKey: "layout.section_tenant",
        icon: Building2,
        defaultOpen: true,
        items: [
          {
            id: "home",
            titleKey: "layout.home",
            href: routes.tenant.home(slug),
            icon: LayoutDashboard,
          },
          {
            id: "profile",
            titleKey: "layout.nav_profile",
            href: routes.tenant.profile.root(slug),
            icon: UserRound,
          },
        ],
      },
      {
        id: "io",
        labelKey: "layout.section_io",
        icon: Settings2,
        defaultOpen: true,
        items: [
          {
            id: "tenant-exports",
            titleKey: "layout.nav_exports",
            href: routes.tenant.exports.root(slug),
            icon: Download,
            permission: permissions.imports.tenantRead,
            orgRoles: ["owner"],
          },
          {
            id: "tenant-imports",
            titleKey: "layout.nav_imports",
            href: routes.tenant.imports.root(slug),
            icon: Upload,
            permission: permissions.imports.tenantRead,
            orgRoles: ["owner"],
          },
          {
            id: "tenant-export-settings",
            titleKey: "layout.nav_export_settings",
            href: routes.tenant.settings.root(slug),
            icon: Settings2,
            permission: permissions.settings.tenantRead,
            orgRoles: ["owner"],
          },
        ],
      },
    ],
  });
}
