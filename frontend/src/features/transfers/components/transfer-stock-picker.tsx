"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Search } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  stockKeys,
  stockService,
  type StockUnitListQuery,
  type StockUnitRow,
} from "@/features/stock/services/stock.service";
import { useDebounce } from "@/hooks/use-debounce";
import { useLocale } from "@/providers/locale-provider";

/** Rows per picker page (the units endpoint caps limit at 100). */
export const TRANSFER_PICKER_PAGE_SIZE = 20;

/**
 * Stock units a transfer may carry: what the stock list shows by default
 * (available and placed); the server rechecks every line on create.
 */
const PICKABLE_STATUSES = "available,placed";

/**
 * Transfer form stock picker (TEC-870): the source organization's units
 * listed with their barcodes, ready to tick into the request instead of
 * scanning. A form-embedded picker (AGENTS §6 "sihirbaz seçicileri"
 * exception), so a compact list over the stock units endpoint with a
 * server-side search (barcode, product name or code) and paging rather
 * than a full EntityTable screen.
 */
export function TransferStockPicker({
  orgUuid,
  picked,
  onToggle,
}: {
  orgUuid: string;
  /** Barcodes already on the request (picked or scanned). */
  picked: ReadonlySet<string>;
  onToggle: (row: StockUnitRow) => void;
}) {
  const { t, format } = useLocale();
  const [search, setSearch] = useState("");
  const [offset, setOffset] = useState(0);
  const q = useDebounce(search.trim(), 300);

  const params: StockUnitListQuery = {
    status: PICKABLE_STATUSES,
    sort: "product",
    limit: TRANSFER_PICKER_PAGE_SIZE,
    offset,
    ...(q ? { q } : {}),
  };
  const list = useQuery({
    queryKey: stockKeys.units(orgUuid, params),
    queryFn: () => stockService.listUnits(orgUuid, params),
    enabled: orgUuid !== "",
    placeholderData: keepPreviousData,
  });
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;

  const available = (r: StockUnitRow) => {
    if (r.unit_kind === "fixed") {
      return t("transfers.picker.pieces", { count: format.number(r.quantity) });
    }
    if (r.remaining_meters != null) {
      return t("transfers.picker.meters", { meters: r.remaining_meters });
    }
    return t("transfers.picker.pieces", { count: format.number(1) });
  };

  return (
    <Card data-testid="transfer-stock-picker">
      <CardHeader className="space-y-1">
        <CardTitle>{t("transfers.picker.title")}</CardTitle>
        <p className="text-muted-foreground text-sm">
          {t("transfers.picker.description")}
        </p>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="relative">
          <Label htmlFor="transfer-stock-search" className="sr-only">
            {t("transfers.picker.search")}
          </Label>
          <Search className="text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2" />
          <Input
            id="transfer-stock-search"
            data-testid="transfer-stock-search"
            className="ps-8"
            type="search"
            autoComplete="off"
            value={search}
            placeholder={t("transfers.picker.search")}
            onChange={(e) => {
              setSearch(e.target.value);
              setOffset(0);
            }}
          />
        </div>

        {list.isError ? (
          <p className="text-destructive text-sm">
            {t("common.error_generic")}
          </p>
        ) : list.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        ) : rows.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="transfer-stock-empty"
          >
            {t(q ? "transfers.picker.no_match" : "transfers.picker.empty")}
          </p>
        ) : (
          <ul
            className="max-h-[28rem] divide-y overflow-y-auto rounded-md border"
            data-testid="transfer-stock-list"
          >
            {rows.map((r) => {
              const checked = picked.has(r.barcode);
              const id = `pick-${r.uuid}`;
              return (
                <li
                  key={r.uuid}
                  data-testid="transfer-stock-row"
                  data-picked={checked ? "true" : "false"}
                  className={
                    checked
                      ? "bg-muted/60 flex items-center gap-3 p-2"
                      : "hover:bg-muted/40 flex items-center gap-3 p-2"
                  }
                >
                  <Checkbox
                    id={id}
                    data-testid="transfer-stock-pick"
                    checked={checked}
                    onCheckedChange={() => onToggle(r)}
                    aria-label={t("transfers.picker.pick", {
                      barcode: r.barcode,
                    })}
                  />
                  <label
                    htmlFor={id}
                    className="grid min-w-0 flex-1 cursor-pointer gap-0.5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:gap-3"
                  >
                    <span className="min-w-0">
                      <span
                        className="block truncate font-mono text-sm"
                        dir="ltr"
                      >
                        {r.barcode}
                      </span>
                      <span className="text-muted-foreground block truncate text-xs">
                        {r.product.name} · {r.product.sku}
                        {r.category ? ` · ${r.category.name}` : ""}
                        {` · ${t(`catalog.unit_types.${r.product.unit_type}`)}`}
                        {r.location ? ` · ${r.location.code}` : ""}
                      </span>
                    </span>
                    <span className="flex items-center gap-2 text-sm">
                      {r.unit_kind === "fixed" ? (
                        <Badge variant="outline">
                          {t("stock.unit_kind.fixed")}
                        </Badge>
                      ) : null}
                      <span
                        className="tabular-nums"
                        data-testid="transfer-stock-available"
                      >
                        {available(r)}
                      </span>
                      {checked ? (
                        <Badge data-testid="transfer-stock-added">
                          {t("transfers.picker.added")}
                        </Badge>
                      ) : null}
                    </span>
                  </label>
                </li>
              );
            })}
          </ul>
        )}

        {total > TRANSFER_PICKER_PAGE_SIZE ? (
          <div className="flex items-center justify-between gap-2 text-sm">
            <span
              className="text-muted-foreground"
              data-testid="transfer-stock-range"
            >
              {t("transfers.picker.range", {
                from: format.number(offset + 1),
                to: format.number(Math.min(offset + rows.length, total)),
                total: format.number(total),
              })}
            </span>
            <div className="flex gap-1">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={offset === 0 || list.isFetching}
                aria-label={t("transfers.picker.prev")}
                onClick={() =>
                  setOffset((o) => Math.max(0, o - TRANSFER_PICKER_PAGE_SIZE))
                }
              >
                <ChevronLeft className="size-4 rtl:rotate-180" />
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                data-testid="transfer-stock-next"
                disabled={
                  offset + TRANSFER_PICKER_PAGE_SIZE >= total || list.isFetching
                }
                aria-label={t("transfers.picker.next")}
                onClick={() => setOffset((o) => o + TRANSFER_PICKER_PAGE_SIZE)}
              >
                <ChevronRight className="size-4 rtl:rotate-180" />
              </Button>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
