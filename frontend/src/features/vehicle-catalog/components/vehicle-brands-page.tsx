"use client";

import { useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CircleCheck, CircleOff, Eye, Pencil, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  useBulkSelection,
  type BulkActionDef,
} from "@/features/bulk-engine";
import { VehicleBrandDialog } from "@/features/vehicle-catalog/components/vehicle-brand-dialog";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import {
  useVehicleBrandMutations,
  useVehicleBrands,
  vehicleCatalogKeys,
} from "@/features/vehicle-catalog/hooks/use-vehicle-catalog";
import { logoVersion } from "@/features/vehicle-catalog/lib/images";
import type {
  VehicleBrand,
  VehicleListParams,
} from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";

export const VEHICLE_BRANDS_PERSIST_KEY = "platform-vehicle-brands-v2";

/** `POST /v1/platform/vehicle-catalog/brands/bulk` (TEC-369), undoable. */
export const VEHICLE_BRAND_BULK_ACTIONS: BulkActionDef[] = [
  {
    id: "activate",
    label_key: "bulk.actions.vehicle_brands.activate",
    permission: permissions.vehicleCatalog.write,
    reversible: true,
    icon: CircleCheck,
  },
  {
    id: "deactivate",
    label_key: "bulk.actions.vehicle_brands.deactivate",
    permission: permissions.vehicleCatalog.write,
    reversible: true,
    confirm_key: "bulk.confirm.vehicle_brands.deactivate",
    icon: CircleOff,
  },
];

/**
 * super_admin car brand list (TEC-150, port of otopoly-go vehicle-brands;
 * TEC-370 DataTable): sort, active / logo filters, bulk activate /
 * deactivate, row actions and a logo grid.
 */
