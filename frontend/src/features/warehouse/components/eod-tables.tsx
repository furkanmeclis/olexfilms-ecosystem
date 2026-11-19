"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { StockMeasureLine } from "@/features/stock/components/stock-measure-line";
import { hasMetered } from "@/features/stock/lib/measures";
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

export const EOD_BALANCES_PERSIST_KEY = "tenant-warehouse-eod-balances-v1";

type EodBalanceRow = EodReport["summary"]["balances"][number];

/**
 * Opening and closing on-hand stock per product of an end-of-day report
 * (TEC-1229): pieces and roll meters apart, a client-side DataTable with
 * sort and search. No prices.
 */
export function EodBalancesTable({ balances }: { balances: EodBalanceRow[] }) {
  const { t, format } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<EodBalanceRow>({
          id: "product",
          accessorFn: (p) => `${p.product_name} ${p.sku}`,
          labelKey: "warehouse.fields.product",
          gridPrimary: true,
          cell: ({ row }) => (
            <div data-testid="eod-balance-row">
              {row.original.product_name}
              <div
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.sku}
              </div>
              {/* TEC-1243: the K14 split when the report was written. */}
              {row.original.measures && hasMetered(row.original.measures) ? (
                <StockMeasureLine
                  measures={{
                    ...row.original.measures,
                    boxes:
                      row.original.closing_boxes ?? row.original.measures.boxes,
                  }}
                  className="mt-0.5"
                />
              ) : null}
            </div>
          ),
        }),
        createColumn<EodBalanceRow>({
          id: "boxes",
          accessorFn: (p) => p.closing_boxes ?? -1,
          labelKey: "warehouse.eod.boxes",
          cell: ({ row }) =>
            row.original.closing_boxes == null
              ? "—"
              : `${format.number(row.original.opening_boxes ?? 0)} → ${format.number(row.original.closing_boxes)}`,
        }),
        createColumn<EodBalanceRow>({
          id: "rolls",
          accessorFn: (p) => p.closing_rolls ?? -1,
          labelKey: "warehouse.eod.rolls",
          cell: ({ row }) =>
            row.original.closing_rolls == null
              ? "—"
              : `${format.number(row.original.opening_rolls ?? 0)} → ${format.number(row.original.closing_rolls)}`,
        }),
        createColumn<EodBalanceRow>({
          accessorKey: "opening_quantity",
          labelKey: "warehouse.eod.opening_quantity",
          cell: ({ row }) => format.number(row.original.opening_quantity),
        }),
        createColumn<EodBalanceRow>({
          accessorKey: "closing_quantity",
          labelKey: "warehouse.eod.closing_quantity",
          cell: ({ row }) => format.number(row.original.closing_quantity),
        }),
        createColumn<EodBalanceRow>({
          accessorKey: "change_quantity",
          labelKey: "warehouse.eod.change_quantity",
          cell: ({ row }) =>
            format.number(row.original.change_quantity, {
              signDisplay: "exceptZero",
            }),
        }),
        createColumn<EodBalanceRow>({
          id: "opening_meters",
          accessorFn: (p) => Number(p.opening_meters),
          labelKey: "warehouse.eod.opening_meters",
          cell: ({ row }) => (
            <span dir="ltr">{row.original.opening_meters}</span>
          ),
        }),
        createColumn<EodBalanceRow>({
          id: "closing_meters",
          accessorFn: (p) => Number(p.closing_meters),
          labelKey: "warehouse.eod.closing_meters",
          cell: ({ row }) => (
            <span dir="ltr">{row.original.closing_meters}</span>
          ),
        }),
      ] as ColumnDef<EodBalanceRow, unknown>[],
    [format],
  );
  return (
    <EntityTable
      columns={columns}
      data={balances}
      getRowId={(row) => row.product_uuid}
      manual={CLIENT_SIDE_MANUAL}
      emptyTitle={t("warehouse.eod.no_balances")}
      emptyDescription=""
      initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
      features={{
        persistKey: EOD_BALANCES_PERSIST_KEY,
        rowSelection: false,
        viewMode: true,
      }}
    />
  );
}
