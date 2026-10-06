"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Boxes, Printer } from "lucide-react";
import { useCallback, useMemo, useRef, useState } from "react";

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
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ExportMenu } from "@/features/io/components/export-menu";
import {
  CONSUMED_STATUS,
  showPurchasePrice,
  STOCK_FILTER_STATUSES,
  STOCK_TABS,
  unitStatusTone,
  type StockTab,
} from "@/features/stock/lib/stock";
import {
  stockKeys,
  stockService,
  stockUnitsExportPath,
  type StockUnitListQuery,
  type StockUnitRow,
} from "@/features/stock/services/stock.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const STOCK_PAGE_SIZE = 20;
export const STOCK_UNITS_PERSIST_KEY = "tenant-stock-units-v1";

/** The active organization itself in the dealer picker. */
const OWN = "own";

/** Filter-only column fed by the product picker (product_uuid). */
const PRODUCT_FILTER = "product_filter";

const selectClass =
  "border-input bg-background h-9 w-full rounded-md border px-2 text-sm";

/** Barcode column filter: a prefix match (TEC-373 barcode_match). */
function barcodeParams(value: unknown): Record<string, string | undefined> {
  const barcode = typeof value === "string" ? value.trim() : "";
  return barcode
    ? { barcode, barcode_match: "prefix" }
    : { barcode: undefined, barcode_match: undefined };
}

/**
 * Tenant > My stock (TEC-224, K12; TEC-374 DataTable): the units held by the
 * active dealer (or distributor) and, on the second tab, the units consumed
 * in services (status `used`). A distributor picks one dealer of its subtree
 * and reads that dealer's stock as is (stock.read at scope subtree,
 * TEC-216). Server table with sort, search, barcode / status / updated
 * filters, an async product picker, label printing for one or the selected
 * units and export. The purchase price column shows only when the API
 * answers a price (K8).
 */
