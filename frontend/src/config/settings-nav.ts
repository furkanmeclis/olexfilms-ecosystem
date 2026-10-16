import {
  Activity,
  BadgeCheck,
  BellRing,
  Blocks,
  Bot,
  Camera,
  Car,
  ClipboardList,
  Coins,
  Cpu,
  Download,
  FileSignature,
  FileText,
  HardDrive,
  KeyRound,
  LogIn,
  MapPinned,
  MessageCircle,
  MessageSquareText,
  Plug,
  RectangleHorizontal,
  Recycle,
  ScrollText,
  Settings2,
  Shield,
  SlidersHorizontal,
  Upload,
  Warehouse,
} from "lucide-react";

import { appleNavIcon } from "@/components/icons/apple-icon";
import { facebookNavIcon } from "@/components/icons/facebook-icon";
import { githubNavIcon } from "@/components/icons/github-icon";
import { googleNavIcon } from "@/components/icons/google-icon";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { aiPlatformNavItem, aiUsageNavItem } from "@/features/ai-admin/nav";
import { defineNav } from "@/features/nav-engine";
import type { NavCatalog } from "@/features/nav-engine/types";

/**
 * TEC-771: settings area sub-navigation. The main sidebar keeps the daily
 * work; configuration pages live under /platform/settings and
 * /t/{slug}/settings and are listed here, with the same gates as the
 * sidebar (permission, orgTypes, orgRoles, feature). The settings layout
 * renders these catalogs and the route guard reads them, so a new settings
 * screen is one item here plus its page under the settings root.
 */
export const platformSettingsNav = defineNav({
  id: "platform-settings",
  groups: [
    {
      id: "access",
      labelKey: "layout.settings_group_access",
      icon: Shield,
      items: [
        {
          id: "roles",
          titleKey: "layout.nav_roles",
          href: routes.platform.roles.root,
          icon: Shield,
          permission: permissions.roles.read,
        },
        {
          id: "access",
          titleKey: "layout.nav_access",
          href: routes.platform.access.root,
          icon: KeyRound,
          permission: permissions.access.read,
        },
        {
          id: "auth-settings",
          titleKey: "layout.section_auth_methods",
          href: routes.platform.authSettings.root,
          icon: LogIn,
          permission: permissions.authSettings.read,
        },
        {
          id: "legal-texts",
          titleKey: "layout.nav_legal_texts",
          href: routes.platform.legalTexts,
          icon: ScrollText,
          permission: permissions.legalTexts.write,
        },
      ],
    },
    {
      id: "integrations",
      labelKey: "layout.settings_group_integrations",
      icon: Plug,
      items: [
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
        {
          id: "whatsapp-integration",
          titleKey: "layout.nav_whatsapp_integration",
          href: routes.platform.integrations.whatsapp,
          icon: MessageCircle,
          permission: permissions.integrations.whatsapp.manage,
        },
        {
          id: "glorian-integration",
          titleKey: "layout.nav_glorian_integration",
          href: routes.platform.integrations.glorian,
          icon: Warehouse,
          permission: permissions.integrations.glorian.view,
        },
        {
          // TEC-403: registered MCP clients (AI apps).
          id: "mcp-clients",
          titleKey: "mcp.clients.nav",
          href: routes.platform.mcpClients,
          icon: Plug,
          permission: permissions.mcp.clientsManage,
        },
        // TEC-391: AI settings, organization quotas and usage report.
        aiPlatformNavItem,
      ],
    },
    {
      id: "system",
      labelKey: "layout.settings_group_system",
      icon: SlidersHorizontal,
      items: [
        {
          id: "modules",
          titleKey: "layout.nav_modules",
          href: routes.platform.modules.root,
          icon: Blocks,
          permission: permissions.modules.platformRead,
        },
        {
          // TEC-222: system settings hub (platform.settings.read).
          id: "system-settings",
          titleKey: "layout.nav_system_settings",
          href: routes.platform.systemSettings.root,
          icon: SlidersHorizontal,
          permission: permissions.settings.read,
        },
        {
          id: "notification-center",
          titleKey: "layout.nav_notification_center",
          href: routes.platform.notifications.center,
          icon: BellRing,
          permission: permissions.notifications.templatesManage,
        },
      ],
    },
    {
      id: "catalogs",
      labelKey: "layout.settings_group_catalogs",
      icon: ClipboardList,
      items: [
        {
          // TEC-150: car brands / models; write is super_admin only.
          id: "vehicle-catalog",
          titleKey: "vehicles.nav",
          href: routes.platform.vehicleCatalog.root,
          icon: Car,
          permission: permissions.vehicleCatalog.write,
        },
        {
          id: "service-catalog",
          titleKey: "layout.nav_service_catalog",
          href: routes.platform.serviceCatalog.root,
          icon: ClipboardList,
          permission: permissions.serviceCatalog.manage,
        },
        {
          id: "certificate-types",
          titleKey: "certificates.types.nav",
          href: routes.platform.certificateTypes.root,
          icon: BadgeCheck,
          permission: permissions.certificates.typesManage,
        },
        {
          // TEC-500: central intake angles of the photo standard add-on.
          id: "photo-angles",
          titleKey: "photo_standard.angles.nav",
          href: routes.platform.photoAngles.root,
          icon: Camera,
          permission: permissions.photoStandard.manage,
        },
        {
          // TEC-489: expected part consumption of the efficiency add-on.
          id: "part-consumption-expectations",
          titleKey: "efficiency.expectations.nav",
          href: routes.platform.partConsumptionExpectations.root,
          icon: Recycle,
          permission: permissions.efficiency.expectationsManage,
        },
        {
          // TEC-353: questions under the portal service review ratings.
          id: "review-questions",
          titleKey: "layout.nav_review_questions",
          href: routes.platform.reviewQuestions.root,
          icon: MessageSquareText,
          permission: permissions.reviews.questionsManage,
        },
      ],
    },
    {
      id: "regional",
      labelKey: "layout.settings_group_regional",
      icon: MapPinned,
      items: [
        {
          id: "territories",
          titleKey: "layout.nav_territories",
          href: routes.platform.territories.root,
          icon: MapPinned,
          permission: permissions.territories.read,
        },
        {
          id: "plate-formats",
          titleKey: "layout.nav_plate_formats",
          href: routes.platform.plateFormats.root,
          icon: RectangleHorizontal,
          permission: permissions.settings.read,
        },
        {
          id: "exchange-rates",
          titleKey: "layout.nav_exchange_rates",
          href: routes.platform.exchangeRates.root,
          icon: Coins,
          permission: permissions.rates.read,
        },
      ],
    },
    {
      id: "templates",
      labelKey: "layout.settings_group_templates",
      icon: FileText,
      items: [
        {
          id: "document-templates",
          titleKey: "layout.nav_document_templates",
          href: routes.platform.documentTemplates.root,
          icon: FileText,
          permission: permissions.documentTemplates.read,
        },
        {
          // TEC-290: contract templates (intake_contracts module; the API
          // answers 403 FEATURE_DISABLED while the module is off).
          id: "contract-templates",
          titleKey: "layout.nav_contract_templates",
          href: routes.platform.contractTemplates.root,
          icon: FileSignature,
          permission: permissions.contractTemplates.manage,
        },
      ],
    },
    {
      id: "data",
      labelKey: "layout.settings_group_data",
      icon: HardDrive,
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
          href: routes.platform.settings.exportDefaults,
          icon: Settings2,
          permission: permissions.settings.read,
        },
        {
          id: "activity",
          titleKey: "layout.nav_activity",
          href: routes.platform.activity.root,
          icon: Activity,
          permission: permissions.activity.read,
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
      ],
    },
  ],
});

