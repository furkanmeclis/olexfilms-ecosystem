"use client";

import {
  flexRender,
  type Cell,
  type Row,
  type Table,
} from "@tanstack/react-table";
import type { ReactNode } from "react";

import type { DataTableColumnMeta } from "@/components/tables/types";
import { isRowClickIgnored } from "@/components/tables/row-click";
import { Checkbox } from "@/components/ui/checkbox";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

type GridViewProps<TData> = {
  table: Table<TData>;
  renderGridItem?: (row: TData, table: Table<TData>) => ReactNode;
  /** Tighter single-column cards for phone layouts */
  mobile?: boolean;
  onRowClick?: (row: TData) => void;
};

function isUtilityColumn(id: string) {
  return id === "__select" || id === "actions";
}

function CardShell({
  selected,
  mobile,
  children,
  onToggleSelect,
  selectable,
  onClick,
}: {
  selected: boolean;
  mobile?: boolean;
  children: ReactNode;
  selectable?: boolean;
  onToggleSelect?: (checked: boolean) => void;
  onClick?: (event: React.MouseEvent<HTMLElement>) => void;
}) {
  const { t } = useLocale();
  return (
    <article
      className={cn(
        "border-border bg-card text-card-foreground relative overflow-hidden rounded-xl border shadow-sm transition-colors",
        "hover:border-primary/25",
        selected && "border-primary/50 bg-primary/5 shadow-md",
        mobile ? "p-4" : "p-4",
        onClick && "cursor-pointer",
      )}
      onClick={onClick}
    >
      {selectable ? (
        <div className="absolute end-3 top-3 z-10">
          <Checkbox
            checked={selected}
            onCheckedChange={(value) => onToggleSelect?.(Boolean(value))}
            aria-label={t("table.select_row")}
            className="border-muted-foreground/40 size-5"
          />
        </div>
      ) : null}
      {children}
    </article>
  );
}

function DefaultCard<TData>({
  row,
  table,
  mobile,
  onRowClick,
}: {
  row: Row<TData>;
  table: Table<TData>;
  mobile?: boolean;
  onRowClick?: (row: TData) => void;
}) {
  const { t } = useLocale();
  const cells = row
    .getVisibleCells()
    .filter((c) => !isUtilityColumn(c.column.id));
  const actionsCell = row
    .getVisibleCells()
    .find((c) => c.column.id === "actions");

  const primary =
    cells.find(
      (c) => (c.column.columnDef.meta as DataTableColumnMeta)?.gridPrimary,
    ) ?? cells[0];
  const secondary = cells.find(
    (c) =>
      c.id !== primary?.id &&
      (c.column.columnDef.meta as DataTableColumnMeta)?.gridSecondary,
  );
  const rest = cells.filter(
    (c) => c.id !== primary?.id && c.id !== secondary?.id,
  );

  return (
    <CardShell
      selected={row.getIsSelected()}
      mobile={mobile}
      selectable={Boolean(table.options.enableRowSelection)}
      onToggleSelect={(checked) => row.toggleSelected(checked)}
      onClick={
        onRowClick
          ? (event) => {
              if (isRowClickIgnored(event)) return;
              onRowClick(row.original);
            }
          : undefined
      }
    >
      <div
        className={cn("space-y-3", table.options.enableRowSelection && "pe-8")}
      >
        <header className="space-y-1">
          {primary ? (
            <h3 className="font-display text-base leading-snug font-semibold tracking-tight">
              {flexRender(primary.column.columnDef.cell, primary.getContext())}
            </h3>
          ) : null}
          {secondary ? (
            <p className="text-muted-foreground text-sm">
              {flexRender(
                secondary.column.columnDef.cell,
                secondary.getContext(),
              )}
            </p>
          ) : null}
        </header>

        {rest.length > 0 ? (
          <dl
            className={cn(
              "divide-border/80 bg-muted/40 divide-y rounded-lg",
              mobile ? "px-3" : "px-3",
            )}
          >
            {rest.map((cell: Cell<TData, unknown>) => {
              const meta = cell.column.columnDef.meta as
                DataTableColumnMeta | undefined;
              return (
                <div
                  key={cell.id}
                  className="flex items-center justify-between gap-3 py-2.5 text-sm"
                >
                  <dt className="text-muted-foreground shrink-0">
                    {meta?.labelKey
                      ? t(meta.labelKey)
                      : (meta?.label ?? cell.column.id)}
                  </dt>
                  <dd className="min-w-0 text-end font-medium">
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </dd>
                </div>
              );
            })}
          </dl>
        ) : null}

        {actionsCell ? (
          <footer className="border-border/60 flex items-center justify-end border-t pt-3 empty:hidden">
            {flexRender(
              actionsCell.column.columnDef.cell,
              actionsCell.getContext(),
            )}
          </footer>
        ) : null}
      </div>
    </CardShell>
  );
}

export function GridView<TData>({
  table,
  renderGridItem,
  mobile,
  onRowClick,
}: GridViewProps<TData>) {
  const rows = table.getRowModel().rows;

  return (
    <div
      className={cn(
        "grid gap-3",
        mobile ? "grid-cols-1" : "grid-cols-1 sm:grid-cols-2 xl:grid-cols-3",
      )}
    >
      {rows.map((row) => {
        if (renderGridItem) {
          return (
            <CardShell
              key={row.id}
              selected={row.getIsSelected()}
              mobile={mobile}
              selectable={Boolean(table.options.enableRowSelection)}
              onToggleSelect={(checked) => row.toggleSelected(checked)}
              onClick={
                onRowClick
                  ? (event) => {
                      if (isRowClickIgnored(event)) return;
                      onRowClick(row.original);
                    }
                  : undefined
              }
            >
              <div className={cn(table.options.enableRowSelection && "pe-8")}>
                {renderGridItem(row.original, table)}
              </div>
            </CardShell>
          );
        }

        return (
          <DefaultCard
            key={row.id}
            row={row}
            table={table}
            mobile={mobile}
            onRowClick={onRowClick}
          />
        );
      })}
    </div>
  );
}
