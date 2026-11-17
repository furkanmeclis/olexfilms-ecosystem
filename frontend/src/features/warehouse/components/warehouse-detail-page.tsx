"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { MoveRight, Warehouse as WarehouseIcon } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityLink,
  EntityTable,
  EntityToolbar,
  refColumn,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { Info } from "@/features/warehouse/components/list-controls";
import { WarehouseProductsTab } from "@/features/warehouse/components/stock-view";
import { enumFilterOptions } from "@/features/warehouse/components/table-options";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  ENTRY_MODES,
  ENTRY_STATUSES,
  entryStatusTone,
} from "@/features/warehouse/lib/entries";
import {
  TRANSFER_STATUSES,
  transferStatusTone,
} from "@/features/warehouse/lib/transfers";
import {
  warehouseKeys,
  warehouseService,
  type StockEntry,
  type WarehouseTraceUnit,
  type WarehouseTransfer,
} from "@/features/warehouse/services/warehouse.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

export const WAREHOUSE_DETAIL_PAGE_SIZE = 20;
export const WAREHOUSE_DETAIL_UNITS_KEY = "tenant-warehouse-detail-units-v1";
export const WAREHOUSE_DETAIL_ENTRIES_KEY =
  "tenant-warehouse-detail-entries-v1";
export const WAREHOUSE_DETAIL_TRANSFERS_KEY = {
  out: "tenant-warehouse-detail-transfers-out-v1",
  in: "tenant-warehouse-detail-transfers-in-v1",
} as const;

const UNIT_SOURCES = ["order", "stock", "unknown"] as const;
const sourceTone = {
  order: "success",
  stock: "default",
  unknown: "warning",
} as const;

/**
 * Units that originate from the warehouse (TEC-1006 list, `stock.read`):
 * product and barcode link to their records. No price columns (warehouse
 * roles must not see money).
 */
