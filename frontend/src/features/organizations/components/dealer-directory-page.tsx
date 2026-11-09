"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Store, ShoppingCart } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  organizationsService,
  type Organization,
} from "@/features/organizations/services/organizations.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const DEALER_DIRECTORY_PERSIST_KEY = "tenant-dealer-directory-v1";
const DIRECTORY_TYPES = ["dealer", "distributor"] as const;

/** Orders list of the active organization filtered on one buyer. */
export function dealerOrdersHref(slug: string, uuid: string): string {
  return `${routes.tenant.orders.list(slug)}?buyer_org_uuid=${encodeURIComponent(uuid)}`;
}

/**
 * TEC-1066: read-only dealer directory of the center / distributor (legacy
 * warehouse "Müşteriler (Bayiler)"): name, location, phone and address of
 * every dealer (and distributor) in the organizations.read scope, with a
 * link to the orders they placed. The API returns one page of up to 200, so
 * the table sorts, filters and searches in the browser.
 */
export function DealerDirectoryPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const canOrders = can(permissions.orders.read);
  const list = useQuery({
    queryKey: ["organizations", "tenant", "directory"] as const,
    queryFn: () => organizationsService.listInScope(),
    staleTime: 5 * 60_000,
  });
  const rows = useMemo(
    () =>
      (list.data ?? []).filter(
        (o) => o.uuid !== org?.uuid && o.type !== "center",
      ),
    [list.data, org?.uuid],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<Organization>({
          accessorKey: "name",
          labelKey: "organizations.directory.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium" data-testid="dealer-directory-row">
              {row.original.name}
            </span>
          ),
        }),
        createColumn<Organization>({
          accessorKey: "type",
          labelKey: "organizations.directory.columns.type",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: DIRECTORY_TYPES.map((value) => ({
            value,
            label: value,
            labelKey: `organizations.types.${value}`,
          })),
          cell: ({ row }) => t(`organizations.types.${row.original.type}`),
        }),
        createColumn<Organization>({
          id: "location",
          accessorFn: (o) =>
            [o.city, o.district].filter((v) => v && v.trim()).join(" / "),
          labelKey: "organizations.directory.columns.location",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ getValue }) => String(getValue() || "—"),
        }),
        createColumn<Organization>({
          accessorKey: "phone",
          labelKey: "organizations.directory.columns.phone",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.phone ? (
              <a
                href={`tel:${row.original.phone}`}
                className="whitespace-nowrap hover:underline"
                dir="ltr"
                onClick={(e) => e.stopPropagation()}
              >
                {row.original.phone}
              </a>
            ) : (
              "—"
            ),
        }),
        createColumn<Organization>({
          accessorKey: "address",
          labelKey: "organizations.directory.columns.address",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="line-clamp-2 max-w-md">
              {row.original.address || "—"}
            </span>
          ),
        }),
        ...(canOrders
          ? [
              createColumn<Organization>({
                id: "orders",
                labelKey: "organizations.directory.columns.orders",
                enableSorting: false,
                cell: ({ row }) => (
                  <Button asChild variant="outline" size="sm">
                    <Link
                      href={dealerOrdersHref(slug, row.original.uuid)}
                      onClick={(e) => e.stopPropagation()}
                      data-testid="dealer-directory-orders"
                    >
                      <ShoppingCart className="size-3.5" />
                      {t("organizations.directory.orders")}
                    </Link>
                  </Button>
                ),
              }),
            ]
          : []),
      ] as ColumnDef<Organization, unknown>[],
    [canOrders, slug, t],
  );

  const title = t("organizations.directory.title");
  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={title}
        icon={<Store className="size-6" />}
        description={t("organizations.directory.description")}
        breadcrumbs={[
          {
            label: t("layout.breadcrumb_home"),
            href: routes.tenant.home(slug),
          },
          { label: title },
        ]}
      />
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("organizations.directory.empty_title")}
        emptyDescription={t("organizations.directory.empty_description")}
        features={{
          persistKey: DEALER_DIRECTORY_PERSIST_KEY,
          rowSelection: false,
        }}
      />
    </div>
  );
}
