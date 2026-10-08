"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CalendarPlus, Trash2, Upload } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import {
  createColumn,
  createSelectColumnDef,
  type DataTableBulkAction,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  FLEET_VEHICLE_IMPORT_RESOURCE,
  fleetKeys,
  fleetsService,
  type FleetCard,
  type FleetVehicle,
} from "@/features/fleets/services/fleets.service";
import { ImportWizard } from "@/features/io";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const FLEET_VEHICLES_PERSIST_KEY = "tenant-fleet-vehicles-v1";

export function vehicleLabel(v: Pick<FleetVehicle, "car_brand" | "car_model">) {
  return [v.car_brand?.name, v.car_model?.name].filter(Boolean).join(" ");
}

/**
 * Fleet card > Vehicles (TEC-477): server DataTable over
 * GET /v1/fleets/{uuid}/vehicles (plate, brand / model, last service and
 * active warranties of the caller's own work); the selection opens the bulk
 * service plan wizard; the CSV / XLSX import runs with a dry-run preview.
 */
export function FleetVehiclesTab({
  slug,
  fleet,
  canPlan,
}: {
  slug: string;
  fleet: FleetCard;
  canPlan: boolean;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const qc = useQueryClient();
  const canManage = can(permissions.fleets.manage);
  const [importOpen, setImportOpen] = useState(false);

  const remove = useMutation({
    mutationFn: (v: FleetVehicle) =>
      fleetsService.removeVehicle(fleet.uuid, v.uuid),
    onSuccess: async () => {
      appToast.success(t("fleets.vehicles.removed"));
      await qc.invalidateQueries({ queryKey: ["fleets", fleet.uuid] });
      await qc.invalidateQueries({ queryKey: fleetKeys.card(fleet.uuid) });
    },
  });

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<FleetVehicle>(),
        createColumn<FleetVehicle>({
          accessorKey: "plate",
          labelKey: "fleets.fields.plate",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.vehicles.detail(slug, row.original.uuid)}
              className="font-mono font-medium hover:underline"
              dir="ltr"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.plate ?? "—"}
            </Link>
          ),
        }),
        createColumn<FleetVehicle>({
          id: "car_brand",
          accessorFn: (row) => vehicleLabel(row),
          labelKey: "fleets.vehicles.brand_model",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => vehicleLabel(row.original) || "—",
        }),
        createColumn<FleetVehicle>({
          accessorKey: "model_year",
          labelKey: "fleets.fields.model_year",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.model_year ?? "—",
        }),
        createColumn<FleetVehicle>({
          accessorKey: "vin",
          labelKey: "fleets.fields.vin",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.vin ?? "—"}
            </span>
          ),
        }),
        createColumn<FleetVehicle>({
          accessorKey: "last_service_at",
          labelKey: "fleets.columns.last_service_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.last_service_at
              ? format.date(row.original.last_service_at)
              : "—",
        }),
        createColumn<FleetVehicle>({
          accessorKey: "active_warranty_count",
          labelKey: "fleets.vehicles.active_warranties",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {format.number(row.original.active_warranty_count)}
            </span>
          ),
        }),
        createColumn<FleetVehicle>({
          accessorKey: "created_at",
          labelKey: "fleets.vehicles.added_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.date(row.original.created_at),
        }),
        createColumn<FleetVehicle>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "plan",
                  label: t("fleets.plan.create"),
                  icon: CalendarPlus,
                  disabled: !canPlan,
                  onSelect: () =>
                    router.push(
                      routes.tenant.fleets.newPlan(slug, fleet.uuid, [
                        row.original.uuid,
                      ]),
                    ),
                },
                {
                  id: "remove",
                  label: t("fleets.vehicles.remove"),
                  icon: Trash2,
                  variant: "destructive",
                  permission: permissions.fleets.manage,
                  onSelect: () => remove.mutate(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<FleetVehicle, unknown>[],
    [canPlan, fleet.uuid, format, remove, router, slug, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "plate",
    initialPageSize: 20,
    persistKey: FLEET_VEHICLES_PERSIST_KEY,
  });
  const params = listState.params;
  const list = useQuery({
    queryKey: fleetKeys.vehicles(fleet.uuid, params),
    queryFn: () => fleetsService.listVehicles(fleet.uuid, params),
  });

  const bulkActions = useMemo<DataTableBulkAction<FleetVehicle>[]>(
    () =>
      canPlan
        ? [
            {
              id: "create_plan",
              label: t("bulk.actions.fleet_vehicles.create_plan"),
              icon: CalendarPlus,
              onClick: (selected) =>
                router.push(
                  routes.tenant.fleets.newPlan(
                    slug,
                    fleet.uuid,
                    selected.map((v) => v.uuid),
                  ),
                ),
            },
          ]
        : [],
    [canPlan, fleet.uuid, router, slug, t],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("fleets.vehicles.empty_title")}
        emptyDescription={t("fleets.vehicles.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: FLEET_VEHICLES_PERSIST_KEY,
          rowSelection: true,
          viewMode: true,
        }}
        bulkActions={bulkActions}
        renderGridItem={(v) => (
          <div className="space-y-1">
            <div className="font-mono font-medium" dir="ltr">
              {v.plate ?? "—"}
            </div>
            <div className="text-muted-foreground text-xs">
              {vehicleLabel(v) || "—"}
            </div>
          </div>
        )}
        toolbarExtra={
          <>
            {canPlan ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                data-testid="fleet-plan-new"
                onClick={() =>
                  router.push(routes.tenant.fleets.newPlan(slug, fleet.uuid))
                }
              >
                <CalendarPlus className="size-4" />
                {t("fleets.plan.create")}
              </Button>
            ) : null}
            {canManage ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                data-testid="fleet-vehicles-import"
                onClick={() => setImportOpen(true)}
              >
                <Upload className="size-4" />
                {t("fleets.vehicles.import")}
              </Button>
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      {canManage ? (
        <ImportWizard
          resource={FLEET_VEHICLE_IMPORT_RESOURCE}
          paths={{
            upload: `/v1/fleets/${encodeURIComponent(fleet.uuid)}/vehicles/import`,
            sample: "/v1/fleets/vehicle-import/sample",
          }}
          fixedDefaults={{ fleet_uuid: fleet.uuid }}
          scope="tenant"
          jobsHref={routes.tenant.imports.root(slug)}
          open={importOpen}
          onOpenChange={setImportOpen}
          onComplete={() => {
            void qc.invalidateQueries({ queryKey: ["fleets", fleet.uuid] });
            void qc.invalidateQueries({ queryKey: fleetKeys.card(fleet.uuid) });
          }}
        />
      ) : null}
    </>
  );
}