/**
 * Tenant settings. Gates mirror the backend role catalog
 * (backend/internal/platform/rbac/catalog.go): showcase.read for dealer and
 * distributor, photo_standard.override for the distributor owner,
 * measurement_devices.manage, ai.usage.read, and the owner-only data
 * transfer pages (tenant.imports.read / tenant.settings.read).
 */
export function tenantSettingsNav(slug: string): NavCatalog {
  return defineNav({
    id: "tenant-settings",
    groups: [
      {
        id: "organization",
        labelKey: "layout.settings_group_organization",
        icon: Blocks,
        items: [
          {
            // Özellikler belongs to the core organizations module (always
            // on), so the entry needs no `feature` gate.
            id: "features",
            titleKey: "layout.nav_features",
            href: routes.tenant.features.root(slug),
            icon: Blocks,
          },
          {
            id: "showcase",
            titleKey: "showcase.nav",
            href: routes.tenant.showcase.root(slug),
            icon: BadgeCheck,
            permission: permissions.showcase.read,
            feature: "dealer_showcase",
            orgTypes: ["dealer", "distributor"],
          },
          {
            // TEC-500: the distributor's photo standard overrides.
            id: "photo-standard",
            titleKey: "photo_standard.settings.nav",
            href: routes.tenant.photoStandard.root(slug),
            icon: Camera,
            permission: permissions.photoStandard.override,
            feature: "photo_standard",
            orgTypes: ["distributor"],
          },
        ],
      },
      {
        id: "operations",
        labelKey: "layout.settings_group_operations",
        icon: Cpu,
        items: [
          {
            // TEC-299: NexPTG devices (same gates as /v1/measurement-devices).
            id: "measurement-devices",
            titleKey: "measurements.nav_devices",
            href: routes.tenant.measurements.devices(slug),
            icon: Cpu,
            permission: permissions.measurements.devicesManage,
            feature: "measurements",
          },
        ],
      },
      {
        id: "ai",
        labelKey: "layout.settings_group_ai",
        icon: Bot,
        // TEC-391: the organization's AI usage report (ai.usage.read).
        items: [aiUsageNavItem(slug)],
      },
      {
        id: "data",
        labelKey: "layout.settings_group_data",
        icon: HardDrive,
        items: [
          {
            id: "tenant-imports",
            titleKey: "layout.nav_imports",
            href: routes.tenant.imports.root(slug),
            icon: Upload,
            permission: permissions.imports.tenantRead,
            orgRoles: ["owner"],
          },
          {
            id: "tenant-exports",
            titleKey: "layout.nav_exports",
            href: routes.tenant.exports.root(slug),
            icon: Download,
            permission: permissions.imports.tenantRead,
            orgRoles: ["owner"],
          },
          {
            id: "tenant-export-settings",
            titleKey: "layout.nav_export_settings",
            href: routes.tenant.settings.exportDefaults(slug),
            icon: Settings2,
            permission: permissions.settings.tenantRead,
            orgRoles: ["owner"],
          },
        ],
      },
    ],
  });
}
