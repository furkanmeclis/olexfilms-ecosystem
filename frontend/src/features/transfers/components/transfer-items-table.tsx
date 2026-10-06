"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { CLIENT_SIDE_MANUAL, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import type { StockTransferItem } from "@/features/transfers/services/transfers.service";
import { useLocale } from "@/providers/locale-provider";

export const TRANSFER_ITEMS_PERSIST_KEY = "tenant-transfer-items-v1";

/** Stock state of a line, in movement order. */
export type TransferItemState = "pending" | "shipped" | "received" | "restored";

const ITEM_STATES: TransferItemState[] = [
  "pending",
  "shipped",
  "received",
  "restored",
];

export function transferItemState(item: StockTransferItem): TransferItemState {
  if (item.restored) return "restored";
  if (item.received) return "received";
  if (item.shipped) return "shipped";
  return "pending";
}

/** Requested amount: fixed quantity, roll meters, or one serial unit. */
function itemAmount(item: StockTransferItem): string {
  if (item.quantity !== null) return String(item.quantity);
  if (item.meters) return `${item.meters} m`;
  return "1";
}

/** Decimal string for sorting; unpriced lines sort first. */
function decimalOrNone(value: string | null): number {
  const n = Number(value);
  return value === null || !Number.isFinite(n) ? -1 : n;
}

/**
 * Units of a transfer request as a nested client-side table (TEC-374):
 * sort, search and a stock state filter over the embedded lines.
 */
export function TransferItemsTable({
  items,
  currency,
}: {
  items: StockTransferItem[];
  currency: string;
}) {
  const { t } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<StockTransferItem>({
          accessorKey: "barcode",
          labelKey: "transfers.form.barcode",
          gridPrimary: true,
          cell: ({ row }) => (
            <span
              className="font-mono text-xs"
              dir="ltr"
              data-testid="transfer-item"
            >
              {row.original.barcode}
            </span>
          ),
        }),
        createColumn<StockTransferItem>({
          id: "product",
          accessorFn: (item) => `${item.product.name} ${item.product.sku}`,
          labelKey: "transfers.detail.product",
          gridSecondary: true,
          cell: ({ row }) => (
            <span>
              {row.original.product.name}{" "}
              <span
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.product.sku}
              </span>
            </span>
          ),
        }),
        createColumn<StockTransferItem>({
          id: "quantity",
          accessorFn: (item) => itemAmount(item),
          labelKey: "transfers.form.quantity",
          cell: ({ row }) => <span dir="ltr">{itemAmount(row.original)}</span>,
        }),
        createColumn<StockTransferItem>({
          id: "unit_price",
          accessorFn: (item) => decimalOrNone(item.unit_price),
          labelKey: "transfers.detail.unit_price",
          cell: ({ row }) => (
            <span dir="ltr">
              {row.original.unit_price
                ? `${row.original.unit_price} ${currency}`
                : "—"}
            </span>
          ),
        }),
        createColumn<StockTransferItem>({
          id: "line_total",
          accessorFn: (item) => decimalOrNone(item.line_total),
          labelKey: "transfers.detail.line_total",
          defaultHidden: true,
          cell: ({ row }) => (
            <span dir="ltr">
              {row.original.line_total
                ? `${row.original.line_total} ${currency}`
                : "—"}
            </span>
          ),
        }),
        createColumn<StockTransferItem>({
          id: "state",
          accessorFn: (item) => transferItemState(item),
          labelKey: "transfers.detail.stock_state",
          filterVariant: "faceted",
          filterOptions: ITEM_STATES.map((value) => ({
            value,
            label: value,
            labelKey: `transfers.item.${value}`,
          })),
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {t(`transfers.item.${transferItemState(row.original)}`)}
            </span>
          ),
        }),
      ] as ColumnDef<StockTransferItem, unknown>[],
    [currency, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={items}
      getRowId={(item) => item.uuid}
      manual={CLIENT_SIDE_MANUAL}
      features={{
        persistKey: TRANSFER_ITEMS_PERSIST_KEY,
        rowSelection: false,
      }}
    />
  );
}