export function MyStockPage({
  slug,
  initialBarcode,
  initialOrganization,
}: {
  slug: string;
  /** TEC-213: palette deep link (?barcode=). */
  initialBarcode?: string;
  /** TEC-213: palette deep link to an organization's stock (?organization=). */
  initialOrganization?: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const canRead = can(Permission.StockRead);
  const isDistributor = org?.type === "distributor";

  const [tab, setTab] = useState<StockTab>("stock");
  const [dealer, setDealer] = useState<string>(initialOrganization || OWN);
  const [product, setProduct] = useState<ComboboxOption | null>(null);
  const [printing, setPrinting] = useState(false);
  const [initialFilters] = useState(() =>
    initialBarcode ? [{ id: "barcode", value: initialBarcode }] : [],
  );

  const targetUuid = dealer === OWN ? (org?.uuid ?? "") : dealer;

  const dealers = useQuery({
    queryKey: stockKeys.dealers,
    queryFn: () => stockService.listDealers(),
    enabled: canRead && isDistributor,
    staleTime: 60_000,
  });

  // The product picker searches the organization's stock products page by
  // page (TEC-373: the endpoint caps limit at 100).
  const seenProducts = useRef(new Map<string, ComboboxOption>());
  const loadProducts = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      if (!targetUuid) return [];
      const query = q.trim();
      const page = await stockService.listProducts(targetUuid, {
        ...(query ? { q: query } : {}),
        limit: 20,
        offset: 0,
      });
      const options = page.items.map((p) => ({
        value: p.product.uuid,
        label: p.product.name,
        description: p.product.sku,
      }));
      for (const option of options) {
        seenProducts.current.set(option.value, option);
      }
      return options;
    },
    [targetUuid],
  );

  const printLabels = useCallback(
    async (rows: readonly StockUnitRow[]) => {
      if (rows.length === 0) return;
      setPrinting(true);
      try {
        const { blob, filename } = await stockService.unitLabels(
          rows.map((r) => r.barcode),
        );
        triggerBrowserDownload(
          blob,
          filename ??
            (rows.length === 1 ? `${rows[0].barcode}.pdf` : "labels.pdf"),
        );
      } catch {
        appToast.error(t("warehouse.labels.failed"));
      } finally {
        setPrinting(false);
      }
    },
    [t],
  );

  // Without a page of rows the price column cannot be decided; it is added
  // once a row carries a price.
  const [priceColumn, setPriceColumn] = useState(false);

  const columns = useMemo(() => {
    const cols: ColumnDef<StockUnitRow, unknown>[] = [
      createSelectColumnDef<StockUnitRow>(),
      createColumn<StockUnitRow>({
        accessorKey: "barcode",
        labelKey: "stock.columns.unit",
        enableSorting: true,
        gridPrimary: true,
        filterVariant: "text",
        param: "barcode",
        paramFormat: barcodeParams,
        // TEC-213: the palette deep link (?barcode=) fills this input.
        meta: { filterInputId: "stock-barcode" },
        cell: ({ row }) => (
          <div data-testid="stock-row" data-uuid={row.original.uuid}>
            <span className="font-mono text-xs font-medium" dir="ltr">
              {row.original.barcode}
            </span>
            <div className="text-muted-foreground text-xs">
              {t(`stock.unit_kind.${row.original.unit_kind}`)}
            </div>
          </div>
        ),
      }) as ColumnDef<StockUnitRow, unknown>,
      createColumn<StockUnitRow>({
        id: "product",
        accessorFn: (r) => r.product.name,
        labelKey: "stock.columns.product",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => (
          <div>
            <div className="font-medium">{row.original.product.name}</div>
            <div className="text-muted-foreground font-mono text-xs">
              {row.original.product.sku}
            </div>
          </div>
        ),
      }) as ColumnDef<StockUnitRow, unknown>,
      // Filter only: the product picker above the table sets it.
      createColumn<StockUnitRow>({
        id: PRODUCT_FILTER,
        accessorFn: (r) => r.product.uuid,
        labelKey: "stock.filters.product",
        enableSorting: false,
        enableHiding: false,
        enableColumnFilter: false,
        defaultHidden: true,
        param: "product_uuid",
        paramFormat: "string",
        cell: () => null,
      }) as ColumnDef<StockUnitRow, unknown>,
      createColumn<StockUnitRow>({
        accessorKey: "status",
        labelKey: "stock.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: STOCK_FILTER_STATUSES.map((value) => ({
          value,
          label: value,
          labelKey: `stock.status.${value}`,
        })),
        // The consumed tab pins status=used.
        enableColumnFilter: tab === "stock",
        param: "status",
        cell: ({ row }) => (
          <StatusChip
            label={t(`stock.status.${row.original.status}`)}
            tone={unitStatusTone(row.original.status)}
          />
        ),
      }) as ColumnDef<StockUnitRow, unknown>,
      createColumn<StockUnitRow>({
        accessorKey: "quantity",
        labelKey: "stock.columns.quantity",
        enableSorting: true,
        meta: { cellClassName: "text-end tabular-nums" },
        cell: ({ row }) => format.number(row.original.quantity),
      }) as ColumnDef<StockUnitRow, unknown>,
      createColumn<StockUnitRow>({
        id: "meters",
        accessorFn: (r) => Number(r.remaining_meters ?? 0),
        labelKey: "stock.columns.meters",
        enableSorting: true,
        meta: { cellClassName: "text-end whitespace-nowrap tabular-nums" },
        cell: ({ row }) =>
          row.original.product.unit_type === "roll_meter" &&
          row.original.remaining_meters
            ? t("stock.list.meters_of", {
                remaining: row.original.remaining_meters,
                initial:
                  row.original.initial_meters ?? row.original.remaining_meters,
              })
            : "—",
      }) as ColumnDef<StockUnitRow, unknown>,
      createColumn<StockUnitRow>({
        id: "location",
        accessorFn: (r) => r.location?.code ?? "",
        labelKey: "stock.columns.location",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.location ? (
            <>
              <span className="font-mono text-xs" dir="ltr">
                {row.original.location.code}
              </span>
              <div className="text-muted-foreground text-xs">
                {row.original.location.name}
              </div>
            </>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      }) as ColumnDef<StockUnitRow, unknown>,
    ];
    if (priceColumn) {
      cols.push(
        createColumn<StockUnitRow>({
          id: "purchase_price",
          accessorFn: (r) => Number(r.purchase_price?.amount ?? 0),
          labelKey: "stock.columns.purchase_price",
          enableSorting: false,
          meta: { cellClassName: "text-end whitespace-nowrap tabular-nums" },
          cell: ({ row }) => (
            <span data-testid="stock-price">
              {row.original.purchase_price
                ? format.currency(
                    Number(row.original.purchase_price.amount),
                    row.original.purchase_price.currency,
                  )
                : "—"}
            </span>
          ),
        }) as ColumnDef<StockUnitRow, unknown>,
      );
    }
    cols.push(
      createColumn<StockUnitRow>({
        accessorKey: "updated_at",
        labelKey: "stock.columns.updated",
        enableSorting: true,
        filterVariant: "date-range",
        param: "updated",
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs whitespace-nowrap">
            {format.dateTime(row.original.updated_at)}
          </span>
        ),
      }) as ColumnDef<StockUnitRow, unknown>,
      createColumn<StockUnitRow>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <EntityRowActions
            actions={[
              {
                id: "print_label",
                label: t("warehouse.labels.print"),
                icon: Printer,
                disabled: printing,
                onSelect: () => void printLabels([row.original]),
              },
            ]}
          />
        ),
      }) as ColumnDef<StockUnitRow, unknown>,
    );
    return cols;
  }, [format, priceColumn, printLabels, printing, t, tab]);

  // Column meta drives the params: barcode (prefix), product, status
  // (CSV), updated (updated_from/_to).
  const listState = useServerListState({
    columns,
    initialSort: "product",
    initialPageSize: STOCK_PAGE_SIZE,
    persistKey: STOCK_UNITS_PERSIST_KEY,
    initialColumnFilters: initialFilters,
  });
  const params: StockUnitListQuery = useMemo(
    () =>
      tab === "consumed"
        ? { ...listState.params, status: CONSUMED_STATUS }
        : listState.params,
    [listState.params, tab],
  );

  const list = useQuery({
    queryKey: stockKeys.units(targetUuid, params),
    queryFn: () => stockService.listUnits(targetUuid, params),
    enabled: canRead && targetUuid !== "",
  });
  const rows = useMemo(() => list.data?.items ?? [], [list.data]);
  const total = list.data?.total ?? 0;
  const hasPrice = showPurchasePrice(rows);
  if (hasPrice !== priceColumn && !list.isFetching) setPriceColumn(hasPrice);

  // Export uses the same tab, filters, search and sort as the list.
  const exportQuery = useMemo(
    () => ({
      ...listState.filterParams,
      ...(tab === "consumed" ? { status: CONSUMED_STATUS } : {}),
      q: params.q,
      sort: params.sort,
    }),
    [listState.filterParams, params.q, params.sort, tab],
  );

  const bulkActions = useMemo<DataTableBulkAction<StockUnitRow>[]>(
    () => [
      {
        id: "print_labels",
        label: t("bulk.actions.stock_units.print_labels"),
        icon: Printer,
        disabled: printing,
        onClick: (selected) => void printLabels(selected),
      },
    ],
    [printLabels, printing, t],
  );

  const { onColumnFiltersChange, setPagination, columnFilters } = listState;
  const pickProduct = (option: ComboboxOption | null) => {
    setProduct(option);
    onColumnFiltersChange((prev) => [
      ...prev.filter((f) => f.id !== PRODUCT_FILTER),
      ...(option ? [{ id: PRODUCT_FILTER, value: option.value }] : []),
    ]);
  };
  const pickTab = (next: StockTab) => {
    setTab(next);
    // The consumed tab pins its own status.
    onColumnFiltersChange((prev) => prev.filter((f) => f.id !== "status"));
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  };

  const title = t("stock.page.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Boxes className="size-6" />}
      description={t("stock.page.description")}
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
          description={t("stock.page.forbidden")}
        />
      </div>
    );
  }

  const viewingDealer = isDistributor && dealer !== OWN;
  const dealerName = dealers.data?.find((d) => d.uuid === dealer)?.name ?? "";
  const filtered = columnFilters.length > 0 || Boolean(params.q);

  return (
    <div className="space-y-6">
      {header}

      {isDistributor ? (
        <Card>
          <CardContent className="space-y-2 pt-6">
            <Label htmlFor="stock-dealer">{t("stock.dealer.label")}</Label>
            <select
              id="stock-dealer"
              data-testid="stock-dealer"
              className={cn(selectClass, "max-w-md")}
              value={dealer}
              onChange={(e) => {
                setDealer(e.target.value);
                setProduct(null);
                onColumnFiltersChange([]);
                setPagination((prev) => ({ ...prev, pageIndex: 0 }));
              }}
            >
              <option value={OWN}>
                {t("stock.dealer.own", { name: org?.name ?? "" })}
              </option>
              {(dealers.data ?? []).map((d) => (
                <option key={d.uuid} value={d.uuid}>
                  {d.name}
                </option>
              ))}
            </select>
            <p className="text-muted-foreground text-xs">
              {viewingDealer
                ? t("stock.dealer.read_only", { name: dealerName })
                : t("stock.dealer.hint")}
            </p>
          </CardContent>
        </Card>
      ) : null}

      <div
        className="flex flex-wrap gap-2"
        role="tablist"
        aria-label={t("stock.tabs.label")}
      >
        {STOCK_TABS.map((value) => {
          const active = tab === value;
          return (
            <Button
              key={value}
              type="button"
              role="tab"
              size="sm"
              variant={active ? "default" : "outline"}
              aria-selected={active}
              data-testid={`stock-tab-${value}`}
              onClick={() => pickTab(value)}
            >
              {t(`stock.tabs.${value}`)}
            </Button>
          );
        })}
      </div>

      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t(
          tab === "consumed"
            ? "stock.list.empty_consumed_title"
            : "stock.list.empty_title",
        )}
        emptyDescription={t(
          filtered
            ? "stock.list.empty_filtered"
            : tab === "consumed"
              ? "stock.list.empty_consumed_description"
              : "stock.list.empty_description",
        )}
        rowCount={total}
        state={listState.tableState}
        features={{
          persistKey: STOCK_UNITS_PERSIST_KEY,
          rowSelection: true,
          viewMode: true,
        }}
        bulkActions={bulkActions}
        renderGridItem={(r) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <span className="font-mono text-sm font-semibold" dir="ltr">
                {r.barcode}
              </span>
              <StatusChip
                label={t(`stock.status.${r.status}`)}
                tone={unitStatusTone(r.status)}
              />
            </div>
            <p className="text-sm">
              {r.product.name}{" "}
              <span className="text-muted-foreground font-mono text-xs">
                {r.product.sku}
              </span>
            </p>
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>
                {r.product.unit_type === "roll_meter" && r.remaining_meters
                  ? t("stock.list.meters_of", {
                      remaining: r.remaining_meters,
                      initial: r.initial_meters ?? r.remaining_meters,
                    })
                  : format.number(r.quantity)}
              </span>
              <span>{format.dateTime(r.updated_at)}</span>
            </div>
          </div>
        )}
        toolbarExtra={
          <>
            <div className="w-full sm:w-64" data-testid="stock-product">
              <AsyncCombobox
                id="stock-product"
                value={product?.value ?? ""}
                options={product ? [product] : undefined}
                clearable
                loadOptions={loadProducts}
                onValueChange={(value) =>
                  pickProduct(
                    value
                      ? (seenProducts.current.get(value) ?? {
                          value,
                          label: value,
                        })
                      : null,
                  )
                }
                placeholder={t("stock.filters.all_products")}
                searchPlaceholder={t("stock.filters.product_search")}
                emptyText={t("stock.filters.product_none")}
              />
            </div>
            <ExportMenu
              exportPath={stockUnitsExportPath(targetUuid)}
              query={exportQuery}
              formats={["xlsx", "csv", "pdf"]}
              disabled={targetUuid === ""}
              jobsHref={routes.tenant.exports.root(slug)}
            />
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
    </div>
  );
}
