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
  cleanDraftLine,
  effectiveThreshold,
  forecastDraftLine,
  isSelectableForecast,
  missingDataDays,
  productThresholdInput,
  stockForecastStatusTone,
} from "@/features/stock-forecast/lib/stock-forecast";
import {
  STOCK_FORECAST_EXPORT_PATH,
  STOCK_FORECAST_NETWORK_EXPORT_PATH,
  stockForecastKeys,
  stockForecastService,
  type NetworkDemandRow,
  type StockForecastDraftLine,
  type StockForecastOrderDraft,
  type StockForecastRow,
  type StockForecastThresholds,
  type SubtreeSummaryRow,
} from "@/features/stock-forecast/services/stock-forecast.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const STOCK_FORECAST_PAGE_SIZE = 20;
export const STOCK_FORECAST_PERSIST_KEY = "tenant-stock-forecast-v1";
export const STOCK_FORECAST_NETWORK_PERSIST_KEY =
  "tenant-stock-forecast-network-v1";
export const STOCK_FORECAST_SUBTREE_PERSIST_KEY =
  "tenant-stock-forecast-subtree-v1";

/** Fallback until /meta answers (backend default of forecast.min_days). */
const DEFAULT_MIN_DATA_DAYS = 90;

function isRoll(row: StockForecastRow) {
  return row.product.unit_type === "roll_meter";
}

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

  function update(index: number, patch: Partial<StockForecastDraftLine>) {
    setLines((prev) =>
      rows.map((source, i) => {
        const item = prev[i] ?? forecastDraftLine(source);
        return i === index ? { ...item, ...patch } : item;
      }),
    );
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
                {isRoll(row) ? (
                  <Input
                    aria-label={t("stock_forecast.columns.suggested_meters")}
                    inputMode="decimal"
                    className="sm:col-span-2"
                    value={line.meters ?? ""}
                    onChange={(event) => {
                      const value = event.target.value;
                      update(index, { meters: value });
                    }}
                  />
                ) : (
                  <Input
                    aria-label={t("stock_forecast.columns.suggested_qty")}
                    inputMode="numeric"
                    className="sm:col-span-2"
                    value={line.quantity ?? ""}
                    onChange={(event) => {
                      const value = event.target.value;
                      update(index, {
                        quantity: value ? Number(value) : undefined,
                      });
                    }}
                  />
                )}
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
            onClick={() =>
              onSubmit(
                rows.map((row, i) =>
                  cleanDraftLine(lines[i] ?? forecastDraftLine(row)),
                ),
              )
            }
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
  const data = useMemo(() => {
    if (!detail) return [];
    const roll = detail.forecast.product.unit_type === "roll_meter";
    return [
      ...detail.consumption.map((point) => ({
        date: point.date,
        consumption: roll ? numberValue(point.meters) : point.qty,
        projected: null as number | null,
      })),
      ...detail.projection.map((point) => ({
        date: point.date,
        consumption: null as number | null,
        projected: roll ? numberValue(point.stock_meters) : point.stock_qty,
      })),
    ];
  }, [detail]);

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
                consumption: { label: t("stock_forecast.detail.consumption") },
                projected: { label: t("stock_forecast.detail.projected") },
              }}
              series={[
                { key: "consumption", type: "bar", yAxisId: "right" },
                { key: "projected", type: "line", strokeDasharray: "5 5" },
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
                value={
                  detail ? format.number(detail.thresholds.cover_days) : "—"
                }
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
  const [createdDraft, setCreatedDraft] = useState<
    StockForecastOrderDraft["order"] | null
  >(null);
  const meta = useQuery({
    queryKey: stockForecastKeys.meta,
    queryFn: () => stockForecastService.meta(),
    enabled: canRead,
    staleTime: 60_000,
  });
  const thresholds: StockForecastThresholds | undefined = meta.data?.thresholds;
  const minDataDays =
    meta.data?.parameters.min_data_days ?? DEFAULT_MIN_DATA_DAYS;
  const canEditThreshold = canManage && Boolean(thresholds);

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<StockForecastRow>(),
        createColumn<StockForecastRow>({
          id: "product",
          accessorFn: (row) => row.product.name,
          labelKey: "stock_forecast.columns.product",
          enableSorting: true,
          sortParam: "product_name",
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
          id: "category",
          accessorFn: (row) => row.product.category_name ?? "",
          labelKey: "stock_forecast.columns.category",
          enableSorting: false,
          cell: ({ row }) => row.original.product.category_name || "—",
        }),
        createColumn<StockForecastRow>({
          accessorKey: "on_hand_qty",
          labelKey: "stock_forecast.columns.on_hand",
          enableSorting: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.number(
              isRoll(row.original)
                ? numberValue(row.original.on_hand_meters)
                : row.original.on_hand_qty,
            ),
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
          enableSorting: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.number(numberValue(row.original.avg_daily_90)),
        }),
        createColumn<StockForecastRow>({
          accessorKey: "seasonality_factor",
          labelKey: "stock_forecast.columns.seasonality",
          enableSorting: false,
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
          enableSorting: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => {
            const value = isRoll(row.original)
              ? row.original.suggested_meters
              : row.original.suggested_qty;
            return value == null ? "—" : format.number(numberValue(value));
          },
        }),
        createColumn<StockForecastRow>({
          id: "cover_days",
          accessorFn: (row) =>
            effectiveThreshold(thresholds, row.product.uuid)?.cover_days ??
            null,
          labelKey: "stock_forecast.columns.cover_days",
          enableSorting: false,
          editVariant: canEditThreshold ? "number" : undefined,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => {
            const value = effectiveThreshold(
              thresholds,
              row.original.product.uuid,
            )?.cover_days;
            return (
              <span data-testid="stock-forecast-threshold">
                {value == null ? "—" : format.number(value)}
              </span>
            );
          },
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
    [canEditThreshold, format, t, thresholds],
  );

  const listState = useServerListState({
    columns,
    initialSort: meta.data?.default_sort ?? "days_left",
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
    mutationFn: (items: StockForecastDraftLine[]) =>
      stockForecastService.createOrderDraft({ items }),
    onSuccess: (draft) => {
      setDraftRows([]);
      setCreatedDraft(draft.order);
      appToast.success(
        t("stock_forecast.draft.success", { no: draft.order.order_no }),
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
      stockForecastService.putThresholds(
        productThresholdInput(thresholds!, row.product.uuid, cover_days),
      ),
    onSuccess: (saved) => {
      queryClient.setQueryData(stockForecastKeys.meta, {
        ...meta.data!,
        thresholds: saved,
      });
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
          enableSorting: false,
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
          enableSorting: false,
          cell: ({ row }) => format.date(row.original.forecast_month),
        }),
        createColumn<NetworkDemandRow>({
          accessorKey: "network_on_hand_qty",
          labelKey: "stock_forecast.network.stock",
          enableSorting: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(row.original.network_on_hand_qty),
        }),
        createColumn<NetworkDemandRow>({
          accessorKey: "open_order_qty",
          labelKey: "stock_forecast.network.open_orders",
          enableSorting: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => format.number(row.original.open_order_qty),
        }),
        createColumn<NetworkDemandRow>({
          accessorKey: "suggested_production_qty",
          labelKey: "stock_forecast.network.suggested",
          enableSorting: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.number(row.original.suggested_production_qty),
        }),
      ] satisfies ColumnDef<NetworkDemandRow, unknown>[],
    [format],
  );
  const networkState = useServerListState({
    columns: networkColumns,
    initialSort: null,
    initialPageSize: STOCK_FORECAST_PAGE_SIZE,
    persistKey: STOCK_FORECAST_NETWORK_PERSIST_KEY,
  });
  const network = useQuery({
    queryKey: stockForecastKeys.network(networkState.params),
    queryFn: () => stockForecastService.network(networkState.params),
    enabled: canNetwork && org?.type === "center",
  });

  const subtreeColumns = useMemo(
    () =>
      [
        createColumn<SubtreeSummaryRow>({
          accessorKey: "name",
          labelKey: "stock_forecast.dealers.organization",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        ...(
          [
            ["critical_count", "stock_forecast.status.critical"],
            ["warning_count", "stock_forecast.status.warning"],
            [
              "insufficient_data_count",
              "stock_forecast.status.insufficient_data",
            ],
            ["total_count", "stock_forecast.dealers.total"],
          ] as const
        ).map(([key, labelKey]) =>
          createColumn<SubtreeSummaryRow>({
            accessorKey: key,
            labelKey,
            enableSorting: false,
            meta: { cellClassName: "text-end tabular-nums" },
            cell: ({ row }) => format.number(row.original[key]),
          }),
        ),
      ] satisfies ColumnDef<SubtreeSummaryRow, unknown>[],
    [format],
  );
  const subtreeState = useServerListState({
    columns: subtreeColumns,
    initialSort: null,
    initialPageSize: STOCK_FORECAST_PAGE_SIZE,
    persistKey: STOCK_FORECAST_SUBTREE_PERSIST_KEY,
  });
  const subtree = useQuery({
    queryKey: stockForecastKeys.subtree(subtreeState.params),
    queryFn: () => stockForecastService.subtree(subtreeState.params),
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
            min: minDataDays,
            left: missingDataDays(insufficient, minDataDays),
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
          inlineEdit: canEditThreshold,
          columnOrdering: true,
          columnPinning: true,
        }}
        bulkActions={bulkActions}
        onCellEdit={({ row, value }) => {
          if (!canEditThreshold) return;
          const current = effectiveThreshold(thresholds, row.product.uuid);
          if (!current) return;
          const cover = thresholdFromValue(value, current.cover_days);
          if (cover === current.cover_days) return;
          thresholdMutation.mutate({ row, cover_days: cover });
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
                  query={{ q: networkState.params.q }}
                  jobsHref={routes.tenant.exports.root(slug)}
                  formats={["xlsx"]}
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
          <EntityTable
            columns={subtreeColumns}
            data={subtree.data?.items ?? []}
            getRowId={(row) => row.organization_uuid}
            rowCount={subtree.data?.total ?? 0}
            isLoading={subtree.isLoading}
            isError={subtree.isError}
            onRetry={() => void subtree.refetch()}
            state={subtreeState.tableState}
            features={{
              persistKey: STOCK_FORECAST_SUBTREE_PERSIST_KEY,
              rowSelection: false,
              columnOrdering: true,
              columnPinning: true,
            }}
          />
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
