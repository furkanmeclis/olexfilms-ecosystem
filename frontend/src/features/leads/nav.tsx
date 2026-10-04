"use client";

import { useQuery } from "@tanstack/react-query";
import { Flame } from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  createNavAdornment,
  defineNavItem,
  type NavAdornment,
} from "@/features/nav-engine";
import {
  leadKeys,
  leadsService,
} from "@/features/leads/services/leads.service";
import { useLocale } from "@/providers/locale-provider";

function useLeadNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const count = useQuery({
    queryKey: leadKeys.followUpCount,
    queryFn: () => leadsService.followUpCount(),
    refetchInterval: 30_000,
  });
  const overdue = count.data?.overdue ?? 0;
  const today = count.data?.today ?? 0;
  const total = overdue + today;
  return {
    badges: [
      {
        kind: "count",
        value: total,
        variant: overdue > 0 ? "danger" : "warning",
      },
    ],
    info: count.data
      ? {
          title: t("leads.nav_info.title"),
          rows: [
            {
              label: t("leads.follow_up.overdue"),
              value: overdue,
              tone: "danger",
            },
            {
              label: t("leads.follow_up.today"),
              value: today,
              tone: "warning",
            },
          ],
        }
      : null,
  };
}

export const LeadNavAdornment = createNavAdornment(useLeadNavAdornment);

export function leadsNavItem(slug: string) {
  return defineNavItem({
    id: "leads-list",
    titleKey: "leads.nav",
    href: routes.tenant.leads.list(slug),
    icon: Flame,
    permission: permissions.leads.read,
    feature: "leads",
    Adornment: LeadNavAdornment,
  });
}