function UnitsTab({ slug, uuid }: { slug: string; uuid: string }) {
  const { t, format } = useLocale();
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
            <span className="font-mono" dir="ltr">
              <EntityLink
                kind="stock_unit"
                slug={slug}
                entity={{
                  barcode: row.original.barcode,
                  name: row.original.barcode,
                }}
              />
            </span>
          ),
        }),
        refColumn<WarehouseTraceUnit>({
          id: "product",
          kind: "product",
          labelKey: "warehouse.units.columns.product",
          enableSorting: true,
          getRef: (u) => ({
            uuid: u.product.uuid,
            name: u.product.name,
            sub: u.product.sku,
          }),
        }),
        createColumn<WarehouseTraceUnit>({
          id: "state",
          accessorFn: (u) => (u.in_warehouse ? "in" : "left"),
          labelKey: "warehouse.units.columns.state",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(
            ["in", "left"],
            "warehouse.units.state",
          ),
          param: "state",
          cell: ({ row }) => {
            const u = row.original;
            return (
              <span>
                {t(`warehouse.units.state.${u.in_warehouse ? "in" : "left"}`)}
                {u.in_warehouse && u.location_code ? (
                  <span className="ms-1 font-mono text-xs" dir="ltr">
                    {u.location_code}
                  </span>
                ) : null}
                {!u.in_warehouse && u.holder ? (
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
          filterOptions: enumFilterOptions(UNIT_SOURCES, "stock.trace.source"),
          param: "source",
          cell: ({ row }) => (
            <StatusChip
              label={t(`stock.trace.source.${row.original.source}`)}
              tone={sourceTone[row.original.source]}
            />
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
    initialPageSize: WAREHOUSE_DETAIL_PAGE_SIZE,
    persistKey: WAREHOUSE_DETAIL_UNITS_KEY,
  });
  const list = useQuery({
    queryKey: warehouseKeys.units(uuid, listState.params),
    queryFn: () => warehouseService.listWarehouseUnits(uuid, listState.params),
  });
  return (
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
      features={{ persistKey: WAREHOUSE_DETAIL_UNITS_KEY }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void list.refetch()}
          refreshDisabled={list.isFetching}
        />
      }
    />
  );
}

/** Stock entries of the warehouse (fixed `warehouse_uuid` filter). */
function EntriesTab({ slug, uuid }: { slug: string; uuid: string }) {
  const { t, format } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<StockEntry>({
          accessorKey: "created_at",
          labelKey: "warehouse.entries.columns.created",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <div className="min-w-0">
              <Link
                href={routes.tenant.warehouse.entry(slug, row.original.uuid)}
                className="text-primary whitespace-nowrap hover:underline"
                data-testid="warehouse-detail-entry"
              >
                {format.dateTime(row.original.created_at)}
              </Link>
              {row.original.note ? (
                <div className="text-muted-foreground truncate text-xs">
                  {row.original.note}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<StockEntry>({
          accessorKey: "status",
          labelKey: "warehouse.entries.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(
            ENTRY_STATUSES,
            "warehouse.entry_status",
          ),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`warehouse.entry_status.${row.original.status}`)}
              tone={entryStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<StockEntry>({
          accessorKey: "mode",
          labelKey: "warehouse.entries.columns.mode",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(ENTRY_MODES, "warehouse.entry_mode"),
          param: "mode",
          cell: ({ row }) => t(`warehouse.entry_mode.${row.original.mode}`),
        }),
        createColumn<StockEntry>({
          accessorKey: "line_count",
          labelKey: "warehouse.entries.columns.lines",
          enableSorting: false,
          cell: ({ row }) => format.number(row.original.line_count),
        }),
      ] as ColumnDef<StockEntry, unknown>[],
    [format, slug, t],
  );
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: WAREHOUSE_DETAIL_PAGE_SIZE,
    persistKey: WAREHOUSE_DETAIL_ENTRIES_KEY,
  });
  const query = { ...listState.params, warehouse_uuid: uuid };
  const list = useQuery({
    queryKey: warehouseKeys.entries(query),
    queryFn: () => warehouseService.listEntries(query),
  });
  return (
    <EntityTable
      columns={columns}
      data={list.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      emptyTitle={t("warehouse.entries.empty_title")}
      emptyDescription=""
      rowCount={list.data?.total ?? 0}
      state={listState.tableState}
      features={{ persistKey: WAREHOUSE_DETAIL_ENTRIES_KEY }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void list.refetch()}
          refreshDisabled={list.isFetching}
        />
      }
    />
  );
}

/**
 * Transfers leaving (`out`, from_warehouse_uuid) or reaching (`in`,
 * to_warehouse_uuid) the warehouse; the other side links to its warehouse.
 */
function TransfersTab({
  slug,
  uuid,
  direction,
}: {
  slug: string;
  uuid: string;
  direction: "out" | "in";
}) {
  const { t, format } = useLocale();
  const persistKey = WAREHOUSE_DETAIL_TRANSFERS_KEY[direction];
  const columns = useMemo(
    () =>
      [
        createColumn<WarehouseTransfer>({
          accessorKey: "transfer_no",
          labelKey: "warehouse.transfers.columns.no",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.warehouse.transfer(slug, row.original.uuid)}
              className="text-primary font-mono hover:underline"
              dir="ltr"
              data-testid="warehouse-detail-transfer"
            >
              {row.original.transfer_no}
            </Link>
          ),
        }),
        createColumn<WarehouseTransfer>({
          id: direction === "out" ? "to_warehouse" : "from_warehouse",
          accessorFn: (r) =>
            (direction === "out" ? r.to_warehouse : r.from_warehouse).code,
          labelKey:
            direction === "out"
              ? "warehouse.fields.to_warehouse"
              : "warehouse.fields.from_warehouse",
          enableSorting: false,
          cell: ({ row }) => {
            const other =
              direction === "out"
                ? row.original.to_warehouse
                : row.original.from_warehouse;
            return (
              <span className="inline-flex items-center gap-1">
                {direction === "out" ? (
                  <MoveRight className="text-muted-foreground size-3 rtl:rotate-180" />
                ) : null}
                <EntityLink kind="warehouse" slug={slug} entity={other}>
                  {other.code} · {other.name}
                </EntityLink>
              </span>
            );
          },
        }),
        createColumn<WarehouseTransfer>({
          accessorKey: "status",
          labelKey: "warehouse.entries.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(
            TRANSFER_STATUSES,
            "warehouse.transfer_status",
          ),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`warehouse.transfer_status.${row.original.status}`)}
              tone={transferStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<WarehouseTransfer>({
          accessorKey: "line_count",
          labelKey: "warehouse.entries.columns.lines",
          enableSorting: false,
          cell: ({ row }) => format.number(row.original.line_count),
        }),
        createColumn<WarehouseTransfer>({
          accessorKey: "created_at",
          labelKey: "warehouse.entries.columns.created",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
      ] as ColumnDef<WarehouseTransfer, unknown>[],
    [direction, format, slug, t],
  );
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: WAREHOUSE_DETAIL_PAGE_SIZE,
    persistKey,
  });
  const query = {
    ...listState.params,
    [direction === "out" ? "from_warehouse_uuid" : "to_warehouse_uuid"]: uuid,
  };
  const list = useQuery({
    queryKey: warehouseKeys.transfers(query),
    queryFn: () => warehouseService.listTransfers(query),
  });
  return (
    <EntityTable
      columns={columns}
      data={list.data?.items ?? []}
      getRowId={(row) => row.uuid}
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      emptyTitle={t("warehouse.transfers.empty_title")}
      emptyDescription=""
      rowCount={list.data?.total ?? 0}
      state={listState.tableState}
      features={{ persistKey }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void list.refetch()}
          refreshDisabled={list.isFetching}
        />
      }
    />
  );
}

