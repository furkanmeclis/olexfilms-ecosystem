"use client";

import type { RowSelectionState } from "@tanstack/react-table";
import { ChartColumn, type LucideIcon } from "lucide-react";
import { useCallback, useMemo, useRef, useState } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { ExportMenu } from "@/features/io/components/export-menu";
import type { ExportFormat } from "@/features/io/types";
import { warehouseExportPaths } from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";

/** Formats of the warehouse exports (TEC-761). */
export const WAREHOUSE_EXPORT_FORMATS: ExportFormat[] = ["xlsx", "csv", "pdf"];

/**
 * Row selection of a warehouse list (row ids are uuids): the export holds
 * only the selected rows when there are any.
 */
export function useExportSelection() {
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const selected = useMemo(
    () => Object.keys(rowSelection).filter((id) => rowSelection[id]),
    [rowSelection],
  );
  return { rowSelection, onRowSelectionChange: setRowSelection, selected };
}

/**
 * Export query of a list: its filter params, `q` and `sort`, plus `uuid`
 * (CSV) of the selected rows.
 */
export function listExportQuery(
  filterParams: Record<string, string | undefined>,
  params: { q?: string | number; sort?: string | number },
  selected: readonly string[] = [],
): Record<string, string | undefined> {
  return {
    ...filterParams,
    q: params.q === undefined ? undefined : String(params.q),
    sort: params.sort === undefined ? undefined : String(params.sort),
    uuid: selected.length > 0 ? selected.join(",") : undefined,
  };
}

/** Export menu of a warehouse screen (XLSX, CSV, PDF; tenant job list). */
export function WarehouseExportMenu({
  slug,
  exportPath,
  query,
  label,
  icon,
  testId,
  disabled,
}: {
  slug: string;
  exportPath: string;
  query?: Record<string, string | undefined>;
  label?: string;
  icon?: LucideIcon;
  testId?: string;
  disabled?: boolean;
}) {
  return (
    <ExportMenu
      exportPath={exportPath}
      query={query}
      formats={WAREHOUSE_EXPORT_FORMATS}
      jobsHref={routes.tenant.exports.root(slug)}
      label={label}
      icon={icon}
      testId={testId}
      disabled={disabled}
    />
  );
}

/**
 * Transfer summary report (TEC-761): per product totals of the transfers
 * the list shows (its filters and selection), optionally for one product.
 */
export function TransferSummaryExport({
  slug,
  query,
}: {
  slug: string;
  query: Record<string, string | undefined>;
}) {
  const { t } = useLocale();
  const [product, setProduct] = useState<ComboboxOption | null>(null);
  const seen = useRef(new Map<string, ComboboxOption>());
  const loadProducts = useCallback(async (q: string) => {
    const page = await catalogService.listProducts({
      ...(q.trim() ? { q: q.trim() } : {}),
      limit: 20,
    });
    const options = page.items.map((p) => ({
      value: p.uuid,
      label: p.name,
      description: p.sku,
    }));
    for (const option of options) seen.current.set(option.value, option);
    return options;
  }, []);

  return (
    <>
      <div className="w-full sm:w-56" data-testid="transfer-summary-product">
        <AsyncCombobox
          id="transfer-summary-product"
          value={product?.value ?? ""}
          options={product ? [product] : undefined}
          clearable
          loadOptions={loadProducts}
          onValueChange={(value) =>
            setProduct(
              value
                ? (seen.current.get(value) ?? { value, label: value })
                : null,
            )
          }
          placeholder={t("warehouse.export.summary_all_products")}
          searchPlaceholder={t("warehouse.export.summary_product_search")}
          emptyText={t("warehouse.export.summary_product_none")}
        />
      </div>
      <WarehouseExportMenu
        slug={slug}
        exportPath={warehouseExportPaths.transferSummary}
        query={{ ...query, product_uuid: product?.value }}
        label={t("warehouse.export.summary")}
        icon={ChartColumn}
        testId="transfer-summary-export"
      />
    </>
  );
}
