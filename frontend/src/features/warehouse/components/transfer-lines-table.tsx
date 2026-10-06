"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { Trash2 } from "lucide-react";
import { useMemo } from "react";

import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { placementFilter } from "@/features/warehouse/components/entry-lines-table";
import type { WarehouseTransferLine } from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

export const TRANSFER_LINES_PERSIST_KEY = "tenant-warehouse-transfer-lines-v1";

/**
 * Lines of a warehouse transfer (TEC-205, TEC-376): a client-side
 * DataTable over the embedded lines with sort, search and a targeted /
 * not targeted filter. While target locations can be set, row selection
 * narrows "place" to the selected lines; a draft can remove lines.
 */
export function TransferLinesTable({
  lines,
  selectable,
  removable,
  selected,
  onSelectedChange,
  onRemove,
  removing,
}: {
  lines: WarehouseTransferLine[];
  selectable: boolean;
  removable: boolean;
  selected: string[];
  onSelectedChange: (next: string[]) => void;
  onRemove: (lineUuid: string) => void;
  removing: boolean;
}) {
  const { t } = useLocale();

  const columns = useMemo(
    () =>
      [
        ...(selectable ? [createSelectColumnDef<WarehouseTransferLine>()] : []),
        createColumn<WarehouseTransferLine>({
          accessorKey: "barcode",
          labelKey: "warehouse.fields.barcode",
          gridPrimary: true,
          cell: ({ row }) => (
            <span
              className="font-mono text-xs"
              dir="ltr"
              data-testid="transfer-line"
              data-barcode={row.original.barcode}
            >
              {row.original.barcode}
            </span>
          ),
        }),
        createColumn<WarehouseTransferLine>({
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
        createColumn<WarehouseTransferLine>({
          id: "source",
          accessorFn: (l) =>
            l.source_location?.full_code ?? l.source_location?.code ?? "",
          labelKey: "warehouse.transfer.source",
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.source_location?.full_code ??
                row.original.source_location?.code ??
                "—"}
            </span>
          ),
        }),
        createColumn<WarehouseTransferLine>({
          id: "target",
          accessorFn: (l) =>
            l.target_location?.full_code ?? l.target_location?.code ?? "",
          labelKey: "warehouse.transfer.target",
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
            placementFilter(row.original.target_location, value),
          cell: ({ row }) => (
            <span data-testid="transfer-line-target">
              {row.original.target_location ? (
                <span className="font-mono text-xs" dir="ltr">
                  {row.original.target_location.full_code ??
                    row.original.target_location.code}
                </span>
              ) : (
                <span className="text-muted-foreground text-xs">
                  {t("warehouse.entry.unplaced")}
                </span>
              )}
            </span>
          ),
        }),
        ...(removable
          ? [
              createColumn<WarehouseTransferLine>({
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
                      aria-label={t("warehouse.entry.remove_line", {
                        barcode: row.original.barcode,
                      })}
                      onClick={() => onRemove(row.original.uuid)}
                      disabled={removing}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                ),
              }),
            ]
          : []),
      ] as ColumnDef<WarehouseTransferLine, unknown>[],
    [onRemove, removable, removing, selectable, t],
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
        selectable
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
        persistKey: TRANSFER_LINES_PERSIST_KEY,
        rowSelection: selectable,
        viewMode: true,
      }}
    />
  );
}
