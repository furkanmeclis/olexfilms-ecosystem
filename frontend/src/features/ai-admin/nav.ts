import { ChartColumn, Sparkles } from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNavItem } from "@/features/nav-engine";

/** TEC-391: AI settings, organization quotas and usage (super_admin). */
export const aiPlatformNavItem = defineNavItem({
  id: "ai-admin",
  titleKey: "ai_admin.nav",
  href: routes.platform.ai.root,
  icon: Sparkles,
  permission: permissions.aiAdmin.settingsManage,
});

/**
 * TEC-391: the organization's AI usage report. Not tied to the
 * `ai_assistant` module (TEC-389): past usage stays readable while the
 * module is off.
 */
export function aiUsageNavItem(slug: string) {
  return defineNavItem({
    id: "ai-usage",
    titleKey: "ai_admin.usage_nav",
    href: routes.tenant.aiUsage(slug),
    icon: ChartColumn,
    permission: permissions.aiAdmin.usageRead,
  });
}