/**
 * Warehouse > warehouse detail (TEC-1218): the warehouse card (code, name,
 * address, active, primary, placement rule, rooms with a link to the
 * locations tree), its units (`stock.read`), stock entries and transfers in
 * and out. Center and distributor with `warehouse.read` (K12). No money.
 */
export function WarehouseDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const canReadUnits = access.can(Permission.StockRead);

  const detail = useQuery({
    queryKey: warehouseKeys.warehouse(uuid),
    queryFn: () => warehouseService.getWarehouse(uuid),
    enabled: access.allowed && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });
  const rooms = useQuery({
    queryKey: warehouseKeys.rooms(uuid),
    queryFn: () => warehouseService.listRooms(uuid),
    enabled: access.allowed && uuid !== "",
  });
  const w = detail.data;
  const yesNo = (v: boolean | undefined) =>
    v ? t("common.yes") : t("common.no");

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={w ? `${w.code} · ${w.name}` : t("warehouse.detail.title")}
      icon={<WarehouseIcon className="size-6" />}
      crumbs={[
        {
          label: t("warehouse.locations.warehouses"),
          href: routes.tenant.warehouse.locations(slug),
        },
      ]}
    >
      {detail.isError ? (
        <ErrorState
          title={
            isApiError(detail.error) && detail.error.status === 404
              ? t("warehouse.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={() => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !w ? (
        <p className="text-muted-foreground text-sm">
          {t("warehouse.list.loading")}
        </p>
      ) : (
        <div className="space-y-6" data-testid="warehouse-detail">
          <Card>
            <CardContent className="grid gap-4 pt-6 sm:grid-cols-2 lg:grid-cols-4">
              <Info label={t("warehouse.fields.code")}>
                <span className="font-mono" dir="ltr">
                  {w.code}
                </span>
              </Info>
              <Info label={t("warehouse.fields.name")}>{w.name}</Info>
              <Info label={t("warehouse.fields.address")}>
                {w.address || "—"}
              </Info>
              <Info label={t("warehouse.fields.active")}>
                <StatusChip
                  label={yesNo(w.active)}
                  tone={w.active ? "success" : "default"}
                />
              </Info>
              <Info label={t("warehouse.detail.primary")}>
                <span data-testid="warehouse-detail-primary">
                  {yesNo(w.is_primary)}
                </span>
              </Info>
              <Info label={t("warehouse.form.require_location_placement")}>
                {yesNo(w.require_location_placement)}
              </Info>
              <Info label={t("warehouse.locations.rooms")}>
                <span className="inline-flex flex-wrap items-baseline gap-2">
                  <span data-testid="warehouse-detail-rooms">
                    {rooms.data ? format.number(rooms.data.items.length) : "—"}
                  </span>
                  <Link
                    href={routes.tenant.warehouse.locations(slug)}
                    className="text-primary text-xs font-normal hover:underline"
                  >
                    {t("warehouse.detail.open_locations")}
                  </Link>
                </span>
              </Info>
              <Info label={t("warehouse.detail.updated_at")}>
                {format.dateTime(w.updated_at)}
              </Info>
            </CardContent>
          </Card>

          <Tabs defaultValue={canReadUnits ? "units" : "entries"}>
            <TabsList className="flex-wrap">
              {canReadUnits ? (
                <TabsTrigger
                  value="products"
                  data-testid="warehouse-detail-products-tab"
                >
                  {t("warehouse.stock.products_tab")}
                </TabsTrigger>
              ) : null}
              {canReadUnits ? (
                <TabsTrigger value="units">
                  {t("warehouse.units.title")}
                </TabsTrigger>
              ) : null}
              <TabsTrigger value="entries">
                {t("warehouse.entries.title")}
              </TabsTrigger>
              <TabsTrigger value="transfers-out">
                {t("warehouse.detail.transfers_out")}
              </TabsTrigger>
              <TabsTrigger value="transfers-in">
                {t("warehouse.detail.transfers_in")}
              </TabsTrigger>
            </TabsList>
            {canReadUnits ? (
              <TabsContent value="products">
                <WarehouseProductsTab uuid={w.uuid} />
              </TabsContent>
            ) : null}
            {canReadUnits ? (
              <TabsContent value="units">
                <UnitsTab slug={slug} uuid={w.uuid} />
              </TabsContent>
            ) : null}
            <TabsContent value="entries">
              <EntriesTab slug={slug} uuid={w.uuid} />
            </TabsContent>
            <TabsContent value="transfers-out">
              <TransfersTab slug={slug} uuid={w.uuid} direction="out" />
            </TabsContent>
            <TabsContent value="transfers-in">
              <TransfersTab slug={slug} uuid={w.uuid} direction="in" />
            </TabsContent>
          </Tabs>
        </div>
      )}
    </WarehouseShell>
  );
}
