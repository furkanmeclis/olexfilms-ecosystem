"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Barcode, Printer } from "lucide-react";
import { useMemo, useState } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { GenerateForm } from "@/features/warehouse/components/generate-form";
import { LabelButton } from "@/features/warehouse/components/label-button";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  warehouseKeys,
  warehouseService,
  type BarcodeBatch,
  type BatchListQuery,
  warehouseExportPaths,
} from "@/features/warehouse/services/warehouse.service";
import {
  listExportQuery,
  useExportSelection,
  WarehouseExportMenu,
} from "@/features/warehouse/components/warehouse-export";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { voidHeldWarnings } from "@/features/warehouse/lib/void-result";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";
import { useDialogs } from "@/providers/dialog-provider";

export const BATCH_PAGE_SIZE = 20;
export const BATCHES_PERSIST_KEY = "tenant-warehouse-barcodes-v1";

/**
 * Warehouse > Barcodes (TEC-202, center only, K14): reserve N barcodes of
 * a product (units in status reserved until printed, then enter stock through a stock
 * entry) and print the batch label sheet. The batches are a server
 * DataTable (TEC-376): sort (created, quantity, printed count), search
 * (prefix, range, product, any unit barcode), product / printed / created
 * filters, a print-labels row action and mobile cards.
 */
