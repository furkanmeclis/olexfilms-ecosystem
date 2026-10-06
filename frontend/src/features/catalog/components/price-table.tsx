"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Trash2 } from "lucide-react";
import { useMemo } from "react";

import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  priceNumber,
  visiblePriceColumns,
} from "@/features/catalog/lib/prices";
import type {
  EffectivePrice,
  ProductPriceView,
} from "@/features/catalog/services/pricing.service";
import { useLocale } from "@/providers/locale-provider";

type PriceTableProps = {
  view: ProductPriceView;
  /** Shows edit / delete per currency row (writes go through step-up). */
  canEdit?: boolean;
  onEdit?: (row: EffectivePrice) => void;
  onDelete?: (row: EffectivePrice) => void;
  /** Rows that have something to delete (default: every row). */
  canDelete?: (row: EffectivePrice) => boolean;
};

export const PRICE_TABLE_PERSIST_KEY = "tenant-catalog-product-prices-v1";

/**
 * Effective price view of one product (TEC-146) as a nested client-side
 * DataTable (TEC-370), one row per currency. Columns follow the masked API
 * answer: a field the caller may not see never gets a column (K8).
 */
export function PriceTable({
  view,
  canEdit,
  onEdit,
  onDelete,
  canDelete,
}: PriceTableProps) {
  const { t, format } = useLocale();
  const priceColumns = useMemo(
    () => visiblePriceColumns(view.viewer, view.prices),
    [view.prices, view.viewer],
  );
  const showSource = view.prices.some((p) => p.purchase_price_source);

  const columns = useMemo(() => {
    const cols: ColumnDef<EffectivePrice, unknown>[] = [
      createColumn<EffectivePrice>({
        accessorKey: "currency",
        labelKey: "catalog.prices.currency",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-mono">{row.original.currency}</span>
        ),
      }) as ColumnDef<EffectivePrice, unknown>,
      ...priceColumns.map(
        (col) =>
          createColumn<EffectivePrice>({
            id: col.field,
            accessorFn: (row) => priceNumber(row[col.field]),
            labelKey: col.labelKey,
            enableSorting: true,
            sortUndefined: "last",
            cell: ({ row }) => (
              <span className="tabular-nums" data-price-column={col.field}>
                {format.currency(
                  priceNumber(row.original[col.field]),
                  row.original.currency,
                )}
              </span>
            ),
          }) as ColumnDef<EffectivePrice, unknown>,
      ),
    ];
    if (showSource) {
      cols.push(
        createColumn<EffectivePrice>({
          accessorKey: "purchase_price_source",
          labelKey: "catalog.prices.source",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.purchase_price_source ? (
              <Badge variant="outline">
                {t(
                  `catalog.prices.sources.${row.original.purchase_price_source}`,
                )}
              </Badge>
            ) : (
              "—"
            ),
        }) as ColumnDef<EffectivePrice, unknown>,
      );
    }
    if (canEdit) {
      cols.push(
        createColumn<EffectivePrice>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const actions: EntityRowAction[] = [
              {
                id: "edit",
                label: t("common.edit"),
                icon: Pencil,
                onSelect: () => onEdit?.(row.original),
              },
            ];
            if (!canDelete || canDelete(row.original)) {
              actions.push({
                id: "delete",
                label: t("common.delete"),
                icon: Trash2,
                variant: "destructive",
                onSelect: () => onDelete?.(row.original),
              });
            }
            return <EntityRowActions actions={actions} />;
          },
        }) as ColumnDef<EffectivePrice, unknown>,
      );
    }
    return cols;
  }, [
    canDelete,
    canEdit,
    format,
    onDelete,
    onEdit,
    priceColumns,
    showSource,
    t,
  ]);

  if (!view.prices.length) {
    return (
      <p className="text-muted-foreground text-sm" data-testid="price-empty">
        {t("catalog.prices.empty")}
      </p>
    );
  }

  return (
    <div data-testid="price-table">
      <EntityTable
        columns={columns}
        data={view.prices}
        getRowId={(row) => row.currency}
        manual={CLIENT_SIDE_MANUAL}
        initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
        features={{
          persistKey: PRICE_TABLE_PERSIST_KEY,
          rowSelection: false,
          columnFilters: false,
          facetedFilters: false,
        }}
      />
    </div>
  );
}
