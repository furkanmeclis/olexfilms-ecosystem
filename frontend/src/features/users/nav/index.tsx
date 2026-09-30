"use client";

import { Users } from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  createNavAdornment,
  defineNavItem,
  type NavAdornment,
} from "@/features/nav-engine";
import { useUsersNavStats } from "@/features/users/nav/use-users-nav-stats";
import { useLocale } from "@/providers/locale-provider";

function useUsersNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const { data, isLoading } = useUsersNavStats();
  const total = data?.total ?? 0;
  const pending = data?.pending ?? 0;
  const ready = !isLoading && data;

  return {
    badges: [
      { kind: "count", value: total, variant: "secondary" },
      ...(pending > 0
        ? [
            {
              kind: "label" as const,
              text: t("users.nav.pending_badge", { count: pending }),
              variant: "warning" as const,
            },
            {
              kind: "custom" as const,
              id: "users-pending-pulse",
              render: () => (
                <span className="relative flex size-2" aria-hidden>
                  <span className="absolute inline-flex size-full animate-ping rounded-full bg-amber-400 opacity-75" />
                  <span className="relative inline-flex size-2 rounded-full bg-amber-500" />
                </span>
              ),
            },
          ]
        : []),
    ],
    info: ready
      ? {
          title: t("users.nav.info_title"),
          description: t("users.nav.info_description", { count: total }),
          rows: [
            {
              label: t("users.status.active"),
              value: data.active,
              tone: "success",
            },
            {
              label: t("users.status.pending"),
              value: data.pending,
              tone: "warning",
            },
            {
              label: t("users.status.disabled"),
              value: data.disabled,
              tone: "danger",
            },
          ],
        }
      : null,
  };
}

export const UsersNavAdornment = createNavAdornment(useUsersNavAdornment);

export const usersNavItem = defineNavItem({
  id: "users",
  titleKey: "layout.nav_users",
  href: routes.platform.users.root,
  icon: Users,
  permission: permissions.users.read,
  Adornment: UsersNavAdornment,
});
