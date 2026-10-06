"use client";

import type { ColumnDef, FilterFn } from "@tanstack/react-table";
import { ArrowRightLeft, Eye, Pencil } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
  clientDateRangeFilter,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { routes } from "@/config/routes";
import type { Vehicle } from "@/features/customers/services/customers.service";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { useLocale } from "@/providers/locale-provider";

export const CUSTOMER_VEHICLES_PERSIST_KEY = "tenant-customer-vehicles-v1";

const dash = (v: string | number | null | undefined) =>
  v === null || v === undefined || v === "" ? "—" : v;

/**
 * Vehicles of one customer (TEC-163, TEC-372) as a nested client-side
 * DataTable: the API returns the few vehicles of the customer in one page,
 * so sort, search, the brand facet and paging run in the browser. Row
 * actions: open, edit (vehicles.write on an editable account) and the
 * ownership transfer (vehicles.transfer, on the vehicle page).
 */
export function CustomerVehiclesTable({
  slug,
  vehicles,
  isLoading,
  canWrite,
  canTransfer,
  onEdit,
}: {
  slug: string;
  vehicles: Vehicle[];
  isLoading?: boolean;
  canWrite: boolean;
  canTransfer: boolean;
  onEdit: (vehicle: Vehicle) => void;
}) {
  const { t, format } = useLocale();
  const router = useRouter();

  const brandOptions = useMemo(() => {
    const seen = new Map<string, string>();
    for (const v of vehicles) {
      if (v.car_brand) seen.set(v.car_brand.name, v.car_brand.name);
    }
    return [...seen.keys()]
      .sort((a, b) => a.localeCompare(b))
      .map((name) => ({ value: name, label: name }));
  }, [vehicles]);

  const columns = useMemo(
    () =>
      [
        createColumn<Vehicle>({
          id: "plate",
          accessorFn: (row) => row.plate_normalized ?? row.plate ?? "",
          labelKey: "customers.vehicle.plate",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const v = row.original;
            return (
              <div className="flex items-center gap-2">
                <VehicleBrandLogo
                  uuid={v.car_brand?.uuid}
                  name={v.car_brand?.name}
                  height={24}
                />
                <Link
                  href={routes.tenant.vehicles.detail(slug, v.uuid)}
                  className="font-mono font-medium whitespace-nowrap hover:underline"
                  dir="ltr"
                  data-testid="vehicle-row"
                  onClick={(event) => event.stopPropagation()}
                >
                  {dash(v.plate)}
                  {v.plate_country ? ` (${v.plate_country})` : ""}
                </Link>
              </div>
            );
          },
        }),
        createColumn<Vehicle>({
          id: "brand",
          accessorFn: (row) => row.car_brand?.name ?? "",
          labelKey: "customers.vehicle.brand",
          enableSorting: true,
          sortUndefined: "last",
          filterVariant: "faceted",
          filterOptions: brandOptions,
          enableColumnFilter: brandOptions.length > 1,
          cell: ({ row }) => dash(row.original.car_brand?.name),
        }),
        createColumn<Vehicle>({
          id: "model",
          accessorFn: (row) => row.car_model?.name ?? "",
          labelKey: "customers.vehicle.model",
          enableSorting: true,
          cell: ({ row }) => dash(row.original.car_model?.name),
        }),
        createColumn<Vehicle>({
          accessorKey: "model_year",
          labelKey: "customers.vehicle.year",
          enableSorting: true,
          sortUndefined: "last",
          cell: ({ row }) => dash(row.original.model_year),
        }),
        createColumn<Vehicle>({
          accessorKey: "vin",
          labelKey: "vehicles.detail.vin",
          enableSorting: true,
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
          defaultHidden: true,
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<Vehicle>,
          cell: ({ row }) => format.date(row.original.created_at),
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
            if (canWrite) {
              items.push({
                id: "edit-vehicle",
                label: t("customers.vehicle.edit"),
                icon: Pencil,
                onSelect: () => onEdit(v),
              });
            }
            if (canTransfer) {
              items.push({
                id: "transfer",
                label: t("vehicles.transfer.title"),
                icon: ArrowRightLeft,
                onSelect: () =>
                  router.push(routes.tenant.vehicles.detail(slug, v.uuid)),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<Vehicle, unknown>[],
    [brandOptions, canTransfer, canWrite, format, onEdit, router, slug, t],
  );

  return (
    <div data-testid="vehicle-list">
      <EntityTable
        columns={columns}
        data={vehicles}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={isLoading}
        onRowClick={(v) =>
          router.push(routes.tenant.vehicles.detail(slug, v.uuid))
        }
        initialState={{
          pagination: { pageIndex: 0, pageSize: 10 },
          sorting: [{ id: "created_at", desc: true }],
        }}
        emptyTitle={t("customers.detail.no_vehicles")}
        emptyDescription=""
        features={{ persistKey: CUSTOMER_VEHICLES_PERSIST_KEY }}
      />
    </div>
  );
}