export function VehicleBrandsPage() {
  const { t, format } = useLocale();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { confirmDelete } = useDialogs();
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<VehicleBrand | null>(null);
  const { update, remove } = useVehicleBrandMutations();
  const updateBrand = update.mutate;
  const removeBrand = remove.mutate;

  const baseColumns = useMemo<ColumnDef<VehicleBrand, unknown>[]>(
    () => [
      createColumn<VehicleBrand>({
        id: "logo",
        accessorKey: "has_logo",
        labelKey: "vehicles.columns.logo",
        enableSorting: false,
        filterVariant: "boolean",
        param: "has_logo",
        cell: ({ row }) => (
          <VehicleBrandLogo
            uuid={row.original.uuid}
            name={row.original.name}
            version={logoVersion(row.original.logo_url)}
            height={28}
          />
        ),
      }) as ColumnDef<VehicleBrand, unknown>,
      createColumn<VehicleBrand>({
        accessorKey: "name",
        labelKey: "vehicles.columns.name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }) as ColumnDef<VehicleBrand, unknown>,
      createColumn<VehicleBrand>({
        accessorKey: "external_id",
        labelKey: "vehicles.columns.external_id",
        enableSorting: false,
        cell: ({ row }) => row.original.external_id ?? "—",
      }) as ColumnDef<VehicleBrand, unknown>,
      createColumn<VehicleBrand>({
        accessorKey: "model_count",
        labelKey: "vehicles.columns.model_count",
        enableSorting: true,
      }) as ColumnDef<VehicleBrand, unknown>,
      createColumn<VehicleBrand>({
        accessorKey: "active",
        labelKey: "vehicles.columns.status",
        enableSorting: true,
        filterVariant: "boolean",
        param: "active",
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.active ? t("common.active") : t("common.passive")
            }
            tone={row.original.active ? "success" : "default"}
          />
        ),
      }) as ColumnDef<VehicleBrand, unknown>,
      createColumn<VehicleBrand>({
        accessorKey: "created_at",
        labelKey: "vehicles.columns.created_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => format.dateTime(row.original.created_at),
      }) as ColumnDef<VehicleBrand, unknown>,
      createColumn<VehicleBrand>({
        accessorKey: "updated_at",
        labelKey: "vehicles.columns.updated_at",
        enableSorting: true,
        cell: ({ row }) => format.dateTime(row.original.updated_at),
      }) as ColumnDef<VehicleBrand, unknown>,
      createColumn<VehicleBrand>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const brand = row.original;
          const actions: EntityRowAction[] = [
            {
              id: "view",
              label: t("common.view"),
              icon: Eye,
              onSelect: () =>
                router.push(routes.platform.vehicleCatalog.brand(brand.uuid)),
            },
            {
              id: "edit",
              label: t("common.edit"),
              icon: Pencil,
              onSelect: () => setEditing(brand),
            },
            {
              id: "toggle_active",
              label: brand.active
                ? t("bulk.actions.vehicle_brands.deactivate")
                : t("bulk.actions.vehicle_brands.activate"),
              icon: brand.active ? CircleOff : CircleCheck,
              onSelect: () =>
                updateBrand({
                  uuid: brand.uuid,
                  body: { active: !brand.active },
                }),
            },
            {
              id: "delete",
              label: t("common.delete"),
              icon: Trash2,
              variant: "destructive",
              onSelect: () => {
                void (async () => {
                  const ok = await confirmDelete({
                    title: t("vehicles.brand.delete_title"),
                    description: t("vehicles.brand.delete_confirm", {
                      name: brand.name,
                    }),
                  });
                  if (ok) removeBrand(brand.uuid);
                })();
              },
            },
          ];
          return <EntityRowActions actions={actions} />;
        },
      }) as ColumnDef<VehicleBrand, unknown>,
    ],
    [confirmDelete, format, removeBrand, router, t, updateBrand],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<VehicleBrand>(), ...baseColumns],
    [baseColumns],
  );

  const listState = useServerListState({
    columns,
    initialSort: "name",
    initialPageSize: 20,
    persistKey: VEHICLE_BRANDS_PERSIST_KEY,
  });
  const params: VehicleListParams = listState.params;
  const listQuery = useVehicleBrands(params);
  const total = listQuery.data?.total ?? 0;

  const bulkQuery = useMemo(
    () => ({ ...listState.filterParams, q: params.q, sort: params.sort }),
    [listState.filterParams, params.q, params.sort],
  );
  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery,
    total,
  });

  return (
    <EntityPage
      title={t("vehicles.title")}
      description={t("vehicles.description")}
      permission={permissions.vehicleCatalog.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("vehicles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("vehicles.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setCreateOpen(true)}
          label={t("vehicles.brand.create")}
          permission={permissions.vehicleCatalog.write}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.platform.vehicleCatalog.brand(row.uuid))
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("vehicles.error")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("vehicles.brand.empty_title")}
        emptyDescription={t("vehicles.brand.empty_description")}
        rowCount={total}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: VEHICLE_BRANDS_PERSIST_KEY,
          rowSelection: true,
          viewMode: true,
        }}
        renderGridItem={(brand) => (
          <div className="flex flex-col items-center gap-3 text-center">
            <div className="flex h-16 items-center justify-center">
              <VehicleBrandLogo
                uuid={brand.uuid}
                name={brand.name}
                version={logoVersion(brand.logo_url)}
                height={48}
              />
            </div>
            <div className="space-y-1">
              <p className="font-display font-semibold">{brand.name}</p>
              <p className="text-muted-foreground text-xs">
                {t("vehicles.brand.model_count", {
                  count: String(brand.model_count),
                })}
              </p>
            </div>
            <StatusChip
              label={brand.active ? t("common.active") : t("common.passive")}
              tone={brand.active ? "success" : "default"}
            />
          </div>
        )}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource="vehicle_catalog.brands"
              actions={VEHICLE_BRAND_BULK_ACTIONS}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                void queryClient.invalidateQueries({
                  queryKey: vehicleCatalogKeys.all,
                });
              }}
            />
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <VehicleBrandDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onSaved={(brand) =>
          router.push(routes.platform.vehicleCatalog.brand(brand.uuid))
        }
      />
      <VehicleBrandDialog
        open={Boolean(editing)}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
        brand={editing}
      />
    </EntityPage>
  );
}
