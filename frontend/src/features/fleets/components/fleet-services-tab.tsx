"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { routes } from "@/config/routes";
import {
  fleetKeys,
  fleetsService,
  type FleetCard,
} from "@/features/fleets/services/fleets.service";
import {
  serviceStatusTone,
  vehicleTitle,
} from "@/features/services/lib/detail";
import { SERVICE_STATUSES } from "@/features/services/lib/list-filters";
import {
  serviceWizardService,
  type Service,
} from "@/features/services/services/service-wizard.service";
import { useLocale } from "@/providers/locale-provider";

export const FLEET_SERVICES_PERSIST_KEY = "tenant-fleet-services-v1";

/**
 * Fleet card > Services (TEC-477): GET /v1/services of the fleet's primary
 * user (the owner of the fleet vehicles); the API keeps a dealer to its
 * own services.
 */
export function FleetServicesTab({
  slug,
  fleet,
}: {
  slug: string;
  fleet: FleetCard;
}) {
  const { t, format } = useLocale();
  const router = useRouter();

  const primary = useQuery({
    queryKey: fleetKeys.primaryUser(fleet.uuid),
    queryFn: async () => {
      const page = await fleetsService.listUsers(fleet.uuid, {
        limit: 100,
        offset: 0,
        sort: "created_at",
      });
      return page.items.find((u) => u.is_primary) ?? null;
    },
    enabled: fleet.has_primary_user,
  });
  const customerUuid = primary.data?.user_uuid;

  const columns = useMemo(
    () =>
      [
        createColumn<Service>({
          accessorKey: "service_no",
          labelKey: "services.list.columns.service_no",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.services.detail(slug, row.original.uuid)}
              className="font-mono font-medium hover:underline"
              dir="ltr"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.service_no}
            </Link>
          ),
        }),
        createColumn<Service>({
          accessorKey: "status",
          labelKey: "services.list.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          param: "status",
          filterOptions: SERVICE_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `services.status.${value}`,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={row.original.status_label}
              tone={serviceStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Service>({
          id: "plate",
          accessorFn: (row) => row.plate ?? "",
          labelKey: "services.list.columns.vehicle",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <div className="min-w-0">
              <div>{vehicleTitle(row.original) || "—"}</div>
              {row.original.plate ? (
                <div
                  className="text-muted-foreground font-mono text-xs"
                  dir="ltr"
                >
                  {row.original.plate}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<Service>({
          id: "organization",
          accessorFn: (row) => row.organization.name,
          labelKey: "services.list.columns.organization",
          enableSorting: true,
          defaultHidden: true,
        }),
        createColumn<Service>({
          accessorKey: "created_at",
          labelKey: "services.list.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<Service>({
          accessorKey: "completed_at",
          labelKey: "services.list.columns.completed_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "completed",
          cell: ({ row }) =>
            row.original.completed_at
              ? format.dateTime(row.original.completed_at)
              : "—",
        }),
      ] as ColumnDef<Service, unknown>[],
    [format, slug],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey: FLEET_SERVICES_PERSIST_KEY,
  });
  const params = useMemo(
    () => ({ ...listState.params, customer_uuid: customerUuid }),
    [customerUuid, listState.params],
  );
  const list = useQuery({
    queryKey: ["fleets", fleet.uuid, "services", params],
    queryFn: () => serviceWizardService.listServices(params),
    enabled: Boolean(customerUuid),
  });

  return (
    <EntityTable
      columns={columns}
      data={customerUuid ? (list.data?.items ?? []) : []}
      getRowId={(row) => row.uuid}
      onRowClick={(s) =>
        router.push(routes.tenant.services.detail(slug, s.uuid))
      }
      isLoading={primary.isLoading || (Boolean(customerUuid) && list.isLoading)}
      isError={primary.isError || list.isError}
      onRetry={() => {
        void primary.refetch();
        void list.refetch();
      }}
      emptyTitle={t("fleets.services.empty_title")}
      emptyDescription={
        fleet.has_primary_user
          ? t("fleets.services.empty_description")
          : t("fleets.summary.no_primary_user")
      }
      rowCount={customerUuid ? (list.data?.total ?? 0) : 0}
      state={listState.tableState}
      features={{ persistKey: FLEET_SERVICES_PERSIST_KEY }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void list.refetch()}
          refreshDisabled={list.isFetching}
        />
      }
    />
  );
}
