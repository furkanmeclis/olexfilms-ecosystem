"use client";

import { Pencil, Trash2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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

/**
 * Effective price view of one product (TEC-146). Columns follow the masked
 * API answer: a field the caller may not see never gets a column (K8).
 */
export function PriceTable({
  view,
  canEdit,
  onEdit,
  onDelete,
  canDelete,
}: PriceTableProps) {
  const { t, format } = useLocale();
  const columns = visiblePriceColumns(view.viewer, view.prices);
  const showSource = view.prices.some((p) => p.purchase_price_source);

  if (!view.prices.length) {
    return (
      <p className="text-muted-foreground text-sm" data-testid="price-empty">
        {t("catalog.prices.empty")}
      </p>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm" data-testid="price-table">
        <thead>
          <tr className="text-muted-foreground border-b text-start">
            <th className="py-2 pe-4 text-start font-medium">
              {t("catalog.prices.currency")}
            </th>
            {columns.map((col) => (
              <th
                key={col.field}
                className="py-2 pe-4 text-end font-medium"
                data-price-column={col.field}
              >
                {t(col.labelKey)}
              </th>
            ))}
            {showSource ? (
              <th className="py-2 pe-4 text-start font-medium">
                {t("catalog.prices.source")}
              </th>
            ) : null}
            {canEdit ? (
              <th className="py-2 text-end font-medium">
                <span className="sr-only">{t("common.actions")}</span>
              </th>
            ) : null}
          </tr>
        </thead>
        <tbody>
          {view.prices.map((row) => (
            <tr key={row.currency} className="border-b last:border-0">
              <td className="py-2 pe-4 font-mono">{row.currency}</td>
              {columns.map((col) => (
                <td key={col.field} className="py-2 pe-4 text-end tabular-nums">
                  {format.currency(priceNumber(row[col.field]), row.currency)}
                </td>
              ))}
              {showSource ? (
                <td className="py-2 pe-4">
                  {row.purchase_price_source ? (
                    <Badge variant="outline">
                      {t(`catalog.prices.sources.${row.purchase_price_source}`)}
                    </Badge>
                  ) : (
                    "—"
                  )}
                </td>
              ) : null}
              {canEdit ? (
                <td className="py-2 text-end whitespace-nowrap">
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="size-8"
                    aria-label={t("common.edit")}
                    onClick={() => onEdit?.(row)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  {!canDelete || canDelete(row) ? (
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      className="text-destructive size-8"
                      aria-label={t("common.delete")}
                      onClick={() => onDelete?.(row)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  ) : null}
                </td>
              ) : null}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
