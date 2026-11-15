"use client";

import { AccountingDashboard } from "@/features/accounting";
import { CenterOverview } from "@/features/overview/components/center-overview";
import { DealerOverview } from "@/features/overview/components/dealer-overview";
import { DistributorOverview } from "@/features/overview/components/distributor-overview";
import { resolveOverviewRole } from "@/features/overview/lib/role";
import { WarehouseDashboard } from "@/features/warehouse";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { usePermission } from "@/providers/permission-provider";

/**
 * TEC-1212: /t/[slug] overview by role (center, distributor, dealer,
 * warehouse, accounting). Each overview has one PeriodBar that drives all
 * of its panels.
 */
export function RoleOverview({ slug }: { slug: string }) {
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const role = resolveOverviewRole(can, org?.type);
  switch (role) {
    case "accounting":
      return <AccountingDashboard slug={slug} />;
    case "warehouse":
      return <WarehouseDashboard slug={slug} />;
    case "center":
      return <CenterOverview slug={slug} />;
    case "distributor":
      return <DistributorOverview slug={slug} />;
    default:
      return <DealerOverview slug={slug} />;
  }
}
