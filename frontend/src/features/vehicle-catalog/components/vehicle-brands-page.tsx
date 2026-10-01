"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityCreateButton,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { VehicleBrandDialog } from "@/features/vehicle-catalog/components/vehicle-brand-dialog";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { useVehicleBrands } from "@/features/vehicle-catalog/hooks/use-vehicle-catalog";
import { logoVersion } from "@/features/vehicle-catalog/lib/images";
import type {
  VehicleBrand,
  VehicleListParams,
} from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import { useLocale } from "@/providers/locale-provider";

type ActiveFilter = "all" | "true" | "false";

/** super_admin car brand list (TEC-150, port of otopoly-go vehicle-brands). */
export function VehicleBrandsPage() {
  const { t, format } = useLocale();
  const router = useRouter();
  const [active, setActive] = useState<ActiveFilter>("all");
  const [createOpen, setCreateOpen] = useState(false);

  const listState = useServerListState({
    initialSort: "name",
    initialPageSize: 20,
  });

  const params = useMemo<VehicleListParams>(
    () => ({
      limit: listState.params.limit,
      offset: listState.params.offset,
      q: listState.params.q,
      active: active === "all" ? undefined : active,
    }),
    [active, listState.params],
  );
  const listQuery = useVehicleBrands(params);

  const columns = useMemo<ColumnDef<VehicleBrand>[]>(
    () => [
      createColumn<VehicleBrand>({
        id: "logo",
        labelKey: "vehicles.columns.logo",
        enableSorting: false,
        cell: ({ row }) => (
          <VehicleBrandLogo
            uuid={row.original.uuid}
            name={row.original.name}
            version={logoVersion(row.original.logo_url)}
            height={28}
          />
        ),
      }),
      createColumn<VehicleBrand>({
        accessorKey: "name",
        labelKey: "vehicles.columns.name",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }),
      createColumn<VehicleBrand>({
        accessorKey: "external_id",
        labelKey: "vehicles.columns.external_id",
        enableSorting: false,
        cell: ({ row }) => row.original.external_id ?? "—",
      }),
      createColumn<VehicleBrand>({
        accessorKey: "model_count",
        labelKey: "vehicles.columns.model_count",
        enableSorting: false,
      }),
      createColumn<VehicleBrand>({
        accessorKey: "active",
        labelKey: "vehicles.columns.status",
        enableSorting: false,
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.active ? t("common.active") : t("common.passive")
            }
            tone={row.original.active ? "success" : "default"}
          />
        ),
      }),
      createColumn<VehicleBrand>({
        accessorKey: "updated_at",
        labelKey: "vehicles.columns.updated_at",
        enableSorting: false,
        cell: ({ row }) => format.dateTime(row.original.updated_at),
      }),
    ],
    [format, t],
  );

  const pageCount = Math.max(
    1,
    Math.ceil((listQuery.data?.total ?? 0) / (params.limit || 20)),
  );

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
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "platform-vehicle-brands-v1",
          sorting: false,
          columnFilters: false,
          facetedFilters: false,
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <Select
              value={active}
              onValueChange={(value) => {
                setActive(value as ActiveFilter);
                listState.setPagination((prev) => ({ ...prev, pageIndex: 0 }));
              }}
            >
              <SelectTrigger
                size="sm"
                className="w-36"
                aria-label={t("vehicles.filters.status")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("vehicles.filters.all")}</SelectItem>
                <SelectItem value="true">{t("common.active")}</SelectItem>
                <SelectItem value="false">{t("common.passive")}</SelectItem>
              </SelectContent>
            </Select>
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
    </EntityPage>
  );
}
