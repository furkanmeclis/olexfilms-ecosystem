import type {
  ColumnDef,
  ColumnFiltersState,
  ColumnOrderState,
  ColumnPinningState,
  ColumnSizingState,
  OnChangeFn,
  PaginationState,
  RowSelectionState,
  SortingState,
  Table,
  VisibilityState,
} from "@tanstack/react-table";
import type { ReactNode } from "react";

import type { DataTableBulkAction } from "@/components/tables/bulk-actions";

/** Density affects row/cell padding */
export type TableDensity = "compact" | "comfortable" | "spacious";

/** Primary presentation mode */
export type TableViewMode = "table" | "grid";

/** Column filter UI kinds */
export type ColumnFilterVariant =
  | "text"
  | "select"
  | "multi-select"
  | "faceted"
  | "number-range"
  | "boolean"
  | "date-range";

/** Inline cell editor kinds (double-click) */
export type ColumnEditVariant = "text" | "number" | "select" | "boolean";

export type DataTableColumnMeta = {
  /** i18n key for header label — preferred over raw header string */
  labelKey?: string;
  /** Fallback label if labelKey missing */
  label?: string;
  /** Show per-column filter UI */
  filterVariant?: ColumnFilterVariant;
  /** Options for select / multi-select filters */
  filterOptions?: { label: string; labelKey?: string; value: string }[];
  /** Enable double-click inline editing with this editor */
  editVariant?: ColumnEditVariant;
  /** Options for select edit editor */
  editOptions?: { label: string; value: string }[];
  /** Hide from column visibility menu */
  enableHiding?: boolean;
  /** Default hidden */
  defaultHidden?: boolean;
  /** Grid card emphasis */
  gridPrimary?: boolean;
  /** Grid card secondary line */
  gridSecondary?: boolean;
  /** Class overrides */
  headerClassName?: string;
  cellClassName?: string;
};

declare module "@tanstack/react-table" {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  interface ColumnMeta<TData, TValue> {
    labelKey?: string;
    label?: string;
    filterVariant?: ColumnFilterVariant;
    filterOptions?: { label: string; labelKey?: string; value: string }[];
    editVariant?: ColumnEditVariant;
    editOptions?: { label: string; value: string }[];
    enableHiding?: boolean;
    defaultHidden?: boolean;
    gridPrimary?: boolean;
    gridSecondary?: boolean;
    headerClassName?: string;
    cellClassName?: string;
  }
}

export type DataTableState = {
  sorting: SortingState;
  columnFilters: ColumnFiltersState;
  globalFilter: string;
  columnVisibility: VisibilityState;
  columnOrder: ColumnOrderState;
  columnPinning: ColumnPinningState;
  columnSizing: ColumnSizingState;
  rowSelection: RowSelectionState;
  pagination: PaginationState;
  density: TableDensity;
  viewMode: TableViewMode;
};

export type DataTableControlledState = Partial<{
  sorting: SortingState;
  onSortingChange: OnChangeFn<SortingState>;
  columnFilters: ColumnFiltersState;
  onColumnFiltersChange: OnChangeFn<ColumnFiltersState>;
  globalFilter: string;
  onGlobalFilterChange: OnChangeFn<string>;
  columnVisibility: VisibilityState;
  onColumnVisibilityChange: OnChangeFn<VisibilityState>;
  columnOrder: ColumnOrderState;
  onColumnOrderChange: OnChangeFn<ColumnOrderState>;
  columnPinning: ColumnPinningState;
  onColumnPinningChange: OnChangeFn<ColumnPinningState>;
  columnSizing: ColumnSizingState;
  onColumnSizingChange: OnChangeFn<ColumnSizingState>;
  rowSelection: RowSelectionState;
  onRowSelectionChange: OnChangeFn<RowSelectionState>;
  pagination: PaginationState;
  onPaginationChange: OnChangeFn<PaginationState>;
  density: TableDensity;
  onDensityChange: (density: TableDensity) => void;
  viewMode: TableViewMode;
  onViewModeChange: (mode: TableViewMode) => void;
}>;

export type DataTableFeatures = {
  sorting?: boolean;
  multiSort?: boolean;
  globalFilter?: boolean;
  columnFilters?: boolean;
  columnVisibility?: boolean;
  columnOrdering?: boolean;
  columnPinning?: boolean;
  columnResizing?: boolean;
  rowSelection?: boolean;
  pagination?: boolean;
  density?: boolean;
  viewMode?: boolean;
  /**
   * Below the md breakpoint, force card layout regardless of viewMode.
   * Default: true
   */
  mobileAutoCards?: boolean;
  /**
   * Opt-in row drag-and-drop reorder (table/list view only).
   * Shows a toolbar toggle; default off. Requires `onRowReorder`.
   * Default: false
   */
  rowReorder?: boolean;
  /**
   * Opt-in double-click inline cell editing for columns with `editVariant`.
   * Default: false
   */
  inlineEdit?: boolean;
  /**
   * Toolbar faceted filters for columns with `filterVariant: "faceted"`.
   * Default: true
   */
  facetedFilters?: boolean;
  /** Persist table UI state to localStorage under this key */
  persistKey?: string;
};

export type DataTableManual = {
  sorting?: boolean;
  filtering?: boolean;
  pagination?: boolean;
};

export type DataTableProps<TData> = {
  columns: ColumnDef<TData, unknown>[];
  data: TData[];
  getRowId?: (originalRow: TData, index: number) => string;

  isLoading?: boolean;
  emptyTitle?: string;
  emptyDescription?: string;

  features?: DataTableFeatures;
  manual?: DataTableManual;
  state?: DataTableControlledState;
  initialState?: Partial<DataTableState>;

  pageCount?: number;
  pageSizeOptions?: number[];

  toolbar?: ReactNode | ((table: Table<TData>) => ReactNode);
  /**
   * Bulk actions for selected rows.
   * - 1 action → labeled "Bulk" button
   * - 2+ actions → icon dropdown
   * - function may return actions[] or a custom ReactNode
   */
  bulkActions?:
    | DataTableBulkAction<TData>[]
    | ((
        selected: TData[],
        table: Table<TData>,
      ) => DataTableBulkAction<TData>[] | ReactNode);
  rowActions?: (row: TData, table: Table<TData>) => ReactNode;
  /** Custom card renderer for grid view */
  renderGridItem?: (row: TData, table: Table<TData>) => ReactNode;

  /**
   * Called after a successful DnD reorder with the full reordered dataset.
   * Parent should update `data` (controlled).
   */
  onRowReorder?: (rows: TData[]) => void;

  /**
   * Called when an inline cell edit is committed.
   * Parent should update `data` (controlled).
   */
  onCellEdit?: (payload: {
    rowId: string;
    columnId: string;
    value: unknown;
    row: TData;
  }) => void;

  onExportReady?: (rows: TData[], table: Table<TData>) => void;
  /**
   * Called when a data row / card is clicked, excluding interactive controls
   * (checkboxes, action buttons, links, inline editors).
   */
  onRowClick?: (row: TData) => void;
  className?: string;
};

export type UseDataTableOptions<TData> = Pick<
  DataTableProps<TData>,
  | "columns"
  | "data"
  | "getRowId"
  | "features"
  | "manual"
  | "state"
  | "initialState"
  | "pageCount"
>;
