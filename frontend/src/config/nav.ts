import {
  Activity,
  ArrowLeftRight,
  Bell,
  BellRing,
  BookOpen,
  Building2,
  Car,
  ClipboardList,
  Blocks,
  Bot,
  Boxes,
  Coins,
  Download,
  FileSignature,
  FileText,
  Flame,
  FolderTree,
  HardDrive,
  KeyRound,
  Landmark,
  LayoutDashboard,
  ListChecks,
  LogIn,
  MapPinned,
  MessageCircle,
  MessageSquareText,
  MessageSquareWarning,
  Megaphone,
  Package,
  RectangleHorizontal,
  ScrollText,
  Settings2,
  Shield,
  ShoppingCart,
  SlidersHorizontal,
  ShieldAlert,
  ShieldCheck,
  Upload,
  UserRound,
  Users,
  Wrench,
  ListTodo,
  Plus,
  Warehouse,
  ScanLine,
  PackagePlus,
  Barcode,
  ClipboardCheck,
  Sunset,
  ChartColumn,
  Gauge,
  Cpu,
  CalendarClock,
  Inbox,
  Plug,
  Repeat,
} from "lucide-react";

import { appleNavIcon } from "@/components/icons/apple-icon";
import { facebookNavIcon } from "@/components/icons/facebook-icon";
import { githubNavIcon } from "@/components/icons/github-icon";
import { googleNavIcon } from "@/components/icons/google-icon";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { aiPlatformNavItem, aiUsageNavItem } from "@/features/ai-admin/nav";
import { announcementsNavAdornment } from "@/features/announcements/nav";
import { conversationsNavItem } from "@/features/conversations/nav";
import { pendingActionsNavItem } from "@/features/mcp/nav";
import { dealerSalesNavGroup } from "@/features/dealer-sales/nav";
import { leadsNavItem } from "@/features/leads/nav";
import { staffReportsNavGroup } from "@/features/staff-reports/nav";
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
        // TEC-399: WhatsApp inbox, platform admin only (S2).
        conversationsNavItem,
        {
          // TEC-403: registered MCP clients (AI apps).
          id: "mcp-clients",
          titleKey: "mcp.clients.nav",
          href: routes.platform.mcpClients,
          icon: Plug,
          permission: permissions.mcp.clientsManage,
        },
        {
          id: "notification-center",
          titleKey: "layout.nav_notification_center",
          href: routes.platform.notifications.center,
          icon: BellRing,
          permission: permissions.notifications.templatesManage,
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
        {
          id: "modules",
          titleKey: "layout.nav_modules",
          href: routes.platform.modules.root,
          icon: Blocks,
          permission: permissions.modules.platformRead,
        },
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
          // TEC-290: contract templates (intake_contracts module; the API
          // answers 403 FEATURE_DISABLED while the module is off).
          id: "contract-templates",
          titleKey: "layout.nav_contract_templates",
          href: routes.platform.contractTemplates.root,
          icon: FileSignature,
          permission: permissions.contractTemplates.manage,
        },
        {
          // TEC-353: questions under the portal service review ratings.
          id: "review-questions",
          titleKey: "layout.nav_review_questions",
          href: routes.platform.reviewQuestions.root,
          icon: MessageSquareText,
          permission: permissions.reviews.questionsManage,
        },
        {
          // TEC-222: system settings hub (platform.settings.read).
          id: "system-settings",
          titleKey: "layout.nav_system_settings",
          href: routes.platform.systemSettings.root,
          icon: SlidersHorizontal,
          permission: permissions.settings.read,
        },
        // TEC-391: AI settings, organization quotas and usage report.
        aiPlatformNavItem,
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
          id: "legal-texts",
          titleKey: "layout.nav_legal_texts",
          href: routes.platform.legalTexts,
          icon: ScrollText,
          permission: permissions.legalTexts.write,
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
 * distributor | dealer), member role (orgRoles) and module (feature: a key
 * from GET /v1/features, hidden while the module is off); tenant I/O routes
 * are owner-only on the API (RequireOrgRole).
 */
