"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { permissions } from "@/config/permissions";
import {
  customerKeys,
  customersService,
} from "@/features/customers/services/customers.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { usePermission } from "@/providers/permission-provider";

/**
 * Organization filter options of the service and warranty lists (TEC-378,
 * `organization_uuid` of TEC-377): center and distributor users see several
 * organizations, a dealer only its own, so the filter is off for dealers.
 * Shares the cache of the customer list's organization filter.
 */
export function useScopeOrganizationOptions(
  slug: string,
  listReadable: boolean,
) {
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const enabled =
    listReadable &&
    can(permissions.organizations.tenantRead) &&
    (org?.type === "center" || org?.type === "distributor");
  const organizations = useQuery({
    queryKey: customerKeys.organizations,
    queryFn: () => customersService.listOrganizations(),
    enabled,
    staleTime: 5 * 60_000,
  });
  const options = useMemo(
    () =>
      (organizations.data ?? []).map((o) => ({ value: o.uuid, label: o.name })),
    [organizations.data],
  );
  return { enabled: enabled && options.length > 0, options };
}
