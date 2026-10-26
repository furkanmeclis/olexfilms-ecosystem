"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Boxes } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  warehouseKeys,
  warehouseService,
  type WarehouseTraceUnit,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export const WAREHOUSE_UNITS_PAGE_SIZE = 20;
export const WAREHOUSE_UNITS_PERSIST_KEY = "tenant-warehouse-units-v1";

const SOURCES = ["order", "stock", "unknown"] as const;
const sourceTone = {
  order: "success",
  stock: "default",
  unknown: "warning",
} as const;

/**
 * Warehouse > Units (TEC-1006): every unit that originates from the picked
 * warehouse — still in it or gone to a dealer, a service or the trash —
 * with the order that carried it, the service that used it and the
 * customer. Legacy hub units without a location originate from the primary
 * warehouse (AFM Florya Merkez). Departed units are not warehouse stock;
 * "source unknown" marks units at another organization without an order.
 */
export function WarehouseUnitsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug, Permission.StockRead);
  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
    enabled: access.allowed,
  });
  const items = warehouses.data?.items ?? [];
  const [picked, setPicked] = useState<string | null>(null);
  const warehouseUuid =
    picked ?? items.find((w) => w.is_primary)?.uuid ?? items[0]?.uuid ?? "";

  const columns = useMemo(
    () =>
      [
        createColumn<WarehouseTraceUnit>({
          accessorKey: "barcode",
          labelKey: "warehouse.units.columns.barcode",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.stock.unit(slug, row.original.barcode)}
              className="font-mono hover:underline"
              dir="ltr"
              data-testid="warehouse-unit-row"
            >
              {row.original.barcode}
            </Link>
          ),
        }),
        createColumn<WarehouseTraceUnit>({
          id: "product",
          accessorFn: (u) => u.product.name,
          labelKey: "warehouse.units.columns.product",
          enableSorting: true,
          cell: ({ row }) => row.original.product.name,
        }),
        createColumn<WarehouseTraceUnit>({
          id: "state",
          accessorFn: (u) => (u.in_warehouse ? "in" : "left"),
          labelKey: "warehouse.units.columns.state",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ["in", "left"].map((value) => ({
            value,
            label: value,
            labelKey: `warehouse.units.state.${value}`,
          })),
          param: "state",
          cell: ({ row }) => {
            const u = row.original;
            if (u.in_warehouse) {
              return (
                <span>
                  {t("warehouse.units.state.in")}
                  {u.location_code ? (
                    <span className="ms-1 font-mono text-xs" dir="ltr">
                      {u.location_code}
                    </span>
                  ) : null}
                </span>
              );
            }
            return (
              <span>
                {t("warehouse.units.state.left")}
                {u.holder ? (
                  <span className="text-muted-foreground ms-1 text-xs">
                    {u.holder.name}
                  </span>
                ) : null}
              </span>
            );
          },
        }),
        createColumn<WarehouseTraceUnit>({
          accessorKey: "source",
          labelKey: "warehouse.units.columns.source",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: SOURCES.map((value) => ({
            value,
            label: value,
            labelKey: `stock.trace.source.${value}`,
          })),
          param: "source",
          cell: ({ row }) => (
            <StatusChip
              label={t(`stock.trace.source.${row.original.source}`)}
              tone={sourceTone[row.original.source]}
            />
          ),
        }),
        createColumn<WarehouseTraceUnit>({
          id: "order",
          accessorFn: (u) => u.order?.order_no ?? "",
          labelKey: "warehouse.units.columns.order",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.order ? (
              <Link
                href={routes.tenant.orders.detail(
                  slug,
                  row.original.order.uuid,
                )}
                className="hover:underline"
              >
                <span className="font-mono" dir="ltr">
                  {row.original.order.order_no}
                </span>
                <span className="text-muted-foreground ms-1 text-xs">
                  {row.original.order.buyer_name}
                </span>
              </Link>
            ) : (
              "—"
            ),
        }),
        createColumn<WarehouseTraceUnit>({
          id: "service",
          accessorFn: (u) => u.service?.service_no ?? "",
          labelKey: "warehouse.units.columns.service",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.service ? (
              <Link
                href={routes.tenant.services.detail(
                  slug,
                  row.original.service.uuid,
                )}
                className="hover:underline"
              >
                <span className="font-mono" dir="ltr">
                  {row.original.service.service_no}
                </span>
                <span className="text-muted-foreground ms-1 text-xs">
                  {row.original.service.customer_name}
                </span>
              </Link>
            ) : (
              "—"
            ),
        }),
        createColumn<WarehouseTraceUnit>({
          accessorKey: "updated_at",
          labelKey: "warehouse.units.columns.updated_at",
          enableSorting: true,
          cell: ({ row }) => format.dateTime(row.original.updated_at),
        }),
        createColumn<WarehouseTraceUnit>({
          accessorKey: "created_at",
          labelKey: "warehouse.units.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
      ] as ColumnDef<WarehouseTraceUnit, unknown>[],
    [format, slug, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-updated_at",
    initialPageSize: WAREHOUSE_UNITS_PAGE_SIZE,
    persistKey: WAREHOUSE_UNITS_PERSIST_KEY,
  });
  const list = useQuery({
    queryKey: warehouseKeys.units(warehouseUuid, listState.params),
    queryFn: () =>
      warehouseService.listWarehouseUnits(warehouseUuid, listState.params),
    enabled: access.allowed && warehouseUuid !== "",
  });

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.units.title")}
      description={t("warehouse.units.description")}
      icon={<Boxes className="size-6" />}
      forbiddenKey="warehouse.units.forbidden"
    >
      <Card>
        <CardContent className="space-y-4 pt-6">
          <Select value={warehouseUuid} onValueChange={setPicked}>
            <SelectTrigger
              className="w-72"
              aria-label={t("warehouse.units.warehouse")}
              data-testid="warehouse-units-warehouse"
            >
              <SelectValue placeholder={t("warehouse.units.warehouse")} />
            </SelectTrigger>
            <SelectContent>
              {items.map((w) => (
                <SelectItem key={w.uuid} value={w.uuid}>
                  {w.code} · {w.name}
                  {w.is_primary ? ` (${t("warehouse.units.primary")})` : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <EntityTable
            columns={columns}
            data={list.data?.items ?? []}
            getRowId={(row) => row.uuid}
            isLoading={list.isLoading}
            isError={list.isError}
            onRetry={() => void list.refetch()}
            emptyTitle={t("warehouse.units.empty")}
            emptyDescription=""
            rowCount={list.data?.total ?? 0}
            state={listState.tableState}
            features={{
              persistKey: WAREHOUSE_UNITS_PERSIST_KEY,
              viewMode: true,
            }}
            renderGridItem={(u) => (
              <div className="space-y-1 text-sm">
                <div className="flex items-start justify-between gap-2">
                  <span className="font-mono" dir="ltr">
                    {u.barcode}
                  </span>
                  <StatusChip
                    label={t(`stock.trace.source.${u.source}`)}
                    tone={sourceTone[u.source]}
                  />
                </div>
                <p>{u.product.name}</p>
                <p className="text-muted-foreground text-xs">
                  {u.in_warehouse
                    ? t("warehouse.units.state.in")
                    : `${t("warehouse.units.state.left")} · ${u.holder?.name ?? "—"}`}
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
        </CardContent>
      </Card>
    </WarehouseShell>
  );
}
