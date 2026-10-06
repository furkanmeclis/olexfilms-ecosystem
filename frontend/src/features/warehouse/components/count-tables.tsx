"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { ListChecks, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import {
  createColumn,
  createSelectColumnDef,
  type DataTableBulkAction,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { nativeSelectClass } from "@/features/warehouse/components/native-select-field";
import { enumFilterOptions } from "@/features/warehouse/components/table-options";
import {
  COUNT_RESOLUTIONS,
  COUNT_RESULTS,
  COUNT_SCAN_KINDS,
  defaultResolution,
} from "@/features/warehouse/lib/counts";
import type {
  StockCount,
  StockCountLine,
  StockCountResolution,
  StockCountScan,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export const COUNT_EXPECTED_PERSIST_KEY = "tenant-warehouse-count-expected-v1";
export const COUNT_SCANS_PERSIST_KEY = "tenant-warehouse-count-scans-v1";
export const COUNT_LINES_PERSIST_KEY = "tenant-warehouse-count-lines-v1";

type ExpectedLocation = NonNullable<
  StockCount["expected"]
>["locations"][number];

/**
 * Guided counts (TEC-206, TEC-376): what the ledger expects per location,
 * a small client-side DataTable (sort, search).
 */
export function ExpectedLocationsTable({
  locations,
}: {
  locations: ExpectedLocation[];
}) {
  const { t, format } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<ExpectedLocation>({
          id: "location",
          accessorFn: (l) => l.location?.full_code ?? "",
          labelKey: "warehouse.fields.location",
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.location?.full_code ?? t("warehouse.scan.unplaced")}
            </span>
          ),
        }),
        createColumn<ExpectedLocation>({
          accessorKey: "serial_units",
          labelKey: "warehouse.count.serial_units",
          cell: ({ row }) => format.number(row.original.serial_units),
        }),
        createColumn<ExpectedLocation>({
          accessorKey: "fixed_quantity",
          labelKey: "warehouse.count.fixed_quantity",
          cell: ({ row }) => format.number(row.original.fixed_quantity),
        }),
      ] as ColumnDef<ExpectedLocation, unknown>[],
    [format, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={locations}
      getRowId={(row, i) => row.location?.uuid ?? `none-${i}`}
      manual={CLIENT_SIDE_MANUAL}
      emptyTitle={t("warehouse.count.no_expected")}
      emptyDescription=""
      initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
      features={{
        persistKey: COUNT_EXPECTED_PERSIST_KEY,
        rowSelection: false,
        columnFilters: false,
      }}
    />
  );
}

/** Code shown for a scan: the unit barcode, the SKU or the raw code. */
export function scanCode(s: StockCountScan): string {
  return s.unit?.barcode ?? s.product?.sku ?? s.raw_code;
}

/**
 * Scans of a count (TEC-206, TEC-376): a client-side DataTable, newest
 * first, with sort, search, a kind filter and, while counting, the remove
 * action. Location scans only set the context and are not listed.
 */
