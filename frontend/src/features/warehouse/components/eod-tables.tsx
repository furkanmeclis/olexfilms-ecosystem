"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { enumFilterOptions } from "@/features/warehouse/components/table-options";
import { EOD_GROUPS, netQuantity } from "@/features/warehouse/lib/eod";
import type { EodReport } from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export const EOD_GROUPS_PERSIST_KEY = "tenant-warehouse-eod-groups-v1";
export const EOD_PRODUCTS_PERSIST_KEY = "tenant-warehouse-eod-products-v1";

type EodGroup = EodReport["summary"]["groups"][number];
type EodProduct = EodReport["summary"]["products"][number];

/**
 * Movement groups of an end-of-day report (TEC-207, TEC-376): a small
 * client-side DataTable (sort).
 */
export function EodGroupsTable({ groups }: { groups: EodGroup[] }) {
  const { t, format } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<EodGroup>({
          accessorKey: "group",
          labelKey: "warehouse.eod.group",
          gridPrimary: true,
          cell: ({ row }) => t(`warehouse.eod_group.${row.original.group}`),
        }),
        createColumn<EodGroup>({
          accessorKey: "movement_count",
          labelKey: "warehouse.eod.columns.movements",
          cell: ({ row }) => format.number(row.original.movement_count),
        }),
        createColumn<EodGroup>({
          id: "net",
          accessorFn: netQuantity,
          labelKey: "warehouse.eod.net",
          cell: ({ row }) =>
            format.number(netQuantity(row.original), {
              signDisplay: "exceptZero",
            }),
        }),
      ] as ColumnDef<EodGroup, unknown>[],
    [format, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={groups}
      getRowId={(row) => row.group}
      manual={CLIENT_SIDE_MANUAL}
      emptyTitle={t("warehouse.eod.no_movements")}
      emptyDescription=""
      initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
      features={{
        persistKey: EOD_GROUPS_PERSIST_KEY,
        rowSelection: false,
        globalFilter: false,
        columnFilters: false,
      }}
    />
  );
}

/**
 * Product lines of an end-of-day report (TEC-207, TEC-376): a client-side
 * DataTable with sort, search (product, SKU) and a group filter.
 */
export function EodProductsTable({ products }: { products: EodProduct[] }) {
  const { t, format } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<EodProduct>({
          id: "product",
          accessorFn: (p) => `${p.product_name} ${p.sku}`,
          labelKey: "warehouse.fields.product",
          gridPrimary: true,
          cell: ({ row }) => (
            <div>
              {row.original.product_name}
              <div
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.sku}
              </div>
            </div>
          ),
        }),
        createColumn<EodProduct>({
          accessorKey: "group",
          labelKey: "warehouse.eod.group",
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(EOD_GROUPS, "warehouse.eod_group"),
          cell: ({ row }) => t(`warehouse.eod_group.${row.original.group}`),
        }),
        createColumn<EodProduct>({
          accessorKey: "movement_count",
          labelKey: "warehouse.eod.columns.movements",
          cell: ({ row }) => format.number(row.original.movement_count),
        }),
        createColumn<EodProduct>({
          id: "net",
          accessorFn: netQuantity,
          labelKey: "warehouse.eod.net",
          cell: ({ row }) =>
            format.number(netQuantity(row.original), {
              signDisplay: "exceptZero",
            }),
        }),
      ] as ColumnDef<EodProduct, unknown>[],
    [format, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={products}
      getRowId={(row) => `${row.product_uuid}-${row.type}`}
      manual={CLIENT_SIDE_MANUAL}
      emptyTitle={t("warehouse.eod.no_movements")}
      emptyDescription=""
      initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
      features={{
        persistKey: EOD_PRODUCTS_PERSIST_KEY,
        rowSelection: false,
        viewMode: true,
      }}
    />
  );
}
