"use client";

import {
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type ColumnFiltersState,
  type ColumnOrderState,
  type ColumnPinningState,
  type ColumnSizingState,
  type PaginationState,
  type RowSelectionState,
  type SortingState,
  type VisibilityState,
} from "@tanstack/react-table";
import { useEffect, useMemo, useState } from "react";

import type {
  DataTableFeatures,
  DataTableState,
  TableDensity,
  TableViewMode,
  UseDataTableOptions,
} from "@/components/tables/types";

const DEFAULT_FEATURES: Required<Omit<DataTableFeatures, "persistKey">> & {
  persistKey?: string;
} = {
  sorting: true,
  multiSort: false,
  globalFilter: true,
  columnFilters: true,
  columnVisibility: true,
  columnOrdering: true,
  columnPinning: true,
  columnResizing: true,
  rowSelection: true,
  pagination: true,
  density: true,
  viewMode: true,
  mobileAutoCards: true,
  rowReorder: false,
  inlineEdit: false,
  facetedFilters: true,
};

/**
 * Read the UI state DataTable persisted under `persistKey`
 * (`features.persistKey`). Returns null on SSR / missing / invalid JSON.
 */
export function readPersistedTableState(
  key: string | undefined,
): Partial<DataTableState> | null {
  if (!key || typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as Partial<DataTableState>) : null;
  } catch {
    return null;
  }
}

function writePersisted(
  key: string | undefined,
  state: Partial<DataTableState>,
) {
  if (!key || typeof window === "undefined") return;
  try {
    window.localStorage.setItem(key, JSON.stringify(state));
  } catch {
    // ignore quota
  }
}

/** Layout-only subset persisted for server-mode tables. */
function pickLayoutState(
  state: Partial<DataTableState> | null,
): Partial<DataTableState> | null {
  if (!state) return null;
  const {
    columnVisibility,
    columnOrder,
    columnPinning,
    columnSizing,
    density,
    viewMode,
    pagination,
  } = state;
  return {
    columnVisibility,
    columnOrder,
    columnPinning,
    columnSizing,
    density,
    viewMode,
    pagination: pagination
      ? { pageIndex: 0, pageSize: pagination.pageSize }
      : undefined,
  };
}

function defaultHiddenVisibility<TData>(
  columns: ColumnDef<TData, unknown>[],
): VisibilityState {
  const visibility: VisibilityState = {};
  for (const col of columns) {
    const id =
      ("id" in col && col.id) ||
      ("accessorKey" in col && String(col.accessorKey));
    if (id && col.meta?.defaultHidden) {
      visibility[id] = false;
    }
  }
  return visibility;
}

