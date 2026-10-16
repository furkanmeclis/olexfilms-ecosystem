"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { ListTree } from "lucide-react";
import { useMemo } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  stockKeys,
  stockService,
  type StockProduct,
  type StockProductListQuery,
} from "@/features/stock/services/stock.service";
import { useLocale } from "@/providers/locale-provider";

export const STOCK_PRODUCTS_PERSIST_KEY = "tenant-stock-products-v1";

const PRODUCT_STATUSES = ["in_stock", "out_of_stock"] as const;

/**
 * Product stock of an organization (TEC-763 "by product" tab): the
 * product projection over a server DataTable — sort (product, SKU,
 * category, quantity, meters, updated), search, an on-hand filter and a
 * row action that opens the product's units.
 */
export function StockProductsTable({
  orgUuid,
  enabled,
  onOpenUnits,
}: {
  orgUuid: string;
  enabled: boolean;
  onOpenUnits: (product: StockProduct["product"]) => void;
}) {
  const { t, format } = useLocale();

  const columns = useMemo(
    () =>
      [
        createColumn<StockProduct>({
          id: "product",
          accessorFn: (r) => r.product.name,
          labelKey: "stock.columns.product",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium" data-testid="stock-product-row">
              {row.original.product.name}
            </span>
          ),
        }),
        createColumn<StockProduct>({
          id: "sku",
          accessorFn: (r) => r.product.sku,
          labelKey: "stock.products.sku",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.product.sku}
            </span>
          ),
        }),
        createColumn<StockProduct>({
          id: "category",
          accessorFn: (r) => r.product.category.name,
          labelKey: "stock.products.category",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => row.original.product.category.name,
        }),
        createColumn<StockProduct>({
          id: "status",
          accessorFn: (r) => (r.quantity > 0 || Number(r.meters) > 0 ? 1 : 0),
          labelKey: "stock.products.on_hand",
          enableSorting: false,
          enableHiding: false,
          defaultHidden: true,
          filterVariant: "select",
          filterOptions: PRODUCT_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `stock.products.status.${value}`,
          })),
          param: "status",
          cell: () => null,
        }),
        createColumn<StockProduct>({
          accessorKey: "quantity",
          labelKey: "stock.columns.quantity",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(row.original.quantity),
        }),
        createColumn<StockProduct>({
          id: "meters",
          accessorFn: (r) => Number(r.meters),
          labelKey: "stock.columns.meters",
          enableSorting: true,
          meta: { cellClassName: "text-end whitespace-nowrap tabular-nums" },
          cell: ({ row }) =>
            row.original.product.unit_type === "roll_meter"
              ? t("stock.widget.meters_value", { meters: row.original.meters })
              : "—",
        }),
        createColumn<StockProduct>({
          accessorKey: "updated_at",
          labelKey: "stock.columns.updated",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {format.dateTime(row.original.updated_at)}
            </span>
          ),
        }),
        createColumn<StockProduct>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "units",
                  label: t("stock.products.open_units"),
                  icon: ListTree,
                  onSelect: () => onOpenUnits(row.original.product),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<StockProduct, unknown>[],
    [format, onOpenUnits, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "product",
    initialPageSize: 20,
    persistKey: STOCK_PRODUCTS_PERSIST_KEY,
  });
  const params: StockProductListQuery = listState.params;
  const list = useQuery({
    queryKey: stockKeys.products(orgUuid, params),
    queryFn: () => stockService.listProducts(orgUuid, params),
    enabled: enabled && orgUuid !== "",
  });

  return (
    <EntityTable
      columns={columns}
      data={list.data?.items ?? []}
      getRowId={(row) => row.product.uuid}
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      emptyTitle={t("stock.products.empty_title")}
      emptyDescription={t("stock.products.empty_description")}
      rowCount={list.data?.total ?? 0}
      state={listState.tableState}
      features={{
        persistKey: STOCK_PRODUCTS_PERSIST_KEY,
        rowSelection: false,
        viewMode: true,
      }}
      renderGridItem={(r) => (
        <div className="space-y-1">
          <p className="font-medium">{r.product.name}</p>
          <p className="text-muted-foreground font-mono text-xs">
            {r.product.sku}
          </p>
          <p className="text-sm">
            {r.product.unit_type === "roll_meter"
              ? t("stock.widget.meters_value", { meters: r.meters })
              : t("stock.widget.quantity_value", { quantity: r.quantity })}
          </p>
        </div>
      )}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void list.refetch()}
          refreshDisabled={list.isFetching}
        />
      }
    />
  );
}
