"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil, UserRound } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { VehicleFormDialog } from "@/features/customers/components/vehicle-form-dialog";
import {
  customerKeys,
  customersService,
} from "@/features/customers/services/customers.service";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { vehicleCatalogService } from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import {
  vehicleListKeys,
  vehiclesService,
  type Vehicle,
  type VehicleListQuery,
} from "@/features/vehicles/services/vehicles.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const VEHICLES_PERSIST_KEY = "tenant-vehicles-v1";

const dash = (v: string | number | null | undefined) =>
  v === null || v === undefined || v === "" ? "—" : v;

function selectedValues(
  filters: { id: string; value: unknown }[],
  id: string,
): string[] {
  const value = filters.find((f) => f.id === id)?.value;
  return Array.isArray(value) ? value.map(String) : [];
}

/**
 * Tenant > Vehicles (TEC-372): every vehicle of the customers in scope
 * (GET /v1/vehicles, TEC-371) as a server DataTable with sort, `q` (plate,
 * VIN, brand / model name), car brand and model facets (models once one
 * brand is chosen) and an organization filter for center / distributor.
 * A row opens the vehicle page; writers edit the vehicle in place.
 */
export function VehiclesListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const qc = useQueryClient();
  const org = useActiveOrganization(slug);
  const canRead = can(permissions.vehicles.read);
  const canWrite = canRead && can(permissions.vehicles.write);
  const canOpenCustomer = can(permissions.customers.read);
  const [editing, setEditing] = useState<Vehicle | null>(null);
  const [brandFilter, setBrandFilter] = useState<string[]>([]);

  const brands = useQuery({
    queryKey: ["vehicles", "list", "brand-options"],
    queryFn: () =>
      vehicleCatalogService.listBrands({
        limit: 100,
        offset: 0,
        sort: "name",
        active: "true",
      }),
    enabled: canRead,
    staleTime: 5 * 60_000,
  });
  const brandOptions = useMemo(
    () =>
      (brands.data?.items ?? []).map((b) => ({ value: b.uuid, label: b.name })),
    [brands.data?.items],
  );
  // Models are offered once exactly one brand is chosen.
  const modelBrand = brandFilter.length === 1 ? brandFilter[0] : undefined;
  const models = useQuery({
    queryKey: ["vehicles", "list", "model-options", modelBrand],
    queryFn: () =>
      vehicleCatalogService.listModels({
        brand_uuid: modelBrand ?? "",
        limit: 100,
        offset: 0,
        sort: "name",
        active: "true",
      }),
    enabled: canRead && Boolean(modelBrand),
    staleTime: 5 * 60_000,
  });
  const modelOptions = useMemo(
    () =>
      modelBrand
        ? (models.data?.items ?? []).map((m) => ({
            value: m.uuid,
            label: m.name,
          }))
        : [],
    [modelBrand, models.data?.items],
  );

  const canFilterOrg =
    canRead &&
    can(permissions.organizations.tenantRead) &&
    (org?.type === "center" || org?.type === "distributor");
  const organizations = useQuery({
    queryKey: customerKeys.organizations,
    queryFn: () => customersService.listOrganizations(),
    enabled: canFilterOrg,
    staleTime: 5 * 60_000,
  });
  const orgOptions = useMemo(
    () =>
      (organizations.data ?? []).map((o) => ({ value: o.uuid, label: o.name })),
    [organizations.data],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<Vehicle>({
          id: "plate",
          accessorFn: (row) => row.plate ?? "",
          labelKey: "vehicles.detail.plate",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.vehicles.detail(slug, row.original.uuid)}
              className="font-mono font-medium whitespace-nowrap hover:underline"
              dir="ltr"
              data-testid="vehicle-row"
              onClick={(event) => event.stopPropagation()}
            >
              {dash(row.original.plate)}
              {row.original.plate_country
                ? ` (${row.original.plate_country})`
                : ""}
            </Link>
          ),
        }),
        createColumn<Vehicle>({
          id: "brand",
          accessorFn: (row) => row.car_brand?.uuid ?? "",
          labelKey: "customers.vehicle.brand",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: brandOptions,
          enableColumnFilter: brandOptions.length > 0,
          param: "car_brand_uuid",
          cell: ({ row }) =>
            row.original.car_brand ? (
              <div className="flex items-center gap-2">
                <VehicleBrandLogo
                  uuid={row.original.car_brand.uuid}
                  name={row.original.car_brand.name}
                  height={20}
                />
                <span>{row.original.car_brand.name}</span>
              </div>
            ) : (
              "—"
            ),
        }),
        createColumn<Vehicle>({
          id: "model",
          accessorFn: (row) => row.car_model?.uuid ?? "",
          labelKey: "customers.vehicle.model",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: modelOptions,
          enableColumnFilter: modelOptions.length > 0,
          param: "car_model_uuid",
          cell: ({ row }) => dash(row.original.car_model?.name),
        }),
        createColumn<Vehicle>({
          accessorKey: "model_year",
          labelKey: "customers.vehicle.year",
          enableSorting: true,
          cell: ({ row }) => dash(row.original.model_year),
        }),
        createColumn<Vehicle>({
          accessorKey: "vin",
          labelKey: "vehicles.detail.vin",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.vin ? (
              <span className="font-mono text-xs" dir="ltr">
                {row.original.vin}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<Vehicle>({
          accessorKey: "created_at",
          labelKey: "vehicles.columns.created_at",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.created_at)}
            </span>
          ),
        }),
        // Filter only: matches the owner's organization links (the scope),
        // not the registering organization of the row.
        createColumn<Vehicle>({
          id: "organization",
          accessorFn: () => "",
          labelKey: "customers.fields.organization",
          enableSorting: false,
          enableHiding: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: orgOptions,
          enableColumnFilter: canFilterOrg && orgOptions.length > 0,
          param: "organization_uuid",
          cell: () => null,
        }),
        createColumn<Vehicle>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const v = row.original;
            const items: EntityRowAction[] = [
              {
                id: "open",
                label: t("common.open"),
                icon: Eye,
                onSelect: () =>
                  router.push(routes.tenant.vehicles.detail(slug, v.uuid)),
              },
            ];
            if (canOpenCustomer) {
              items.push({
                id: "customer",
                label: t("vehicles.list.open_customer"),
                icon: UserRound,
                onSelect: () =>
                  router.push(
                    routes.tenant.customers.detail(slug, v.customer_uuid),
                  ),
              });
            }
            if (canWrite) {
              items.push({
                id: "edit",
                label: t("customers.vehicle.edit"),
                icon: Pencil,
                onSelect: () => setEditing(v),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<Vehicle, unknown>[],
    [
      brandOptions,
      canFilterOrg,
      canOpenCustomer,
      canWrite,
      format,
      modelOptions,
      orgOptions,
      router,
      slug,
      t,
    ],
  );

  // Column meta drives the params: brand / model / organization (CSV).
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    persistKey: VEHICLES_PERSIST_KEY,
  });
  const brands_ = selectedValues(listState.columnFilters, "brand");
  if (brands_.join(",") !== brandFilter.join(",")) {
    setBrandFilter(brands_);
    // A model filter only makes sense for its one brand.
    if (
      brands_.length !== 1 &&
      selectedValues(listState.columnFilters, "model").length > 0
    ) {
      listState.onColumnFiltersChange((prev) =>
        prev.filter((f) => f.id !== "model"),
      );
    }
  }
  const params: VehicleListQuery = listState.params;

  const list = useQuery({
    queryKey: vehicleListKeys.list(params),
    queryFn: () => vehiclesService.list(params),
    enabled: canRead,
  });
  const total = list.data?.total ?? 0;

  const title = t("vehicles.list.title");
  return (
    <EntityPage
      title={title}
      description={t("vehicles.list.description")}
      permission={permissions.vehicles.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("vehicles.list.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
    >
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.vehicles.detail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("vehicles.list.empty_title")}
        emptyDescription={t("vehicles.list.empty_description")}
        rowCount={total}
        state={listState.tableState}
        features={{ persistKey: VEHICLES_PERSIST_KEY }}
        renderGridItem={(v) => (
          <div className="flex items-center gap-3">
            <VehicleBrandLogo
              uuid={v.car_brand?.uuid}
              name={v.car_brand?.name}
              height={28}
            />
            <div className="min-w-0">
              <Link
                href={routes.tenant.vehicles.detail(slug, v.uuid)}
                className="font-mono font-medium hover:underline"
                dir="ltr"
                onClick={(event) => event.stopPropagation()}
              >
                {dash(v.plate)}
              </Link>
              <p className="text-muted-foreground text-xs">
                {dash(
                  [v.car_brand?.name, v.car_model?.name, v.model_year]
                    .filter(Boolean)
                    .join(" "),
                )}
              </p>
            </div>
          </div>
        )}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />

      {editing ? (
        <VehicleFormDialog
          open
          customerUuid={editing.customer_uuid}
          vehicle={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: vehicleListKeys.all });
          }}
        />
      ) : null}
    </EntityPage>
  );
}
