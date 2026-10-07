import { ChartPie, IdCard } from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNavGroup } from "@/features/nav-engine";

/**
 * TEC-349: staff cards / payments and the own-book reports. Same gates as
 * the API: the accounting module everywhere; staff cards need staff.manage
 * and, in a dealer, the dealer_accounting module (dealer_owner and
 * dealer_accounting hold it, dealer_staff does not); the reports need
 * accounting.read.
 */
export function staffReportsNavGroup(slug: string) {
  return defineNavGroup({
    id: "staff-reports",
    labelKey: "staff_reports.nav",
    icon: ChartPie,
    defaultOpen: true,
    feature: "accounting",
    anyPermission: [permissions.staff.manage, permissions.accounting.read],
    items: [
      {
        id: "staff-reports-staff",
        titleKey: "staff_reports.nav_staff",
        href: routes.tenant.staff.list(slug),
        icon: IdCard,
        permission: permissions.staff.manage,
        orgTypes: ["dealer"],
        feature: "dealer_accounting",
      },
      {
        id: "staff-reports-reports",
        titleKey: "staff_reports.nav_reports",
        href: routes.tenant.accountingReports(slug),
        icon: ChartPie,
        permission: permissions.accounting.read,
        feature: "accounting",
      },
    ],
  });
}
