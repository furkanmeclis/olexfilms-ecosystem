"use client";

import { AccountingOverview } from "@/features/overview/components/accounting-overview";
import { CenterOverview } from "@/features/overview/components/center-overview";
import { DealerOverview } from "@/features/overview/components/dealer-overview";
import { DistributorOverview } from "@/features/overview/components/distributor-overview";
import { resolveOverviewRole } from "@/features/overview/lib/role";
import { WarehouseOverview } from "@/features/overview/components/warehouse-overview";
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
      return <AccountingOverview slug={slug} />;
    case "warehouse":
      return <WarehouseOverview slug={slug} />;
    case "center":
      return <CenterOverview slug={slug} />;
    case "distributor":
      return <DistributorOverview slug={slug} />;
    default:
      return <DealerOverview slug={slug} />;
  }
}
