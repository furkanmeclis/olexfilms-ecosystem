"use client";

import { ListChecks } from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { usePendingActionCount } from "@/features/mcp/hooks/use-mcp";
import {
  createNavAdornment,
  defineNavItem,
  type NavAdornment,
} from "@/features/nav-engine";
import { useLocale } from "@/providers/locale-provider";

function usePendingActionsNavAdornment(): NavAdornment {
  const { t } = useLocale();
  // Mounted only while the item is visible (ai.actions.confirm, mcp on).
  const count = usePendingActionCount(true);
  const value = count.data ?? 0;
  return {
    badges: [
      { kind: "count", value, variant: "warning", hiddenWhenZero: true },
    ],
    info: value
      ? {
          title: t("mcp.approvals.title"),
          rows: [{ label: t("mcp.approvals.nav_pending"), value }],
        }
      : null,
  };
}

export const PendingActionsNavAdornment = createNavAdornment(
  usePendingActionsNavAdornment,
);

/**
 * "Pending AI actions" (TEC-403): MCP / WhatsApp write tools waiting for
 * the user's approval, with the pending count as badge.
 */
export function pendingActionsNavItem(slug: string) {
  return defineNavItem({
    id: "assistant-approvals",
    titleKey: "mcp.approvals.nav",
    href: routes.tenant.assistantApprovals(slug),
    icon: ListChecks,
    permission: permissions.ai.actionsConfirm,
    feature: "mcp",
    Adornment: PendingActionsNavAdornment,
  });
}
