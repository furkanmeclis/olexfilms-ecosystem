"use client";

import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type WarehouseAccess = {
  /** warehouse.read in a center or distributor organization (K12). */
  allowed: boolean;
  isCenter: boolean;
  orgType: string | null;
  can: (permission: string) => boolean;
};

/**
 * Who may open the full warehouse: center and distributor organizations
 * with warehouse.read (K12; a dealer has the simple "My stock" page).
 * The backend enforces the same plus the warehouse module.
 */
export function useWarehouseAccess(
  slug: string,
  permission: string = Permission.WarehouseRead,
): WarehouseAccess {
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const orgType = org?.type ?? null;
  const fullWarehouse = orgType === "center" || orgType === "distributor";
  return {
    allowed: fullWarehouse && can(permission),
    isCenter: orgType === "center",
    orgType,
    can,
  };
}

/** Page chrome of every warehouse screen: header, breadcrumbs, gate. */
export function WarehouseShell({
  slug,
  access,
  title,
  description,
  icon,
  actions,
  crumbs = [],
  forbiddenKey = "warehouse.page.forbidden",
  children,
}: {
  slug: string;
  access: WarehouseAccess;
  title: string;
  description?: string;
  icon?: ReactNode;
  actions?: ReactNode;
  /** Extra breadcrumbs between "Warehouse" and the title. */
  crumbs?: { label: string; href?: string }[];
  forbiddenKey?: string;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const isRoot = title === t("warehouse.page.title");
  return (
    <div className="space-y-6">
      <PageHeader
        title={title}
        icon={icon}
        description={description}
        breadcrumbs={[
          {
            label: t("layout.breadcrumb_home"),
            href: routes.tenant.home(slug),
          },
          ...(isRoot
            ? []
            : [
                {
                  label: t("warehouse.page.title"),
                  href: routes.tenant.warehouse.root(slug),
                },
              ]),
          ...crumbs,
          { label: title },
        ]}
        actions={access.allowed ? actions : null}
      />
      {access.allowed ? (
        children
      ) : (
        <ErrorState
          title={t("common.error_forbidden")}
          description={t(forbiddenKey)}
        />
      )}
    </div>
  );
}
