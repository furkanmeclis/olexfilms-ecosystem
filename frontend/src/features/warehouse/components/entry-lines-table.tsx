"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { Trash2 } from "lucide-react";
import { useMemo } from "react";

import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { LabelButton } from "@/features/warehouse/components/label-button";
import type { StockEntryLine } from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export const ENTRY_LINES_PERSIST_KEY = "tenant-warehouse-entry-lines-v1";

/** Placed / not placed filter of a line's location column. */
export function placementFilter(
  location: { uuid: string } | null | undefined,
  value: unknown,
): boolean {
  if (value === "placed") return Boolean(location);
  if (value === "unplaced") return !location;
  return true;
}

/**
 * Lines of a stock entry (TEC-204, TEC-376): a client-side DataTable over
 * the lines embedded in the entry (≤ 2000) with sort, search, a placed /
 * not placed filter and, on an editable draft, row selection that feeds
 * "place selected" plus the per-line label and remove actions.
 */
export function EntryLinesTable({
  lines,
  editable,
  selected,
  onSelectedChange,
  onRemove,
  removing,
}: {
  lines: StockEntryLine[];
  editable: boolean;
  selected: string[];
  onSelectedChange: (next: string[]) => void;
  onRemove: (lineUuid: string) => void;
  removing: boolean;
}) {
  const { t, format } = useLocale();

  const columns = useMemo(
    () =>
      [
        ...(editable ? [createSelectColumnDef<StockEntryLine>()] : []),
        createColumn<StockEntryLine>({
          accessorKey: "barcode",
          labelKey: "warehouse.fields.barcode",
          gridPrimary: true,
          cell: ({ row }) => (
            <span
              className="font-mono text-xs"
              dir="ltr"
              data-testid="entry-line"
              data-barcode={row.original.barcode}
            >
              {row.original.barcode}
            </span>
          ),
        }),
        createColumn<StockEntryLine>({
          id: "product",
          accessorFn: (l) => `${l.product.name} ${l.product.sku}`,
          labelKey: "warehouse.fields.product",
          gridSecondary: true,
          cell: ({ row }) => (
            <div>
              {row.original.product.name}
              <div
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.product.sku}
              </div>
            </div>
          ),
        }),
        createColumn<StockEntryLine>({
          accessorKey: "quantity",
          labelKey: "warehouse.fields.quantity",
          cell: ({ row }) => format.number(row.original.quantity),
        }),
        createColumn<StockEntryLine>({
          id: "location",
          accessorFn: (l) => l.location?.full_code ?? l.location?.code ?? "",
          labelKey: "warehouse.fields.location",
          filterVariant: "select",
          filterOptions: [
            {
              value: "placed",
              label: "placed",
              labelKey: "warehouse.entry.placed_option",
            },
            {
              value: "unplaced",
              label: "unplaced",
              labelKey: "warehouse.entry.unplaced",
            },
          ],
          filterFn: (row, _id, value) =>
            placementFilter(row.original.location, value),
          cell: ({ row }) => (
            <span data-testid="entry-line-location">
              {row.original.location ? (
                <span className="font-mono text-xs" dir="ltr">
                  {row.original.location.full_code ??
                    row.original.location.code}
                </span>
              ) : (
                <span className="text-muted-foreground text-xs">
                  {t("warehouse.entry.unplaced")}
                </span>
              )}
            </span>
          ),
        }),
        createColumn<StockEntryLine>({
          id: "actions",
          labelKey: "warehouse.entry.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const l = row.original;
            return (
              <div className="flex justify-end gap-1">
                <LabelButton
                  path={l.label_url}
                  filename={`${l.barcode}.pdf`}
                  size="icon-sm"
                  variant="ghost"
                />
                {editable ? (
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("warehouse.entry.remove_line", {
                      barcode: l.barcode,
                    })}
                    onClick={() => onRemove(l.uuid)}
                    disabled={removing}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                ) : null}
              </div>
            );
          },
        }),
      ] as ColumnDef<StockEntryLine, unknown>[],
    [editable, format, onRemove, removing, t],
  );

  const rowSelection = useMemo<RowSelectionState>(
    () => Object.fromEntries(selected.map((id) => [id, true])),
    [selected],
  );

  return (
    <EntityTable
      columns={columns}
      data={lines}
      getRowId={(row) => row.uuid}
      manual={CLIENT_SIDE_MANUAL}
      emptyTitle={t("warehouse.entry.no_lines")}
      emptyDescription=""
      initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
      pageSizeOptions={[20, 50, 100, 200]}
      state={
        editable
          ? {
              rowSelection,
              onRowSelectionChange: (updater) => {
                const next =
                  typeof updater === "function"
                    ? updater(rowSelection)
                    : updater;
                onSelectedChange(
                  Object.keys(next).filter((id) => next[id] === true),
                );
              },
            }
          : undefined
      }
      features={{
        persistKey: ENTRY_LINES_PERSIST_KEY,
        rowSelection: editable,
        viewMode: true,
      }}
    />
  );
}