const SERVICE_WIZARD_PERMISSIONS = [
  permissions.services.write,
  permissions.customers.read,
  permissions.vehicles.read,
];

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
          {
            // Özellikler belongs to the core organizations module (always
            // on), so the entry needs no `feature` gate.
            id: "features",
            titleKey: "layout.nav_features",
            href: routes.tenant.features.root(slug),
            icon: Blocks,
          },
          {
            // TEC-390: AI assistant (ai_assistant module + ai.use; the
            // header button opens the same chat in a sheet).
            id: "assistant",
            titleKey: "ai.nav_title",
            href: routes.tenant.assistant(slug),
            icon: Bot,
            permission: permissions.ai.use,
            feature: "ai_assistant",
          },
          // TEC-403: MCP / WhatsApp write tools waiting for approval.
          pendingActionsNavItem(slug),
          // TEC-391: the organization's AI usage report (ai.usage.read).
          aiUsageNavItem(slug),
        ],
      },
      {
        // TEC-147: every level reads the brand catalog (catalog.read); write
        // controls inside the pages are center only (K4).
        id: "catalog",
        labelKey: "layout.section_catalog",
        icon: Package,
        defaultOpen: true,
        permission: permissions.catalog.read,
        feature: "catalog",
        items: [
          {
            id: "catalog-products",
            titleKey: "layout.nav_catalog_products",
            href: routes.tenant.catalog.products(slug),
            icon: Package,
            permission: permissions.catalog.read,
            feature: "catalog",
          },
          {
            id: "catalog-categories",
            titleKey: "layout.nav_catalog_categories",
            href: routes.tenant.catalog.categories(slug),
            icon: FolderTree,
            permission: permissions.catalog.read,
            feature: "catalog",
          },
        ],
      },
      {
        // TEC-311: subscriptions (service_subscriptions.read) and the center
        // cancellation queue (cancel_approve) live next to the catalog.
        id: "service-catalog",
        labelKey: "layout.nav_service_catalog",
        icon: ClipboardList,
        defaultOpen: true,
        anyPermission: [
          permissions.serviceCatalog.read,
          permissions.serviceSubscriptions.read,
        ],
        feature: "service_catalog",
        items: [
          {
            id: "service-catalog",
            titleKey: "layout.nav_service_catalog",
            href: routes.tenant.catalog.services(slug),
            icon: ClipboardList,
            permission: permissions.serviceCatalog.read,
            feature: "service_catalog",
          },
          {
            id: "service-subscriptions",
            titleKey: "layout.nav_service_subscriptions",
            href: routes.tenant.serviceSubscriptions.list(slug),
            icon: Repeat,
            permission: permissions.serviceSubscriptions.read,
            feature: "service_catalog",
          },
          {
            id: "service-subscription-cancels",
            titleKey: "layout.nav_service_subscription_cancels",
            href: routes.tenant.serviceSubscriptions.cancelRequests(slug),
            icon: Inbox,
            permission: permissions.serviceSubscriptions.cancelApprove,
            feature: "service_catalog",
            orgTypes: ["center"],
          },
        ],
      },
      {
        // TEC-181: the service wizard needs services.write plus reading the
        // customers and vehicles it picks from (same gates as the API).
        // TEC-183: the list needs services.read only.
        id: "services",
        labelKey: "services.nav",
        icon: Wrench,
        defaultOpen: true,
        // TEC-191: the warranty list needs warranties.read (same scopes as
        // services.read).
        anyPermission: [
          permissions.services.read,
          permissions.services.write,
          permissions.warranties.read,
          permissions.warrantyClaims.read,
        ],
        feature: "services",
        items: [
          {
            id: "services-list",
            titleKey: "services.nav_list",
            href: routes.tenant.services.list(slug),
            icon: ClipboardList,
            permission: permissions.services.read,
            feature: "services",
          },
          {
            id: "services-new",
            titleKey: "services.nav_new",
            href: routes.tenant.services.create(slug),
            icon: Wrench,
            permission: SERVICE_WIZARD_PERMISSIONS,
            feature: "services",
          },
          {
            id: "warranties-list",
            titleKey: "warranty.nav_list",
            href: routes.tenant.warranties.list(slug),
            icon: ShieldCheck,
            permission: permissions.warranties.read,
            feature: "services",
          },
          {
            // TEC-339: warranty claims (dealer, distributor, center).
            id: "warranty-claims-list",
            titleKey: "warranty.claims.nav",
            href: routes.tenant.warrantyClaims.list(slug),
            icon: ShieldAlert,
            permission: permissions.warrantyClaims.read,
            feature: "warranty_claims",
          },
          {
            // TEC-340: center / distributor warranty claim report.
            id: "warranty-claim-reports",
            titleKey: "warranty.claim_reports.nav",
            href: routes.tenant.warrantyClaims.reports(slug),
            icon: ChartColumn,
            permission: permissions.warrantyClaims.read,
            orgTypes: ["center", "distributor"],
            feature: "warranty_claims",
          },
        ],
      },
      {
        // TEC-163: customers need customers.read and the customers module
        // (same gates as /v1/customers); a new customer customers.write.
        id: "customers",
        labelKey: "customers.nav",
        icon: Users,
        defaultOpen: true,
        permission: permissions.customers.read,
        feature: "customers",
        items: [
          {
            id: "customers-list",
            titleKey: "customers.nav_list",
            href: routes.tenant.customers.list(slug),
            icon: Users,
            permission: permissions.customers.read,
            feature: "customers",
          },
          {
            id: "customers-new",
            titleKey: "customers.nav_new",
            href: routes.tenant.customers.create(slug),
            icon: UserRound,
            permission: [
              permissions.customers.read,
              permissions.customers.write,
            ],
            feature: "customers",
          },
          {
            // TEC-372: tenant vehicle list (GET /v1/vehicles: vehicles.read
            // and the customers module, like the vehicle detail).
            id: "vehicles-list",
            titleKey: "vehicles.list.nav",
            href: routes.tenant.vehicles.list(slug),
            icon: Car,
            permission: permissions.vehicles.read,
            feature: "customers",
          },
        ],
      },
      {
        // TEC-326: the appointment calendar needs appointments.read and the
        // appointments module (same gates as /v1/appointments).
        id: "appointments",
        labelKey: "appointments.nav",
        icon: CalendarClock,
        defaultOpen: true,
        permission: permissions.appointments.read,
        feature: "appointments",
        items: [
          {
            id: "appointments-calendar",
            titleKey: "appointments.nav_calendar",
            href: routes.tenant.appointments.calendar(slug),
            icon: CalendarClock,
            permission: permissions.appointments.read,
            feature: "appointments",
          },
          {
            // TEC-328: network occupancy (GET /v1/appointments/occupancy),
            // center and distributor only; a dealer has no network.
            id: "appointments-occupancy",
            titleKey: "appointments.occupancy.nav",
            href: routes.tenant.appointments.occupancy(slug),
            icon: Flame,
            permission: permissions.appointments.read,
            orgTypes: ["center", "distributor"],
            feature: "appointments",
          },
        ],
      },
      {
        // TEC-299: paint measurements need measurements.read, the NexPTG
        // devices measurement_devices.manage; both behind the measurements
        // module (same gates as /v1/measurements, /v1/measurement-devices).
        id: "measurements",
        labelKey: "measurements.nav",
        icon: Gauge,
        defaultOpen: true,
        anyPermission: [
          permissions.measurements.read,
          permissions.measurements.devicesManage,
        ],
        feature: "measurements",
        items: [
          {
            id: "measurements-list",
            titleKey: "measurements.nav_list",
            href: routes.tenant.measurements.list(slug),
            icon: Gauge,
            permission: permissions.measurements.read,
            feature: "measurements",
          },
          {
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
        // TEC-170: the order list needs orders.read and the orders module
        // (same gates as /v1/orders). A new order needs orders.write plus
        // the catalog to pick from, and a supplier: the center buys from
        // nobody (K6).
        id: "orders",
        labelKey: "orders.nav",
        icon: ShoppingCart,
        defaultOpen: true,
        permission: permissions.orders.read,
        feature: "orders",
        items: [
          {
            id: "orders-list",
            titleKey: "orders.nav_list",
            href: routes.tenant.orders.list(slug),
            icon: ClipboardList,
            permission: permissions.orders.read,
            feature: "orders",
          },
          {
            id: "orders-new",
            titleKey: "orders.nav_new",
            href: routes.tenant.orders.create(slug),
            icon: ShoppingCart,
            permission: [permissions.orders.write, permissions.catalog.read],
            orgTypes: ["distributor", "dealer"],
            feature: "orders",
          },
        ],
      },
      // TEC-349: staff cards, payments and the own-book reports.
      staffReportsNavGroup(slug),
      {
        // TEC-176: the book reads need accounting.read and the accounting
        // module (same gates as /v1/accounting); write controls inside the
        // pages follow accounting.write and the organization type.
        id: "accounting",
        labelKey: "accounting.nav",
        icon: BookOpen,
        defaultOpen: true,
        permission: permissions.accounting.read,
        feature: "accounting",
        items: [
          {
            id: "accounting-accounts",
            titleKey: "accounting.nav_accounts",
            href: routes.tenant.accounting.accounts(slug),
            icon: Landmark,
            permission: permissions.accounting.read,
            feature: "accounting",
          },
          {
            id: "accounting-cari",
            titleKey: "accounting.nav_cari",
            href: routes.tenant.accounting.cari(slug),
            icon: Users,
            permission: permissions.accounting.read,
            feature: "accounting",
          },
          {
            id: "accounting-entries",
            titleKey: "accounting.nav_entries",
            href: routes.tenant.accounting.entries(slug),
            icon: ScrollText,
            permission: permissions.accounting.read,
            feature: "accounting",
          },
          {
            // TEC-195: a child sees the disputes it opened, the parent the
            // ones addressed to it (K24).
            id: "accounting-disputes",
            titleKey: "accounting.nav_disputes",
            href: routes.tenant.accounting.disputes(slug),
            icon: MessageSquareWarning,
            permission: permissions.accounting.read,
            feature: "accounting",
          },
        ],
      },
      // TEC-348: dealer sale prices, quick sale, suppliers, purchases.
      dealerSalesNavGroup(slug),
      {
        // TEC-197: stock transfer requests between sibling dealers or
        // distributors (K13); same gates as /v1/stock-transfers (the
        // dealer_transfers module and transfers.request or
        // transfers.approve). A new request needs transfers.request and is
        // made by a dealer or a distributor.
        id: "transfers",
        labelKey: "transfers.nav",
        icon: ArrowLeftRight,
        defaultOpen: true,
        anyPermission: [
          permissions.transfers.request,
          permissions.transfers.approve,
        ],
        feature: "dealer_transfers",
        items: [
          {
            id: "transfers-list",
            titleKey: "transfers.nav_list",
            href: routes.tenant.transfers.list(slug),
            icon: ArrowLeftRight,
            anyPermission: [
              permissions.transfers.request,
              permissions.transfers.approve,
            ],
            feature: "dealer_transfers",
          },
          {
            id: "transfers-new",
            titleKey: "transfers.nav_new",
            href: routes.tenant.transfers.create(slug),
            icon: Package,
            permission: permissions.transfers.request,
            orgTypes: ["distributor", "dealer"],
            feature: "dealer_transfers",
          },
        ],
      },
      {
        // TEC-224: the dealer's own stock (K12: units, no bins or counts);
        // same gates as /v1/stock/organizations/{uuid}/units (stock.read
        // and the stock module). A distributor reads its dealers too.
        id: "stock",
        labelKey: "stock.nav",
        icon: Boxes,
        defaultOpen: true,
        permission: permissions.stock.read,
        feature: "stock",
        orgTypes: ["distributor", "dealer"],
        items: [
          {
            id: "stock-mine",
            titleKey: "stock.nav_mine",
            href: routes.tenant.stock.root(slug),
            icon: Boxes,
            permission: permissions.stock.read,
            feature: "stock",
            orgTypes: ["distributor", "dealer"],
          },
        ],
      },
      {
        // TEC-332: published announcements for the full network. Center can
        // target all audiences; distributors can target their subtree.
        id: "announcements",
        labelKey: "announcements.nav",
        icon: Megaphone,
        defaultOpen: true,
        permission: permissions.announcements.read,
        feature: "announcements",
        items: [
          {
            id: "announcements-list",
            titleKey: "announcements.nav_list",
            href: routes.tenant.announcements.list(slug),
            icon: Megaphone,
            permission: permissions.announcements.read,
            feature: "announcements",
            Adornment: announcementsNavAdornment,
          },
        ],
      },
      {
        // TEC-408: push / WhatsApp / e-mail campaigns with approval flow.
        id: "campaigns",
        labelKey: "campaigns.nav",
        icon: Megaphone,
        defaultOpen: true,
        permission: permissions.campaigns.read,
        feature: "campaigns",
        items: [
          {
            id: "campaigns-list",
            titleKey: "campaigns.nav_list",
            href: routes.tenant.campaigns.list(slug),
            icon: Megaphone,
            permission: permissions.campaigns.read,
            feature: "campaigns",
          },
          {
            id: "campaigns-new",
            titleKey: "campaigns.nav_new",
            href: routes.tenant.campaigns.create(slug),
            icon: Plus,
            permission: permissions.campaigns.write,
            feature: "campaigns",
          },
          {
            id: "campaigns-approvals",
            titleKey: "campaigns.nav_approvals",
            href: routes.tenant.campaigns.approvals(slug),
            icon: ListChecks,
            permission: permissions.campaigns.read,
            feature: "campaigns",
          },
        ],
      },
      {
        // TEC-333: center-managed document library (announcements module).
        id: "library",
        labelKey: "library.nav",
        icon: FileText,
        defaultOpen: true,
        permission: permissions.library.read,
        feature: "announcements",
        items: [
          {
            id: "library-list",
            titleKey: "library.nav_list",
            href: routes.tenant.library.list(slug),
            icon: FileText,
            permission: permissions.library.read,
            feature: "announcements",
          },
        ],
      },
      {
        // TEC-318: organization-scoped lead pipeline. The item has a live
        // follow-up badge and stays behind the leads module flag.
        id: "leads",
        labelKey: "leads.nav",
        icon: Flame,
        defaultOpen: true,
        permission: permissions.leads.read,
        feature: "leads",
        items: [leadsNavItem(slug)],
      },
      {
        // TEC-221: center tasks about distributors and dealers (center
        // roles only hold tasks.*; same gates as /v1/tasks).
        id: "tasks",
        labelKey: "tasks.nav",
        icon: ListTodo,
        defaultOpen: true,
        permission: permissions.tasks.read,
        orgTypes: ["center"],
        items: [
          {
            id: "tasks-list",
            titleKey: "tasks.nav_list",
            href: routes.tenant.tasks.list(slug),
            icon: ListTodo,
            permission: permissions.tasks.read,
            orgTypes: ["center"],
          },
          {
            id: "tasks-new",
            titleKey: "tasks.nav_new",
            href: routes.tenant.tasks.create(slug),
            icon: Plus,
            permission: permissions.tasks.write,
            orgTypes: ["center"],
          },
        ],
      },
      {
        // TEC-231: the full warehouse of the center and the distributor
        // (K12; a dealer has "My stock"). Same gates as /v1/warehouse/*
        // (warehouse.read and the warehouse module). Barcode batches are
        // center only (K14, stock.read like /v1/stock/barcodes). TEC-232
        // adds transfers, counts and end of day to this group.
        id: "warehouse",
        labelKey: "warehouse.nav",
        icon: Warehouse,
        defaultOpen: true,
        permission: permissions.warehouse.read,
        feature: "warehouse",
        orgTypes: ["center", "distributor"],
        items: [
          {
            id: "warehouse-locations",
            titleKey: "warehouse.nav_locations",
            href: routes.tenant.warehouse.locations(slug),
            icon: FolderTree,
            permission: permissions.warehouse.read,
            feature: "warehouse",
            orgTypes: ["center", "distributor"],
          },
          {
            id: "warehouse-scan",
            titleKey: "warehouse.nav_scan",
            href: routes.tenant.warehouse.scan(slug),
            icon: ScanLine,
            permission: permissions.warehouse.read,
            feature: "warehouse",
            orgTypes: ["center", "distributor"],
          },
          {
            id: "warehouse-entries",
            titleKey: "warehouse.nav_entries",
            href: routes.tenant.warehouse.entries(slug),
            icon: PackagePlus,
            permission: permissions.warehouse.read,
            feature: "warehouse",
            orgTypes: ["center", "distributor"],
          },
          {
            id: "warehouse-barcodes",
            titleKey: "warehouse.nav_barcodes",
            href: routes.tenant.warehouse.barcodes(slug),
            icon: Barcode,
            permission: [permissions.warehouse.read, permissions.stock.read],
            feature: "warehouse",
            orgTypes: ["center"],
          },
          // TEC-232 (slice 2): transfers, counts, end of day.
          {
            id: "warehouse-transfers",
            titleKey: "warehouse.nav_transfers",
            href: routes.tenant.warehouse.transfers(slug),
            icon: ArrowLeftRight,
            permission: permissions.warehouse.read,
            feature: "warehouse",
            orgTypes: ["center", "distributor"],
          },
          {
            id: "warehouse-counts",
            titleKey: "warehouse.nav_counts",
            href: routes.tenant.warehouse.counts(slug),
            icon: ClipboardCheck,
            permission: permissions.warehouse.read,
            feature: "warehouse",
            orgTypes: ["center", "distributor"],
          },
          {
            id: "warehouse-end-of-day",
            titleKey: "warehouse.nav_end_of_day",
            href: routes.tenant.warehouse.endOfDay(slug),
            icon: Sunset,
            permission: permissions.warehouse.read,
            feature: "warehouse",
            orgTypes: ["center", "distributor"],
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
