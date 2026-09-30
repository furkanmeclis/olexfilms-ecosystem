"use client";

import { useMemo } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { DashboardKpiGrid } from "@/features/platform-overview/components/dashboard-kpi-grid";
import { OverviewQuickLinks } from "@/features/platform-overview/components/overview-quick-links";
import { useOverviewStats } from "@/features/platform-overview/hooks/use-overview-query";
import type { OverviewFetchScopes } from "@/features/platform-overview/services/overview.service";
import { isApiError } from "@/lib/api";
import { isPlatformUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export function OverviewPage() {
  const { t } = useLocale();
  const { user } = useAuth();
  const { can } = usePermission();

  const canOverview = isPlatformUser(user);

  const scopes = useMemo<OverviewFetchScopes>(
    () => ({
      roles: can(permissions.roles.read),
      users: can(permissions.users.read),
      notifications: can(permissions.notifications.platformRead),
    }),
    [can],
  );

  const statsQuery = useOverviewStats(scopes, canOverview);

  const statsError = statsQuery.error;
  const isForbidden =
    !canOverview || (isApiError(statsError) && statsError.isForbidden);

  const hasAnyScope = Boolean(
    scopes.roles || scopes.users || scopes.notifications,
  );

  return (
    <>
      <PageHeader
        title={t("layout.home")}
        description={t("dashboard.description")}
        breadcrumbs={[
          { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        ]}
      />

      {isForbidden ? (
        <EmptyState
          title={t("common.error_forbidden")}
          description={t("dashboard.forbidden_description")}
        />
      ) : statsError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={
            isApiError(statsError)
              ? statsError.message
              : t("dashboard.error_description")
          }
          onRetry={() => void statsQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <div className="space-y-8">
          {hasAnyScope ? (
            <DashboardKpiGrid
              data={statsQuery.data}
              loading={statsQuery.isLoading}
            />
          ) : null}
          <OverviewQuickLinks />
        </div>
      )}
    </>
  );
}
