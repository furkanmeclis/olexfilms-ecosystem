"use client";

import { useMemo, useState, type ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { DataTablePagination } from "@/components/tables/data-table-pagination";
import { DataTableToolbar } from "@/components/tables/data-table-toolbar";
import { GridView } from "@/components/tables/grid-view";
import { createReorderColumnDef } from "@/components/tables/reorder-column";
import { reorderRowsById } from "@/components/tables/reorder-rows";
import { TableView } from "@/components/tables/table-view";
import type { DataTableProps } from "@/components/tables/types";
import { useDataTable } from "@/components/tables/use-data-table";
import { Skeleton } from "@/components/ui/skeleton";
import { useIsMobile } from "@/hooks/use-mobile";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export function DataTable<TData>({
  columns,
  data,
  getRowId,
  isLoading,
  emptyTitle,
  emptyDescription,
  features,
  manual,
  state,
  initialState,
  pageCount,
  rowCount,
  pageSizeOptions,
  toolbar,
  bulkActions,
  renderGridItem,
  onRowReorder,
  onCellEdit,
  onExportReady,
  onRowClick,
  className,
}: DataTableProps<TData>) {
  const { t } = useLocale();
  const isMobile = useIsMobile();
  const [reorderModeRequested, setReorderModeRequested] = useState(false);
  const [editingCell, setEditingCell] = useState<{
    rowId: string;
    columnId: string;
  } | null>(null);

  const resolvedGetRowId =
    getRowId ??
    ((row: TData, index: number) => {
      const maybe = row as { id?: string | number };
      return maybe.id != null ? String(maybe.id) : String(index);
    });

  const forceMobileCardsPreview =
    isMobile && features?.mobileAutoCards !== false;
  const canReorder =
    Boolean(features?.rowReorder) &&
    Boolean(onRowReorder) &&
    !forceMobileCardsPreview;
  const reorderMode = canReorder && reorderModeRequested;

  const columnsWithReorder = useMemo(() => {
    if (!reorderMode) return columns;
    return [createReorderColumnDef<TData>(), ...columns];
  }, [columns, reorderMode]);

  const {
    table,
    features: resolvedFeatures,
    density,
    setDensity,
    viewMode,
    setViewMode,
    resetAll,
  } = useDataTable({
    columns: columnsWithReorder,
    data,
    getRowId: resolvedGetRowId,
    features,
    manual,
    state,
    initialState,
    pageCount,
    rowCount,
  });

  const forceMobileCards =
    isMobile && resolvedFeatures.mobileAutoCards !== false;

  const displayMode = reorderMode
    ? "table"
    : forceMobileCards
      ? "grid"
      : viewMode;

  const activeEditingCell = reorderMode ? null : editingCell;

  const toolbarNode: ReactNode =
    typeof toolbar === "function" ? toolbar(table) : toolbar;

  if (isLoading) {
    return (
      <div className={cn("space-y-3", className)}>
        <Skeleton className="h-9 w-full max-w-sm" />
        {forceMobileCards || displayMode === "grid" ? (
          <div className="grid grid-cols-1 gap-3">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-36 w-full rounded-xl" />
            ))}
          </div>
        ) : (
          <div className="space-y-2">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        )}
      </div>
    );
  }

  const filteredCount = table.getFilteredRowModel().rows.length;
  const hasRows = table.getRowModel().rows.length > 0;
  const columnCount = table.getVisibleLeafColumns().length;

  const emptyMessage = !data.length
    ? {
        title: emptyTitle ?? t("common.empty_title"),
        description: emptyDescription ?? t("common.empty_description"),
      }
    : !filteredCount
      ? {
          title: t("table.no_results"),
          description: t("table.reset_filters"),
        }
      : null;

  return (
    <div className={cn("space-y-3", className)}>
      <DataTableToolbar
        table={table}
        features={resolvedFeatures}
        density={density}
        setDensity={setDensity}
        viewMode={viewMode}
        setViewMode={setViewMode}
        resetAll={resetAll}
        toolbar={toolbarNode}
        bulkActions={bulkActions}
        forceMobileCards={forceMobileCards}
        hideFacetCounts={Boolean(manual?.filtering)}
        reorderMode={reorderMode}
        onReorderModeChange={
          canReorder
            ? (next) => {
                if (next) {
                  setViewMode("table");
                  table.resetSorting();
                  setEditingCell(null);
                }
                setReorderModeRequested(next);
              }
            : undefined
        }
        onExport={
          onExportReady
            ? () =>
                onExportReady(
                  table.getFilteredRowModel().rows.map((r) => r.original),
                  table,
                )
            : undefined
        }
      />

      {reorderMode ? (
        <p className="text-muted-foreground text-xs">
          {t("table.reorder_hint")}
        </p>
      ) : null}

      {displayMode === "grid" ? (
        emptyMessage && !hasRows ? (
          <div className="space-y-3">
            <EmptyState
              title={emptyMessage.title}
              description={emptyMessage.description}
            />
          </div>
        ) : (
          <GridView
            table={table}
            renderGridItem={renderGridItem}
            mobile={forceMobileCards}
            onRowClick={reorderMode ? undefined : onRowClick}
          />
        )
      ) : (
        <TableView
          table={table}
          density={density}
          showColumnFilters={
            resolvedFeatures.columnFilters && !isMobile && !reorderMode
          }
          emptyMessage={emptyMessage}
          columnCount={columnCount}
          reorderMode={reorderMode}
          onReorder={(activeId, overId) => {
            if (!onRowReorder) return;
            onRowReorder(
              reorderRowsById(data, resolvedGetRowId, activeId, overId),
            );
          }}
          inlineEdit={
            Boolean(resolvedFeatures.inlineEdit) &&
            Boolean(onCellEdit) &&
            !reorderMode
          }
          editingCell={activeEditingCell}
          onEditingCellChange={setEditingCell}
          onCellEdit={onCellEdit}
          onRowClick={reorderMode ? undefined : onRowClick}
        />
      )}

      {resolvedFeatures.pagination && hasRows && !reorderMode ? (
        <DataTablePagination
          table={table}
          pageSizeOptions={pageSizeOptions}
          compact={forceMobileCards}
        />
      ) : null}
    </div>
  );
}
