"use client";

import { FileSpreadsheet } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import type { BreadcrumbItem } from "@/components/layout/breadcrumb";
import { PageHeader } from "@/components/layout/page-header";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useFeature } from "@/features/modules/hooks/use-features";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type EinvoiceSection = "list" | "billable" | "settings";

/** Gates of /v1/einvoices: einvoice.read, the brand center, the add-on. */
export function useEinvoiceAccess(slug: string) {
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const feature = useFeature(slug, "e_invoice");
  const allowed = can(permissions.einvoice.read) && org?.type === "center";
  return {
    allowed,
    loading: allowed && feature.isLoading,
    enabled: allowed && feature.enabled,
    canManage: can(permissions.einvoice.manage),
    canSettings: can(permissions.einvoice.settings),
  };
}

function SectionTabs({
  slug,
  active,
}: {
  slug: string;
  active?: EinvoiceSection;
}) {
  const { t } = useLocale();
  const tabs: { id: EinvoiceSection; href: string }[] = [
    { id: "list", href: routes.tenant.einvoices.list(slug) },
    { id: "billable", href: routes.tenant.einvoices.billable(slug) },
    { id: "settings", href: routes.tenant.einvoices.settings(slug) },
  ];
  return (
    <nav
      className="bg-muted text-muted-foreground inline-flex h-9 items-center rounded-lg p-1"
      aria-label={t("einvoice.title")}
    >
      {tabs.map((tab) => (
        <Link
          key={tab.id}
          href={tab.href}
          aria-current={active === tab.id ? "page" : undefined}
          className={cn(
            "rounded-md px-3 py-1 text-sm font-medium whitespace-nowrap transition-colors",
            active === tab.id
              ? "bg-background text-foreground shadow-sm"
              : "hover:text-foreground",
          )}
        >
          {t(`einvoice.tabs.${tab.id}`)}
        </Link>
      ))}
    </nav>
  );
}

/**
 * Frame of the e-invoice screens (TEC-504): header, section tabs and the
 * gates (einvoice.read, center organization, e_invoice add-on); children
 * render only when every gate passes.
 */
export function EinvoiceShell({
  slug,
  section,
  title,
  description,
  breadcrumbs,
  actions,
  children,
}: {
  slug: string;
  section?: EinvoiceSection;
  title: string;
  description?: string;
  breadcrumbs?: BreadcrumbItem[];
  actions?: ReactNode;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const access = useEinvoiceAccess(slug);

  let body: ReactNode;
  if (!access.allowed) {
    body = (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t("einvoice.forbidden")}
      />
    );
  } else if (access.loading) {
    body = <Loading />;
  } else if (!access.enabled) {
    body = (
      <ErrorState
        title={t("einvoice.module_off_title")}
        description={t("einvoice.module_off")}
      />
    );
  } else {
    body = children;
  }

  return (
    <div className="space-y-6" data-testid="einvoice-shell">
      <PageHeader
        title={title}
        description={description}
        icon={<FileSpreadsheet className="size-6" />}
        breadcrumbs={
          breadcrumbs ?? [
            {
              label: t("layout.breadcrumb_home"),
              href: routes.tenant.home(slug),
            },
            { label: t("einvoice.title") },
          ]
        }
        actions={access.enabled ? actions : undefined}
      />
      {access.enabled && section ? (
        <SectionTabs slug={slug} active={section} />
      ) : null}
      {body}
    </div>
  );
}
