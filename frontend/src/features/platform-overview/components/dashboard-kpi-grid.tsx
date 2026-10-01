"use client";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { DashboardStatCard } from "@/features/platform-overview/components/dashboard-stat-card";
import type { PlatformOverviewStats } from "@/features/platform-overview/services/overview.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type DashboardKpiGridProps = {
  data?: PlatformOverviewStats;
  loading?: boolean;
};

type KpiDef = {
  key: keyof PlatformOverviewStats;
  labelKey: string;
  href?: string;
  optional?: boolean;
};

export function DashboardKpiGrid({ data, loading }: DashboardKpiGridProps) {
  const { t, format } = useLocale();
  const { can } = usePermission();

  const cards: KpiDef[] = [
    {
      key: "rolesTotal",
      labelKey: "dashboard.kpi_roles_total",
      href: can(permissions.roles.read)
        ? routes.platform.roles.root
        : undefined,
    },
    {
      key: "usersTotal",
      labelKey: "dashboard.kpi_users_total",
      href: can(permissions.users.read)
        ? routes.platform.users.root
        : undefined,
    },
    {
      key: "notificationsTotal",
      labelKey: "dashboard.kpi_notifications",
      href: can(permissions.notifications.platformRead)
        ? routes.platform.notifications.root
        : undefined,
      optional: true,
    },
  ];

  return (
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
      {cards.map((card) => {
        const raw = data?.[card.key];
        if (card.optional && !loading && raw === undefined) return null;

        return (
          <DashboardStatCard
            key={card.key}
            label={t(card.labelKey)}
            value={format.number(raw)}
            href={card.href}
            loading={loading}
          />
        );
      })}
    </div>
  );
}
