"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { routes } from "@/config/routes";
import { unitStatusTone } from "@/features/stock/lib/stock";
import {
  stockKeys,
  stockService,
  type StockProduct,
  type StockProductListQuery,
  type StockUnitRow,
} from "@/features/stock/services/stock.service";
import type { WarehouseLocation } from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export const LOCATION_PRODUCTS_PERSIST_KEY = "warehouse-location-products-v1";
export const LOCATION_UNITS_PERSIST_KEY = "warehouse-location-units-v1";

/**
 * Stock of one bin (TEC-764): a side sheet next to the location tree with
 * the bin's products (bin_product_stocks: quantity, meters) and its units,
 * each linked to the unit history (TEC-763). Needs stock.read.
 */
export function LocationStockSheet({
  slug,
  orgUuid,
  location,
  onClose,
}: {
  slug: string;
  orgUuid: string;
  location: WarehouseLocation | null;
  onClose: () => void;
}) {
  const { t, format } = useLocale();
  const uuid = location?.uuid ?? "";

  const productColumns = useMemo(
    () =>
      [
        createColumn<StockProduct>({
          id: "product",
          accessorFn: (r) => r.product.name,
          labelKey: "stock.columns.product",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div data-testid="location-stock-product">
              <span className="font-medium">{row.original.product.name}</span>
              <div
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.product.sku}
              </div>
            </div>
          ),
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
      ] as ColumnDef<StockProduct, unknown>[],
    [format, t],
  );
  const productState = useServerListState({
    columns: productColumns,
    initialSort: "product",
    initialPageSize: 10,
    persistKey: LOCATION_PRODUCTS_PERSIST_KEY,
  });
  const productParams: StockProductListQuery = productState.params;
  const products = useQuery({
    queryKey: stockKeys.locationProducts(uuid, productParams),
    queryFn: () => stockService.listLocationProducts(uuid, productParams),
    enabled: uuid !== "",
  });

  const unitColumns = useMemo(
    () =>
      [
        createColumn<StockUnitRow>({
          accessorKey: "barcode",
          labelKey: "stock.columns.unit",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.stock.unit(slug, row.original.barcode)}
              className="font-mono text-xs font-medium underline-offset-4 hover:underline"
              dir="ltr"
              data-testid="location-stock-unit"
            >
              {row.original.barcode}
            </Link>
          ),
        }),
        createColumn<StockUnitRow>({
          id: "product",
          accessorFn: (r) => r.product.name,
          labelKey: "stock.columns.product",
          enableSorting: true,
          cell: ({ row }) => row.original.product.name,
        }),
        createColumn<StockUnitRow>({
          accessorKey: "status",
          labelKey: "stock.columns.status",
          enableSorting: true,
          cell: ({ row }) => (
            <StatusChip
              label={t(`stock.status.${row.original.status}`)}
              tone={unitStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<StockUnitRow>({
          id: "meters",
          accessorFn: (r) => Number(r.remaining_meters ?? 0),
          labelKey: "stock.columns.meters",
          enableSorting: true,
          meta: { cellClassName: "text-end whitespace-nowrap tabular-nums" },
          cell: ({ row }) =>
            row.original.remaining_meters
              ? t("stock.widget.meters_value", {
                  meters: row.original.remaining_meters,
                })
              : "—",
        }),
      ] as ColumnDef<StockUnitRow, unknown>[],
    [slug, t],
  );
  const unitState = useServerListState({
    columns: unitColumns,
    initialSort: "barcode",
    initialPageSize: 10,
    persistKey: LOCATION_UNITS_PERSIST_KEY,
  });
  const unitParams = useMemo(
    () => ({ ...unitState.params, location_uuid: uuid }),
    [unitState.params, uuid],
  );
  const units = useQuery({
    queryKey: stockKeys.units(orgUuid, unitParams),
    queryFn: () => stockService.listUnits(orgUuid, unitParams),
    enabled: uuid !== "" && orgUuid !== "",
  });

  return (
    <Sheet open={location !== null} onOpenChange={(o) => !o && onClose()}>
      <SheetContent
        className="w-full overflow-y-auto sm:max-w-xl"
        data-testid="location-stock-sheet"
      >
        <SheetHeader>
          <SheetTitle>{t("warehouse.stock_panel.title")}</SheetTitle>
          <SheetDescription className="font-mono" dir="ltr">
            {location?.full_code}
          </SheetDescription>
        </SheetHeader>
        <div className="space-y-4 px-4 pb-4">
          <dl className="grid grid-cols-2 gap-3">
            <div className="rounded-md border p-3">
              <dt className="text-muted-foreground text-xs">
                {t("warehouse.stock_panel.products")}
              </dt>
              <dd
                className="text-xl font-semibold tabular-nums"
                data-testid="location-stock-product-count"
              >
                {products.data ? format.number(products.data.total) : "—"}
              </dd>
            </div>
            <div className="rounded-md border p-3">
              <dt className="text-muted-foreground text-xs">
                {t("warehouse.stock_panel.units")}
              </dt>
              <dd
                className="text-xl font-semibold tabular-nums"
                data-testid="location-stock-unit-count"
              >
                {units.data ? format.number(units.data.total) : "—"}
              </dd>
            </div>
          </dl>
          <Tabs defaultValue="products">
            <TabsList>
              <TabsTrigger
                value="products"
                data-testid="location-stock-tab-products"
              >
                {t("warehouse.stock_panel.products")}
              </TabsTrigger>
              <TabsTrigger value="units" data-testid="location-stock-tab-units">
                {t("warehouse.stock_panel.units")}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="products">
              <EntityTable
                columns={productColumns}
                data={products.data?.items ?? []}
                getRowId={(r) => r.product.uuid}
                isLoading={products.isLoading}
                isError={products.isError}
                onRetry={() => void products.refetch()}
                emptyTitle={t("warehouse.stock_panel.empty_title")}
                emptyDescription={t("warehouse.stock_panel.empty_description")}
                rowCount={products.data?.total ?? 0}
                state={productState.tableState}
                features={{
                  persistKey: LOCATION_PRODUCTS_PERSIST_KEY,
                  rowSelection: false,
                }}
                toolbarExtra={
                  <EntityToolbar
                    onRefresh={() => void products.refetch()}
                    refreshDisabled={products.isFetching}
                  />
                }
              />
            </TabsContent>
            <TabsContent value="units">
              <EntityTable
                columns={unitColumns}
                data={units.data?.items ?? []}
                getRowId={(r) => r.uuid}
                isLoading={units.isLoading}
                isError={units.isError}
                onRetry={() => void units.refetch()}
                emptyTitle={t("warehouse.stock_panel.empty_title")}
                emptyDescription={t("warehouse.stock_panel.empty_description")}
                rowCount={units.data?.total ?? 0}
                state={unitState.tableState}
                features={{
                  persistKey: LOCATION_UNITS_PERSIST_KEY,
                  rowSelection: false,
                }}
                toolbarExtra={
                  <EntityToolbar
                    onRefresh={() => void units.refetch()}
                    refreshDisabled={units.isFetching}
                  />
                }
              />
            </TabsContent>
          </Tabs>
        </div>
      </SheetContent>
    </Sheet>
  );
}