export function useDataTable<TData>(options: UseDataTableOptions<TData>) {
  const {
    columns,
    data,
    getRowId,
    features: featuresProp,
    manual,
    state: controlled,
    initialState,
    pageCount,
    rowCount,
  } = options;

  const features = { ...DEFAULT_FEATURES, ...featuresProp };
  // Server mode: sort / filters / page are owned by the server-list state
  // (useServerListState); only layout preferences are persisted here.
  const serverMode = Boolean(
    manual?.pagination || manual?.sorting || manual?.filtering,
  );
  const persistedRaw = useMemo(
    () => readPersistedTableState(features.persistKey),
    [features.persistKey],
  );
  const persisted = useMemo(
    () => (serverMode ? pickLayoutState(persistedRaw) : persistedRaw),
    [persistedRaw, serverMode],
  );

  const [sorting, setSorting] = useState<SortingState>(
    controlled?.sorting ?? persisted?.sorting ?? initialState?.sorting ?? [],
  );
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(
    controlled?.columnFilters ??
      persisted?.columnFilters ??
      initialState?.columnFilters ??
      [],
  );
  const [globalFilter, setGlobalFilter] = useState(
    controlled?.globalFilter ??
      persisted?.globalFilter ??
      initialState?.globalFilter ??
      "",
  );
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>(
    controlled?.columnVisibility ??
      persisted?.columnVisibility ??
      initialState?.columnVisibility ??
      defaultHiddenVisibility(columns),
  );
  const [columnOrder, setColumnOrder] = useState<ColumnOrderState>(
    controlled?.columnOrder ??
      persisted?.columnOrder ??
      initialState?.columnOrder ??
      [],
  );
  const [columnPinning, setColumnPinning] = useState<ColumnPinningState>(
    controlled?.columnPinning ??
      persisted?.columnPinning ??
      initialState?.columnPinning ??
      {},
  );
  const [columnSizing, setColumnSizing] = useState<ColumnSizingState>(
    controlled?.columnSizing ??
      persisted?.columnSizing ??
      initialState?.columnSizing ??
      {},
  );
  const [rowSelection, setRowSelection] = useState<RowSelectionState>(
    controlled?.rowSelection ??
      persisted?.rowSelection ??
      initialState?.rowSelection ??
      {},
  );
  const [pagination, setPagination] = useState<PaginationState>(
    controlled?.pagination ??
      persisted?.pagination ??
      initialState?.pagination ?? { pageIndex: 0, pageSize: 10 },
  );
  const [density, setDensity] = useState<TableDensity>(
    controlled?.density ??
      persisted?.density ??
      initialState?.density ??
      "comfortable",
  );
  const [viewMode, setViewMode] = useState<TableViewMode>(
    controlled?.viewMode ??
      persisted?.viewMode ??
      initialState?.viewMode ??
      "table",
  );

  // Sync controlled props inward
  useEffect(() => {
    if (controlled?.sorting) setSorting(controlled.sorting);
  }, [controlled?.sorting]);
  useEffect(() => {
    if (controlled?.columnFilters) setColumnFilters(controlled.columnFilters);
  }, [controlled?.columnFilters]);
  useEffect(() => {
    if (controlled?.globalFilter !== undefined) {
      setGlobalFilter(controlled.globalFilter);
    }
  }, [controlled?.globalFilter]);

  const effectiveVisibility = controlled?.columnVisibility ?? columnVisibility;
  const effectiveOrder = controlled?.columnOrder ?? columnOrder;
  const effectivePinning = controlled?.columnPinning ?? columnPinning;
  const effectiveSizing = controlled?.columnSizing ?? columnSizing;
  const effectivePagination = controlled?.pagination ?? pagination;
  const effectiveDensity = controlled?.density ?? density;
  const effectiveViewMode = controlled?.viewMode ?? viewMode;
  const effectivePageSize = effectivePagination.pageSize;

  useEffect(() => {
    const layout = {
      columnVisibility: effectiveVisibility,
      columnOrder: effectiveOrder,
      columnPinning: effectivePinning,
      columnSizing: effectiveSizing,
      density: effectiveDensity,
      viewMode: effectiveViewMode,
    };
    writePersisted(
      features.persistKey,
      serverMode
        ? {
            ...layout,
            pagination: { pageIndex: 0, pageSize: effectivePageSize },
          }
        : {
            ...layout,
            sorting,
            columnFilters,
            globalFilter,
            pagination,
          },
    );
  }, [
    features.persistKey,
    serverMode,
    sorting,
    columnFilters,
    globalFilter,
    pagination,
    effectiveVisibility,
    effectiveOrder,
    effectivePinning,
    effectiveSizing,
    effectiveDensity,
    effectiveViewMode,
    effectivePageSize,
  ]);

  // TanStack Table returns functions that React Compiler cannot memoize safely.
  // eslint-disable-next-line react-hooks/incompatible-library -- library API
  const table = useReactTable({
    data,
    columns,
    pageCount,
    rowCount,
    getRowId,
    state: {
      sorting: controlled?.sorting ?? sorting,
      columnFilters: controlled?.columnFilters ?? columnFilters,
      globalFilter: controlled?.globalFilter ?? globalFilter,
      columnVisibility: controlled?.columnVisibility ?? columnVisibility,
      columnOrder: controlled?.columnOrder ?? columnOrder,
      columnPinning: controlled?.columnPinning ?? columnPinning,
      columnSizing: controlled?.columnSizing ?? columnSizing,
      rowSelection: controlled?.rowSelection ?? rowSelection,
      pagination: controlled?.pagination ?? pagination,
    },
    onSortingChange: controlled?.onSortingChange ?? setSorting,
    onColumnFiltersChange:
      controlled?.onColumnFiltersChange ?? setColumnFilters,
    onGlobalFilterChange: controlled?.onGlobalFilterChange ?? setGlobalFilter,
    onColumnVisibilityChange:
      controlled?.onColumnVisibilityChange ?? setColumnVisibility,
    onColumnOrderChange: controlled?.onColumnOrderChange ?? setColumnOrder,
    onColumnPinningChange:
      controlled?.onColumnPinningChange ?? setColumnPinning,
    onColumnSizingChange: controlled?.onColumnSizingChange ?? setColumnSizing,
    onRowSelectionChange: controlled?.onRowSelectionChange ?? setRowSelection,
    onPaginationChange: controlled?.onPaginationChange ?? setPagination,

    enableSorting: features.sorting,
    enableMultiSort: features.multiSort,
    enableSortingRemoval: true,
    enableFilters: features.columnFilters || features.globalFilter,
    enableColumnFilters: features.columnFilters,
    enableGlobalFilter: features.globalFilter,
    enableRowSelection: features.rowSelection,
    enableColumnResizing: features.columnResizing,
    columnResizeMode: "onChange",
    enableColumnPinning: features.columnPinning,

    manualSorting: manual?.sorting,
    manualFiltering: manual?.filtering,
    manualPagination: manual?.pagination,

    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: features.sorting ? getSortedRowModel() : undefined,
    getFilteredRowModel:
      features.columnFilters || features.globalFilter
        ? getFilteredRowModel()
        : undefined,
    getPaginationRowModel: features.pagination
      ? getPaginationRowModel()
      : undefined,
    getFacetedRowModel: features.columnFilters
      ? getFacetedRowModel()
      : undefined,
    getFacetedUniqueValues: features.columnFilters
      ? getFacetedUniqueValues()
      : undefined,

    globalFilterFn: "includesString",
  });

  const resetAll = () => {
    table.resetSorting();
    table.resetColumnFilters();
    table.resetGlobalFilter();
    table.resetColumnVisibility();
    table.resetColumnOrder();
    table.resetColumnPinning();
    table.resetColumnSizing();
    table.resetRowSelection();
    table.resetPagination();
    setDensity(initialState?.density ?? "comfortable");
    setViewMode(initialState?.viewMode ?? "table");
    controlled?.onDensityChange?.(initialState?.density ?? "comfortable");
    controlled?.onViewModeChange?.(initialState?.viewMode ?? "table");
  };

  return {
    table,
    features,
    density: controlled?.density ?? density,
    setDensity: controlled?.onDensityChange ?? setDensity,
    viewMode: controlled?.viewMode ?? viewMode,
    setViewMode: controlled?.onViewModeChange ?? setViewMode,
    resetAll,
  };
}
