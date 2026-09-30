"use client";

import type { Column, Table } from "@tanstack/react-table";
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  EyeOff,
  Pin,
  PinOff,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { DataTableColumnMeta } from "@/components/tables/types";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

type ColumnHeaderProps<TData> = {
  column: Column<TData, unknown>;
  table: Table<TData>;
  title?: string;
  className?: string;
  /** Hide sort controls (e.g. while DnD reorder mode is active) */
  disableSort?: boolean;
};

export function ColumnHeader<TData>({
  column,
  table,
  title,
  className,
  disableSort,
}: ColumnHeaderProps<TData>) {
  const { t } = useLocale();
  const meta = column.columnDef.meta as DataTableColumnMeta | undefined;
  const label =
    title ??
    (meta?.labelKey ? t(meta.labelKey) : undefined) ??
    meta?.label ??
    column.id;

  if (disableSort) {
    return <div className={cn("text-sm font-medium", className)}>{label}</div>;
  }

  const order = table.getState().columnOrder;
  const allIds = order.length
    ? order
    : table.getAllLeafColumns().map((c) => c.id);
  const index = allIds.indexOf(column.id);

  const move = (dir: -1 | 1) => {
    if (index < 0) return;
    const next = [...allIds];
    const target = index + dir;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    table.setColumnOrder(next);
  };

  if (!column.getCanSort() && !column.getCanHide() && !column.getCanPin()) {
    return <div className={cn("text-sm font-medium", className)}>{label}</div>;
  }

  return (
    <div className={cn("flex items-center gap-1", className)}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="data-[state=open]:bg-accent -ms-2 h-8"
          >
            <span>{label}</span>
            {column.getIsSorted() === "desc" ? (
              <ArrowDown className="size-3.5" />
            ) : column.getIsSorted() === "asc" ? (
              <ArrowUp className="size-3.5" />
            ) : column.getCanSort() ? (
              <ArrowUpDown className="size-3.5 opacity-50" />
            ) : null}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          {column.getCanSort() ? (
            <>
              <DropdownMenuItem onClick={() => column.toggleSorting(false)}>
                <ArrowUp className="size-3.5" />
                {t("table.sort_asc")}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => column.toggleSorting(true)}>
                <ArrowDown className="size-3.5" />
                {t("table.sort_desc")}
              </DropdownMenuItem>
              {column.getIsSorted() ? (
                <DropdownMenuItem onClick={() => column.clearSorting()}>
                  {t("table.sort_clear")}
                </DropdownMenuItem>
              ) : null}
              <DropdownMenuSeparator />
            </>
          ) : null}

          <DropdownMenuItem onClick={() => move(-1)} disabled={index <= 0}>
            <ChevronLeft className="size-3.5" />
            {t("table.move_left")}
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => move(1)}
            disabled={index < 0 || index >= allIds.length - 1}
          >
            <ChevronRight className="size-3.5" />
            {t("table.move_right")}
          </DropdownMenuItem>
          <DropdownMenuSeparator />

          {column.getCanPin() ? (
            <>
              <DropdownMenuItem
                onClick={() => column.pin("left")}
                disabled={column.getIsPinned() === "left"}
              >
                <Pin className="size-3.5" />
                {t("table.pin_left")}
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => column.pin("right")}
                disabled={column.getIsPinned() === "right"}
              >
                <Pin className="size-3.5" />
                {t("table.pin_right")}
              </DropdownMenuItem>
              {column.getIsPinned() ? (
                <DropdownMenuItem onClick={() => column.pin(false)}>
                  <PinOff className="size-3.5" />
                  {t("table.unpin")}
                </DropdownMenuItem>
              ) : null}
              <DropdownMenuSeparator />
            </>
          ) : null}

          {column.getCanHide() ? (
            <DropdownMenuItem onClick={() => column.toggleVisibility(false)}>
              <EyeOff className="size-3.5" />
              {t("table.hide_column")}
            </DropdownMenuItem>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