export function BarcodesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const base = useWarehouseAccess(slug, Permission.StockRead);
  const access = { ...base, allowed: base.allowed && base.isCenter };
  const canWrite = access.can(Permission.StockWrite);
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const [last, setLast] = useState<BarcodeBatch | null>(null);

  // Product filter options: the first 100 catalog products (API limit).
  const products = useQuery({
    queryKey: ["warehouse", "batch-filter-products"],
    queryFn: () => catalogService.listProducts({ limit: 100 }),
    enabled: access.allowed,
    staleTime: 5 * 60_000,
  });
  const productOptions = useMemo(
    () =>
      (products.data?.items ?? []).map((p) => ({
        value: p.uuid,
        label: `${p.sku} · ${p.name}`,
      })),
    [products.data?.items],
  );

  const create = useMutation({
    mutationFn: warehouseService.createBatch,
    onSuccess: async (batch) => {
      setLast(batch);
      appToast.success(
        t("warehouse.barcodes.created", { count: batch.quantity }),
      );
      await qc.invalidateQueries({ queryKey: ["warehouse", "batches"] });
    },
  });

  const columns = useMemo(() => {
    const print = async (b: BarcodeBatch) => {
      try {
        const { blob, filename } = await platformDownloadFile(b.labels_url);
        triggerBrowserDownload(blob, filename ?? `${b.first_barcode}.pdf`);
        // The sheet download counts as a print (print_count).
        void qc.invalidateQueries({ queryKey: ["warehouse", "batches"] });
      } catch {
        appToast.error(t("warehouse.labels.failed"));
      }
    };
    return [
      createSelectColumnDef<BarcodeBatch>(),
      createColumn<BarcodeBatch>({
        id: "product",
        accessorFn: (b) => b.product.name,
        labelKey: "warehouse.fields.product",
        enableSorting: false,
        gridPrimary: true,
        filterVariant: "faceted",
        filterOptions: productOptions,
        enableColumnFilter: productOptions.length > 0,
        param: "product_uuid",
        cell: ({ row }) => (
          <div data-testid="batch-row" data-uuid={row.original.uuid}>
            {row.original.product.name}
            <div className="text-muted-foreground font-mono text-xs" dir="ltr">
              {row.original.product.sku}
            </div>
          </div>
        ),
      }),
      createColumn<BarcodeBatch>({
        id: "range",
        accessorFn: (b) => b.first_barcode,
        labelKey: "warehouse.barcodes.columns.range",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => (
          <span className="font-mono text-xs" dir="ltr">
            {row.original.first_barcode} – {row.original.last_barcode}
          </span>
        ),
      }),
      createColumn<BarcodeBatch>({
        accessorKey: "quantity",
        labelKey: "warehouse.fields.quantity",
        enableSorting: true,
        cell: ({ row }) =>
          `${format.number(row.original.quantity)}${
            row.original.meters ? ` × ${row.original.meters} m` : ""
          }`,
      }),
      createColumn<BarcodeBatch>({
        accessorKey: "print_count",
        labelKey: "warehouse.barcodes.columns.printed",
        enableSorting: true,
        // printed=true|false → print_count > 0 (TEC-375).
        filterVariant: "boolean",
        param: "printed",
        cell: ({ row }) => format.number(row.original.print_count),
      }),
      createColumn<BarcodeBatch>({
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
      createColumn<BarcodeBatch>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const items: EntityRowAction[] = [
            {
              id: "print",
              label: t("warehouse.labels.print"),
              icon: Printer,
              onSelect: () => void print(row.original),
            },
          ];
          return <EntityRowActions actions={items} />;
        },
      }),
    ] as ColumnDef<BarcodeBatch, unknown>[];
  }, [format, productOptions, qc, t]);

  // Column meta drives the params: product_uuid (CSV), printed (boolean),
  // created (created_from/_to); sort created_at | quantity | print_count.
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: BATCH_PAGE_SIZE,
    persistKey: BATCHES_PERSIST_KEY,
  });
  const query: BatchListQuery = listState.params;
  // TEC-761: the export keeps the list filters, q, sort and the selection.
  const selection = useExportSelection();
  const voidSelected = useMutation({
    mutationFn: async (ids: string[]) => {
      const results = await Promise.all(
        ids.map((batch_uuid) => warehouseService.voidBarcodes({ batch_uuid })),
      );
      return results.flatMap((result) => result.items);
    },
    onSuccess: (items) => {
      appToast.success(
        t("warehouse.barcodes.void_result", {
          voided: items.filter((item) => item.voided).length,
          rejected: items.filter((item) => !item.voided).length,
        }),
      );
      for (const message of voidHeldWarnings(items, t))
        appToast.warning(message);
      selection.onRowSelectionChange({});
      void qc.invalidateQueries({ queryKey: ["warehouse", "batches"] });
    },
    onError: () => appToast.error(t("warehouse.form.error")),
  });
  const askVoid = async () => {
    const ok = await confirm({
      title: t("warehouse.barcodes.void_title"),
      description: t("warehouse.barcodes.void_description", {
        count: selection.selected.length,
      }),
      confirmLabel: t("warehouse.barcodes.void_selected"),
      variant: "destructive",
    });
    if (ok) voidSelected.mutate(selection.selected);
  };
  const exportQuery = listExportQuery(
    listState.filterParams,
    listState.params,
    selection.selected,
  );
  const list = useQuery({
    queryKey: warehouseKeys.batches(query),
    queryFn: () => warehouseService.listBatches(query),
    enabled: access.allowed,
  });

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.barcodes.title")}
      description={t("warehouse.barcodes.description")}
      icon={<Barcode className="size-6" />}
      forbiddenKey="warehouse.barcodes.forbidden"
    >
      {canWrite ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("warehouse.barcodes.new_title")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <GenerateForm
              submitLabel={t("warehouse.barcodes.generate")}
              pending={create.isPending}
              onSubmit={(body) => create.mutateAsync(body)}
            />
            {last ? (
              <div
                className="bg-muted/50 flex flex-wrap items-center justify-between gap-2 rounded-md p-3 text-sm"
                data-testid="batch-created"
              >
                <span>
                  {t("warehouse.barcodes.range", {
                    first: last.first_barcode,
                    last: last.last_barcode,
                  })}
                </span>
                <LabelButton
                  path={last.labels_url}
                  filename={`${last.first_barcode}.pdf`}
                />
              </div>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("warehouse.barcodes.batches")}</CardTitle>
        </CardHeader>
        <CardContent>
          <EntityTable
            columns={columns}
            data={list.data?.items ?? []}
            getRowId={(row) => row.uuid}
            isLoading={list.isLoading}
            isError={list.isError}
            onRetry={() => void list.refetch()}
            emptyTitle={t("warehouse.barcodes.empty")}
            emptyDescription=""
            rowCount={list.data?.total ?? 0}
            state={{
              ...listState.tableState,
              rowSelection: selection.rowSelection,
              onRowSelectionChange: selection.onRowSelectionChange,
            }}
            features={{
              persistKey: BATCHES_PERSIST_KEY,
              rowSelection: true,
              viewMode: true,
            }}
            renderGridItem={(b) => (
              <div className="space-y-2">
                <div className="flex items-start justify-between gap-2">
                  <span className="font-medium">{b.product.name}</span>
                  <LabelButton
                    path={b.labels_url}
                    filename={`${b.first_barcode}.pdf`}
                    size="icon-sm"
                    variant="ghost"
                  />
                </div>
                <p className="font-mono text-xs" dir="ltr">
                  {b.first_barcode} – {b.last_barcode}
                </p>
                <div className="text-muted-foreground flex justify-between gap-2 text-xs">
                  <span>
                    {format.number(b.quantity)} · {format.number(b.print_count)}
                  </span>
                  <span>{format.dateTime(b.created_at)}</span>
                </div>
              </div>
            )}
            toolbarExtra={
              <>
                {canWrite && selection.selected.length > 0 ? (
                  <Button
                    type="button"
                    variant="destructive"
                    disabled={voidSelected.isPending}
                    onClick={() => void askVoid()}
                  >
                    {t("warehouse.barcodes.void_selected")}
                  </Button>
                ) : null}
                <WarehouseExportMenu
                  slug={slug}
                  exportPath={warehouseExportPaths.batches}
                  query={exportQuery}
                  testId="batches-export"
                />
                <EntityToolbar
                  onRefresh={() => void list.refetch()}
                  refreshDisabled={list.isFetching}
                />
              </>
            }
          />
        </CardContent>
      </Card>
    </WarehouseShell>
  );
}
