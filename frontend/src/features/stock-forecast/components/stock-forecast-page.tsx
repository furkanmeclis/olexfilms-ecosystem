"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, Row } from "@tanstack/react-table";
import {
  BarChart3,
  Eye,
  Factory,
  ShoppingCart,
  TrendingUp,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { AppChart } from "@/components/charts";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import {
  createColumn,
  createSelectColumnDef,
  type DataTableBulkAction,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ExportMenu } from "@/features/io/components/export-menu";
import {
  STOCK_FORECAST_STATUSES,
  forecastDraftLine,
  isSelectableForecast,
  missingDataDays,
  stockForecastStatusTone,
} from "@/features/stock-forecast/lib/stock-forecast";
import {
  STOCK_FORECAST_EXPORT_PATH,
  STOCK_FORECAST_NETWORK_EXPORT_PATH,
  stockForecastKeys,
  stockForecastService,
  type NetworkDemandRow,
  type StockForecastDraftLine,
  type StockForecastRow,
} from "@/features/stock-forecast/services/stock-forecast.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const STOCK_FORECAST_PAGE_SIZE = 20;
export const STOCK_FORECAST_PERSIST_KEY = "tenant-stock-forecast-v1";
export const STOCK_FORECAST_NETWORK_PERSIST_KEY =
  "tenant-stock-forecast-network-v1";

const categoryParam = "category_uuid";

function numberValue(value: unknown) {
  if (value == null || value === "") return 0;
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function thresholdFromValue(value: unknown, previous: number) {
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? Math.round(n) : previous;
}

function DraftDialog({
  open,
  rows,
  pending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  rows: StockForecastRow[];
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (lines: StockForecastDraftLine[]) => void;
}) {
  const { t } = useLocale();
  const [lines, setLines] = useState<StockForecastDraftLine[]>(() =>
    rows.map(forecastDraftLine),
  );

  function changed(next: boolean) {
    onOpenChange(next);
  }

  return (
    <Dialog open={open} onOpenChange={changed}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("stock_forecast.draft.title")}</DialogTitle>
          <DialogDescription>
            {t("stock_forecast.draft.description", { count: rows.length })}
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-80 space-y-3 overflow-auto">
          {rows.map((row, index) => {
            const line = lines[index] ?? forecastDraftLine(row);
            return (
              <div
                key={row.uuid}
                className="grid gap-2 rounded-md border p-3 sm:grid-cols-[1fr_8rem_8rem]"
              >
                <div className="min-w-0">
                  <div className="truncate font-medium">{row.product.name}</div>
                  <div className="text-muted-foreground font-mono text-xs">
                    {row.product.sku}
                  </div>
                </div>
                <Input
                  aria-label={t("stock_forecast.columns.suggested_qty")}
                  inputMode="numeric"
                  value={line.quantity ?? ""}
                  onChange={(event) => {
                    const value = event.target.value;
                    setLines((prev) =>
                      rows.map((source, i) => {
                        const item = prev[i] ?? forecastDraftLine(source);
                        return i === index
                          ? { ...item, quantity: value ? Number(value) : null }
                          : item;
                      }),
                    );
                  }}
                />
                <Input
                  aria-label={t("stock_forecast.columns.suggested_meters")}
                  inputMode="decimal"
                  value={line.meters ?? ""}
                  onChange={(event) => {
                    const value = event.target.value;
                    setLines((prev) =>
                      rows.map((source, i) => {
                        const item = prev[i] ?? forecastDraftLine(source);
                        return i === index
                          ? { ...item, meters: value ? Number(value) : null }
                          : item;
                      }),
                    );
                  }}
                />
              </div>
            );
          })}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={pending || lines.length === 0}
            onClick={() => onSubmit(lines)}
          >
            {t("stock_forecast.draft.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ProductDrawer({
  row,
  open,
  onOpenChange,
}: {
  row: StockForecastRow | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, format } = useLocale();
  const productUuid = row?.product.uuid ?? "";
  const query = useQuery({
    queryKey: stockForecastKeys.detail(productUuid),
    queryFn: () => stockForecastService.detail(productUuid),
    enabled: open && Boolean(productUuid),
  });
  const detail = query.data;
  const data = useMemo(
    () =>
      (detail?.history ?? []).map((point) => ({
        date: point.date,
        actual: point.actual_meters ?? point.actual_qty ?? null,
        projected: point.projected_meters ?? point.projected_qty ?? null,
        threshold: point.threshold ?? null,
        vehicles: point.vehicles_left ?? null,
      })),
    [detail?.history],
  );

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-auto sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>{row?.product.name ?? ""}</SheetTitle>
          <SheetDescription>
            {row
              ? t("stock_forecast.detail.subtitle", { sku: row.product.sku })
              : ""}
          </SheetDescription>
        </SheetHeader>
        <div className="mt-6 space-y-5">
          {query.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              onRetry={() => void query.refetch()}
            />
          ) : (
            <AppChart
              type="composed"
              data={data}
              categoryKey="date"
              config={{
                actual: { label: t("stock_forecast.detail.actual") },
                projected: { label: t("stock_forecast.detail.projected") },
                threshold: { label: t("stock_forecast.detail.threshold") },
                vehicles: { label: t("stock_forecast.detail.vehicles") },
              }}
              series={[
                { key: "actual", type: "line" },
                { key: "projected", type: "line", strokeDasharray: "5 5" },
                { key: "threshold", type: "line" },
                { key: "vehicles", type: "bar", yAxisId: "right" },
              ]}
              showRightYAxis
              loading={query.isLoading}
              emptyTitle={t("stock_forecast.detail.empty")}
              valueFormatter={(v) => format.number(v)}
              height={320}
            />
          )}
          {row ? (
            <dl className="grid gap-3 sm:grid-cols-3">
              <Stat
                label={t("stock_forecast.columns.days_left")}
                value={
                  row.days_left == null
                    ? "—"
                    : format.number(numberValue(row.days_left))
                }
              />
              <Stat
                label={t("stock_forecast.columns.vehicles_left")}
                value={
                  row.vehicles_left == null
                    ? "—"
                    : format.number(numberValue(row.vehicles_left))
                }
              />
              <Stat
                label={t("stock_forecast.columns.cover_days")}
                value={format.number(row.cover_days)}
              />
            </dl>
          ) : null}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border p-3">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="font-semibold tabular-nums">{value}</dd>
    </div>
  );
}

export function StockForecastPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const queryClient = useQueryClient();
  const canRead = can(Permission.StockForecastRead);
  const canManage = can(Permission.StockForecastManage);
  const canNetwork = can(Permission.StockForecastNetworkRead);
  const [draftRows, setDraftRows] = useState<StockForecastRow[]>([]);
  const [detailRow, setDetailRow] = useState<StockForecastRow | null>(null);
  const [createdDraft, setCreatedDraft] = useState<{
    uuid: string;
    order_no: string;
  } | null>(null);

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<StockForecastRow>(),
        createColumn<StockForecastRow>({
          id: "product",
          accessorFn: (row) => row.product.name,
          labelKey: "stock_forecast.columns.product",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <button
              type="button"
              className="text-start hover:underline"
              data-testid="stock-forecast-row"
              data-uuid={row.original.uuid}
              onClick={() => setDetailRow(row.original)}
            >
              <span className="font-medium">{row.original.product.name}</span>
              <span className="text-muted-foreground block font-mono text-xs">
                {row.original.product.sku}
              </span>
            </button>
          ),
        }),
        createColumn<StockForecastRow>({
          id: categoryParam,
          accessorFn: (row) => row.product.category?.uuid ?? "",
          labelKey: "stock_forecast.columns.category",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: [],
          param: categoryParam,
          cell: ({ row }) => row.original.product.category?.name ?? "—",
        }),
        createColumn<StockForecastRow>({
          accessorKey: "on_hand_qty",
          labelKey: "stock_forecast.columns.on_hand",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(row.original.on_hand_qty),
        }),
        createColumn<StockForecastRow>({
          accessorKey: "avg_daily_30",
          labelKey: "stock_forecast.columns.avg30",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.number(numberValue(row.original.avg_daily_30)),
        }),
        createColumn<StockForecastRow>({
          accessorKey: "avg_daily_90",
          labelKey: "stock_forecast.columns.avg90",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.number(numberValue(row.original.avg_daily_90)),
        }),
        createColumn<StockForecastRow>({
          accessorKey: "seasonality_factor",
          labelKey: "stock_forecast.columns.seasonality",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.number(numberValue(row.original.seasonality_factor)),
        }),
        createColumn<StockForecastRow>({
          accessorKey: "days_left",
          labelKey: "stock_forecast.columns.days_left",
          enableSorting: true,
          filterVariant: "number-range",
          param: "days_left",
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            row.original.days_left == null
              ? "—"
              : format.number(numberValue(row.original.days_left)),
        }),
        createColumn<StockForecastRow>({
          accessorKey: "depletion_date",
          labelKey: "stock_forecast.columns.depletion_date",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.depletion_date
              ? format.date(row.original.depletion_date)
              : "—",
        }),
        createColumn<StockForecastRow>({
          accessorKey: "status",
          labelKey: "stock_forecast.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: STOCK_FORECAST_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `stock_forecast.status.${value}`,
          })),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`stock_forecast.status.${row.original.status}`)}
              tone={stockForecastStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<StockForecastRow>({
          accessorKey: "suggested_qty",
          labelKey: "stock_forecast.columns.suggested_qty",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            row.original.suggested_qty == null
              ? "—"
              : format.number(row.original.suggested_qty),
        }),
        createColumn<StockForecastRow>({
          accessorKey: "cover_days",
          labelKey: "stock_forecast.columns.cover_days",
          enableSorting: false,
          editVariant: canManage ? "number" : undefined,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => (
            <span data-testid="stock-forecast-threshold">
              {format.number(row.original.cover_days)}
            </span>
          ),
        }),
        createColumn<StockForecastRow>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("common.view"),
                  icon: Eye,
                  onSelect: () => setDetailRow(row.original),
                },
              ]}
            />
          ),
        }),
      ] satisfies ColumnDef<StockForecastRow, unknown>[],
    [canManage, format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "days_left",
    initialPageSize: STOCK_FORECAST_PAGE_SIZE,
    persistKey: STOCK_FORECAST_PERSIST_KEY,
  });
  const list = useQuery({
    queryKey: stockForecastKeys.list(listState.params),
    queryFn: () => stockForecastService.list(listState.params),
    enabled: canRead,
  });
  const rows = list.data?.items ?? [];
  const insufficient = rows.find((row) => row.status === "insufficient_data");

  const draftMutation = useMutation({
    mutationFn: (lines: StockForecastDraftLine[]) =>
      stockForecastService.createOrderDraft(lines),
    onSuccess: (draft) => {
      setDraftRows([]);
      setCreatedDraft(draft);
      appToast.success(
        t("stock_forecast.draft.success", { no: draft.order_no }),
      );
    },
    onError: () => appToast.error(t("stock_forecast.draft.failed")),
  });

  const thresholdMutation = useMutation({
    mutationFn: ({
      row,
      cover_days,
    }: {
      row: StockForecastRow;
      cover_days: number;
    }) =>
      stockForecastService.updateThreshold(row.product.uuid, {
        warning_days: row.warning_days,
        cover_days,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: stockForecastKeys.all });
      appToast.success(t("stock_forecast.threshold.saved"));
    },
    onError: () => appToast.error(t("stock_forecast.threshold.failed")),
  });

  const bulkActions = useMemo<DataTableBulkAction<StockForecastRow>[]>(
    () => [
      {
        id: "create_draft",
        label: t("stock_forecast.bulk.create_draft"),
        icon: ShoppingCart,
        disabled: (selected) =>
          selected.every((row) => !isSelectableForecast(row)),
        onClick: (selected) =>
          setDraftRows(selected.filter(isSelectableForecast)),
      },
    ],
    [t],
  );

  const networkColumns = useMemo(
    () =>
      [
        createColumn<NetworkDemandRow>({
          id: "product",
          accessorFn: (row) => row.product.name,
          labelKey: "stock_forecast.columns.product",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <div>
              <div className="font-medium">{row.original.product.name}</div>
              <div className="text-muted-foreground font-mono text-xs">
                {row.original.product.sku}
              </div>
            </div>
          ),
        }),
        createColumn<NetworkDemandRow>({
          accessorKey: "forecast_month",
          labelKey: "stock_forecast.network.month",
          enableSorting: true,
          cell: ({ row }) => format.date(row.original.forecast_month),
        }),
        createColumn<NetworkDemandRow>({
          accessorKey: "network_on_hand_qty",
          labelKey: "stock_forecast.network.stock",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(row.original.network_on_hand_qty),
        }),
        createColumn<NetworkDemandRow>({
          accessorKey: "open_order_qty",
          labelKey: "stock_forecast.network.open_orders",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(row.original.open_order_qty),
        }),
        createColumn<NetworkDemandRow>({
          accessorKey: "suggested_production_qty",
          labelKey: "stock_forecast.network.suggested",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.number(row.original.suggested_production_qty),
        }),
      ] satisfies ColumnDef<NetworkDemandRow, unknown>[],
    [format],
  );
  const networkState = useServerListState({
    columns: networkColumns,
    initialSort: "forecast_month",
    initialPageSize: STOCK_FORECAST_PAGE_SIZE,
    persistKey: STOCK_FORECAST_NETWORK_PERSIST_KEY,
  });
  const network = useQuery({
    queryKey: stockForecastKeys.network(networkState.params),
    queryFn: () => stockForecastService.networkDemand(networkState.params),
    enabled: canNetwork && org?.type === "center",
  });
  const dealers = useQuery({
    queryKey: stockForecastKeys.dealers,
    queryFn: () => stockForecastService.dealerSummary(),
    enabled: canRead && org?.type === "distributor",
  });

  const title = t("stock_forecast.page.title");
  const header = (
    <PageHeader
      title={title}
      icon={<TrendingUp className="size-6" />}
      description={t("stock_forecast.page.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("stock_forecast.page.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      {insufficient ? (
        <div
          className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-100"
          data-testid="stock-forecast-insufficient-banner"
        >
          {t("stock_forecast.insufficient.banner", {
            min: insufficient.min_data_days,
            left: missingDataDays(insufficient),
          })}
        </div>
      ) : null}
      {createdDraft ? (
        <div className="rounded-md border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-950 dark:border-emerald-900 dark:bg-emerald-950/30 dark:text-emerald-100">
          {t("stock_forecast.draft.created")}{" "}
          <Link
            className="font-medium underline"
            href={routes.tenant.orders.detail(slug, createdDraft.uuid)}
            data-testid="stock-forecast-draft-link"
          >
            {createdDraft.order_no}
          </Link>
        </div>
      ) : null}
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        rowCount={list.data?.total ?? 0}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        state={listState.tableState}
        features={{
          persistKey: STOCK_FORECAST_PERSIST_KEY,
          rowSelection: ((row: Row<StockForecastRow>) =>
            isSelectableForecast(row.original)) as unknown as boolean,
          inlineEdit: canManage,
          columnOrdering: true,
          columnPinning: true,
        }}
        bulkActions={bulkActions}
        onCellEdit={({ row, value }) => {
          if (!canManage) return;
          thresholdMutation.mutate({
            row,
            cover_days: thresholdFromValue(value, row.cover_days),
          });
        }}
        toolbarExtra={
          <EntityToolbar>
            <ExportMenu
              exportPath={STOCK_FORECAST_EXPORT_PATH}
              query={{
                ...listState.filterParams,
                q: listState.params.q,
                sort: listState.params.sort,
              }}
              jobsHref={routes.tenant.exports.root(slug)}
              formats={["xlsx", "csv"]}
            />
          </EntityToolbar>
        }
        emptyTitle={t("stock_forecast.empty.title")}
        emptyDescription={t("stock_forecast.empty.description")}
      />

      {org?.type === "center" && canNetwork ? (
        <section className="space-y-3">
          <div className="flex items-center gap-2">
            <Factory className="text-muted-foreground size-5" />
            <h2 className="text-lg font-semibold">
              {t("stock_forecast.network.title")}
            </h2>
          </div>
          <EntityTable
            columns={networkColumns}
            data={network.data?.items ?? []}
            getRowId={(row) => row.uuid}
            rowCount={network.data?.total ?? 0}
            isLoading={network.isLoading}
            isError={network.isError}
            onRetry={() => void network.refetch()}
            state={networkState.tableState}
            features={{
              persistKey: STOCK_FORECAST_NETWORK_PERSIST_KEY,
              rowSelection: false,
              columnOrdering: true,
              columnPinning: true,
            }}
            toolbarExtra={
              <EntityToolbar>
                <ExportMenu
                  exportPath={STOCK_FORECAST_NETWORK_EXPORT_PATH}
                  query={{
                    ...networkState.filterParams,
                    q: networkState.params.q,
                    sort: networkState.params.sort,
                  }}
                  jobsHref={routes.tenant.exports.root(slug)}
                  formats={["xlsx", "csv"]}
                />
              </EntityToolbar>
            }
          />
        </section>
      ) : null}

      {org?.type === "distributor" ? (
        <section className="space-y-3">
          <div className="flex items-center gap-2">
            <BarChart3 className="text-muted-foreground size-5" />
            <h2 className="text-lg font-semibold">
              {t("stock_forecast.dealers.title")}
            </h2>
          </div>
          {dealers.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              onRetry={() => void dealers.refetch()}
            />
          ) : (
            <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
              {(dealers.data?.items ?? []).map((dealer) => (
                <div key={dealer.uuid} className="rounded-md border p-3">
                  <div className="font-medium">{dealer.dealer_name}</div>
                  <dl className="mt-3 grid grid-cols-3 gap-2 text-sm">
                    <Stat
                      label={t("stock_forecast.status.critical")}
                      value={format.number(dealer.critical_count)}
                    />
                    <Stat
                      label={t("stock_forecast.status.warning")}
                      value={format.number(dealer.warning_count)}
                    />
                    <Stat
                      label={t("stock_forecast.columns.suggested_qty")}
                      value={format.number(dealer.suggested_qty)}
                    />
                  </dl>
                </div>
              ))}
            </div>
          )}
        </section>
      ) : null}

      {draftRows.length > 0 ? (
        <DraftDialog
          open
          rows={draftRows}
          pending={draftMutation.isPending}
          onOpenChange={(open) => {
            if (!open) setDraftRows([]);
          }}
          onSubmit={(lines) => draftMutation.mutate(lines)}
        />
      ) : null}
      <ProductDrawer
        row={detailRow}
        open={Boolean(detailRow)}
        onOpenChange={(open) => {
          if (!open) setDetailRow(null);
        }}
      />
      <span className="sr-only">
        <Link href={routes.tenant.orders.list(slug)}>
          {t("stock_forecast.draft.orders")}
        </Link>
      </span>
    </div>
  );
}
