"use client";

import type { Table } from "@tanstack/react-table";
import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

type DataTablePaginationProps<TData> = {
  table: Table<TData>;
  pageSizeOptions?: number[];
  compact?: boolean;
};

export function DataTablePagination<TData>({
  table,
  pageSizeOptions = [10, 20, 30, 50, 100],
  compact,
}: DataTablePaginationProps<TData>) {
  const { t } = useLocale();
  const { pageIndex, pageSize } = table.getState().pagination;
  const total = table.getFilteredRowModel().rows.length;
  const from = total === 0 ? 0 : pageIndex * pageSize + 1;
  const to = Math.min(total, (pageIndex + 1) * pageSize);

  return (
    <div
      className={cn(
        "flex gap-3",
        compact
          ? "border-border bg-card flex-col rounded-xl border p-3"
          : "flex-wrap items-center justify-between",
      )}
    >
      <p className="text-muted-foreground text-xs">
        {t("table.showing", { from, to, total })}
      </p>

      <div
        className={cn("flex items-center gap-2", compact && "justify-between")}
      >
        <div className="flex items-center gap-2">
          {!compact ? (
            <span className="text-muted-foreground text-xs">
              {t("table.rows_per_page")}
            </span>
          ) : null}
          <Select
            value={String(pageSize)}
            onValueChange={(value) => table.setPageSize(Number(value))}
          >
            <SelectTrigger className="h-8 w-[72px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {pageSizeOptions.map((size) => (
                <SelectItem key={size} value={String(size)}>
                  {size}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <span className="text-muted-foreground text-xs">
          {pageIndex + 1}/{Math.max(table.getPageCount(), 1)}
        </span>

        <div className="flex items-center gap-1">
          {!compact ? (
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="size-8"
              onClick={() => table.setPageIndex(0)}
              disabled={!table.getCanPreviousPage()}
              aria-label={t("table.go_first")}
            >
              <ChevronsLeft className="size-4" />
            </Button>
          ) : null}
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="size-8"
            onClick={() => table.previousPage()}
            disabled={!table.getCanPreviousPage()}
            aria-label={t("table.go_prev")}
          >
            <ChevronLeft className="size-4" />
          </Button>
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="size-8"
            onClick={() => table.nextPage()}
            disabled={!table.getCanNextPage()}
            aria-label={t("table.go_next")}
          >
            <ChevronRight className="size-4" />
          </Button>
          {!compact ? (
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="size-8"
              onClick={() => table.setPageIndex(table.getPageCount() - 1)}
              disabled={!table.getCanNextPage()}
              aria-label={t("table.go_last")}
            >
              <ChevronsRight className="size-4" />
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