export function CountScansTable({
  scans,
  isLoading,
  editable,
  onRemove,
  removing,
}: {
  scans: StockCountScan[];
  isLoading: boolean;
  editable: boolean;
  onRemove: (scan: StockCountScan) => void;
  removing: boolean;
}) {
  const { t, format } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<StockCountScan>({
          id: "code",
          accessorFn: scanCode,
          labelKey: "warehouse.fields.code",
          gridPrimary: true,
          cell: ({ row }) => (
            <span
              className="font-mono text-xs"
              dir="ltr"
              data-testid="count-scan-row"
            >
              {scanCode(row.original)}
            </span>
          ),
        }),
        createColumn<StockCountScan>({
          accessorKey: "kind",
          labelKey: "warehouse.fields.type",
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(
            COUNT_SCAN_KINDS,
            "warehouse.count_scan_kind",
          ),
          cell: ({ row }) =>
            t(`warehouse.count_scan_kind.${row.original.kind}`),
        }),
        createColumn<StockCountScan>({
          id: "product",
          accessorFn: (s) => s.product?.name ?? "",
          labelKey: "warehouse.fields.product",
          gridSecondary: true,
        }),
        createColumn<StockCountScan>({
          id: "location",
          accessorFn: (s) => s.location?.full_code ?? "",
          labelKey: "warehouse.fields.location",
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.location?.full_code ?? "—"}
            </span>
          ),
        }),
        createColumn<StockCountScan>({
          accessorKey: "quantity",
          labelKey: "warehouse.fields.quantity",
          cell: ({ row }) => format.number(row.original.quantity),
        }),
        createColumn<StockCountScan>({
          accessorKey: "meters",
          labelKey: "warehouse.fields.meters",
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.meters ? (
              <span dir="ltr">{row.original.meters} m</span>
            ) : (
              "—"
            ),
        }),
        createColumn<StockCountScan>({
          accessorKey: "created_at",
          labelKey: "warehouse.count.scanned_at",
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        ...(editable
          ? [
              createColumn<StockCountScan>({
                id: "actions",
                labelKey: "warehouse.entry.actions",
                enableSorting: false,
                enableHiding: false,
                enableResizing: false,
                cell: ({ row }) => (
                  <div className="flex justify-end">
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      aria-label={t("warehouse.count.remove_scan", {
                        code:
                          row.original.unit?.barcode ?? row.original.raw_code,
                      })}
                      disabled={removing}
                      onClick={() => onRemove(row.original)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                ),
              }),
            ]
          : []),
      ] as ColumnDef<StockCountScan, unknown>[],
    [editable, format, onRemove, removing, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={scans}
      getRowId={(row) => row.uuid}
      manual={CLIENT_SIDE_MANUAL}
      isLoading={isLoading}
      emptyTitle={t("warehouse.count.no_scans")}
      emptyDescription=""
      initialState={{
        pagination: { pageIndex: 0, pageSize: 20 },
        sorting: [{ id: "created_at", desc: true }],
      }}
      features={{
        persistKey: COUNT_SCANS_PERSIST_KEY,
        rowSelection: false,
        viewMode: true,
      }}
    />
  );
}

/** One difference line: what was expected, what was counted. */
export function countLineText(l: StockCountLine): {
  expected: string;
  counted: string;
} {
  const where = (loc: StockCountLine["expected_location"]) =>
    loc?.full_code ?? "—";
  const qty = (n: number, m: string | null) => (m ? `${n} · ${m} m` : `${n}`);
  return {
    expected: `${where(l.expected_location)} · ${qty(l.expected_quantity, l.expected_meters)}`,
    counted: `${where(l.counted_location)} · ${qty(l.counted_quantity, l.counted_meters)}`,
  };
}

/**
 * Selected lines a bulk resolution applies to: only the lines that allow
 * it (the others keep their choice).
 */
export function bulkResolutionChoices(
  selected: StockCountLine[],
  resolution: StockCountResolution,
): Record<string, StockCountResolution> {
  return Object.fromEntries(
    selected
      .filter((l) => l.allowed_resolutions.includes(resolution))
      .map((l) => [l.uuid, resolution]),
  );
}

/**
 * Difference lines of a completed count (TEC-206, TEC-376): a client-side
 * DataTable with sort, search and a result filter. With the approval
 * right, each line has its resolution select and the selected lines can
 * take one resolution at once (bulk, applied where allowed).
 */
export function CountLinesTable({
  lines,
  canApprove,
  choices,
  onChoices,
}: {
  lines: StockCountLine[];
  canApprove: boolean;
  choices: Record<string, StockCountResolution | undefined>;
  onChoices: (next: Record<string, StockCountResolution>) => void;
}) {
  const { t } = useLocale();
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});

  const columns = useMemo(
    () =>
      [
        ...(canApprove ? [createSelectColumnDef<StockCountLine>()] : []),
        createColumn<StockCountLine>({
          id: "product",
          accessorFn: (l) =>
            `${l.product.name} ${l.unit?.barcode ?? l.product.sku}`,
          labelKey: "warehouse.fields.product",
          gridPrimary: true,
          cell: ({ row }) => (
            <div data-testid="count-line" data-result={row.original.result}>
              {row.original.product.name}
              <div
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.unit?.barcode ?? row.original.product.sku}
              </div>
            </div>
          ),
        }),
        createColumn<StockCountLine>({
          accessorKey: "result",
          labelKey: "warehouse.count.result",
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(
            COUNT_RESULTS.filter((r) => r !== "matched"),
            "warehouse.count_result",
          ),
          cell: ({ row }) => t(`warehouse.count_result.${row.original.result}`),
        }),
        createColumn<StockCountLine>({
          id: "expected",
          accessorFn: (l) => countLineText(l).expected,
          labelKey: "warehouse.count.expected",
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {countLineText(row.original).expected}
            </span>
          ),
        }),
        createColumn<StockCountLine>({
          id: "counted",
          accessorFn: (l) => countLineText(l).counted,
          labelKey: "warehouse.count.counted",
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {countLineText(row.original).counted}
            </span>
          ),
        }),
        createColumn<StockCountLine>({
          id: "resolution",
          accessorFn: (l) => choices[l.uuid] ?? defaultResolution(l) ?? "",
          labelKey: "warehouse.count.resolution",
          enableSorting: false,
          cell: ({ row }) => {
            const l = row.original;
            if (!canApprove) {
              return l.resolution
                ? t(`warehouse.count_resolution.${l.resolution}`)
                : "—";
            }
            return (
              <select
                className={nativeSelectClass}
                value={choices[l.uuid] ?? defaultResolution(l) ?? ""}
                aria-label={t("warehouse.count.resolution_for", {
                  code: l.unit?.barcode ?? l.product.sku,
                })}
                data-testid="count-resolution"
                onChange={(e) =>
                  onChoices({
                    [l.uuid]: e.target.value as StockCountResolution,
                  })
                }
              >
                {l.allowed_resolutions.map((r) => (
                  <option key={r} value={r}>
                    {t(`warehouse.count_resolution.${r}`)}
                  </option>
                ))}
              </select>
            );
          },
        }),
      ] as ColumnDef<StockCountLine, unknown>[],
    [canApprove, choices, onChoices, t],
  );

  const bulkActions = useMemo<DataTableBulkAction<StockCountLine>[]>(
    () =>
      COUNT_RESOLUTIONS.map((resolution) => ({
        id: `resolution-${resolution}`,
        label: t("bulk.actions.warehouse_count_lines.set_resolution", {
          resolution: t(`warehouse.count_resolution.${resolution}`),
        }),
        icon: ListChecks,
        disabled: (selected: StockCountLine[]) =>
          !selected.some((l) => l.allowed_resolutions.includes(resolution)),
        onClick: (selected: StockCountLine[]) => {
          onChoices(bulkResolutionChoices(selected, resolution));
          setRowSelection({});
        },
      })),
    [onChoices, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={lines}
      getRowId={(row) => row.uuid}
      manual={CLIENT_SIDE_MANUAL}
      emptyTitle={t("warehouse.count.no_differences")}
      emptyDescription=""
      initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
      pageSizeOptions={[20, 50, 100, 200]}
      state={
        canApprove
          ? { rowSelection, onRowSelectionChange: setRowSelection }
          : undefined
      }
      bulkActions={canApprove ? bulkActions : undefined}
      features={{
        persistKey: COUNT_LINES_PERSIST_KEY,
        rowSelection: canApprove,
        viewMode: true,
      }}
    />
  );
}
