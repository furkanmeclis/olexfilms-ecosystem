"use client";

import type { Table } from "@tanstack/react-table";
import {
  Columns3,
  Download,
  GripVertical,
  LayoutGrid,
  List,
  RotateCcw,
  Rows3,
  Search,
  X,
} from "lucide-react";
import type { ReactNode } from "react";
import { isValidElement } from "react";

import {
  DataTableBulkActions,
  resolveBulkActions,
  type DataTableBulkAction,
} from "@/components/tables/bulk-actions";
import { DataTableFacetedFilter } from "@/components/tables/faceted-filter";
import { ToolbarIconButton } from "@/components/tables/toolbar-icon-button";
import type {
  DataTableColumnMeta,
  DataTableFeatures,
  TableDensity,
  TableViewMode,
} from "@/components/tables/types";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

type DataTableToolbarProps<TData> = {
  table: Table<TData>;
  features: Required<Omit<DataTableFeatures, "persistKey">> & {
    persistKey?: string;
  };
  density: TableDensity;
  setDensity: (d: TableDensity) => void;
  viewMode: TableViewMode;
  setViewMode: (m: TableViewMode) => void;
  resetAll: () => void;
  toolbar?: ReactNode;
  bulkActions?:
    | DataTableBulkAction<TData>[]
    | ((
        selected: TData[],
        table: Table<TData>,
      ) => DataTableBulkAction<TData>[] | ReactNode);
  onExport?: () => void;
  forceMobileCards?: boolean;
  reorderMode?: boolean;
  onReorderModeChange?: (next: boolean) => void;
};

export function DataTableToolbar<TData>({
  table,
  features,
  density,
  setDensity,
  viewMode,
  setViewMode,
  resetAll,
  toolbar,
  bulkActions,
  onExport,
  forceMobileCards,
  reorderMode,
  onReorderModeChange,
}: DataTableToolbarProps<TData>) {
  const { t } = useLocale();
  const selected = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original);
  const selectedCount = selected.length;
  const isFiltered =
    table.getState().columnFilters.length > 0 ||
    Boolean(table.getState().globalFilter);
  const globalFilter = String(table.getState().globalFilter ?? "");

  const facetedColumns =
    features.facetedFilters !== false
      ? table.getAllColumns().filter((column) => {
          const meta = column.columnDef.meta as DataTableColumnMeta | undefined;
          return (
            column.getCanFilter() &&
            meta?.filterVariant === "faceted" &&
            (meta.filterOptions?.length ?? 0) > 0
          );
        })
      : [];

  const resolvedBulk = resolveBulkActions(bulkActions, selected, table);
  const bulkNode =
    resolvedBulk == null ? null : isValidElement(resolvedBulk) ||
      typeof resolvedBulk === "string" ? (
      (resolvedBulk as ReactNode)
    ) : Array.isArray(resolvedBulk) ? (
      <DataTableBulkActions
        actions={resolvedBulk}
        selected={selected}
        table={table}
      />
    ) : null;

  return (
    <div className="flex flex-col gap-3">
      {/* Phones: search on its own row, then filters, page actions
          (export/import/refresh) and table controls on a single row that
          scrolls horizontally. From sm up the scroller dissolves (contents)
          into the usual wrapping toolbar with table controls on the right. */}
      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
        {features.globalFilter ? (
          <InputGroup className="w-full sm:max-w-xs">
            <InputGroupAddon align="inline-start">
              <Search className="size-4" aria-hidden />
            </InputGroupAddon>
            <InputGroupInput
              value={globalFilter}
              onChange={(e) => table.setGlobalFilter(e.target.value)}
              placeholder={t("table.search")}
            />
            {globalFilter ? (
              <InputGroupAddon align="inline-end">
                <InputGroupButton
                  size="icon-xs"
                  aria-label={t("table.clear_filter")}
                  onClick={() => table.setGlobalFilter("")}
                >
                  <X className="size-3.5" />
                </InputGroupButton>
              </InputGroupAddon>
            ) : null}
          </InputGroup>
        ) : null}

        <div className="-mx-1 flex [scrollbar-width:none] items-center gap-2 overflow-x-auto px-1 py-0.5 sm:contents [&::-webkit-scrollbar]:hidden">
          <div className="flex shrink-0 items-center gap-2 sm:flex-1 sm:flex-wrap [&>*]:shrink-0">
            {facetedColumns.map((column) => {
              const meta = column.columnDef.meta as DataTableColumnMeta;
              const title = meta.labelKey
                ? t(meta.labelKey)
                : (meta.label ?? column.id);
              const options = (meta.filterOptions ?? []).map((option) => ({
                value: option.value,
                label: option.labelKey ? t(option.labelKey) : option.label,
              }));
              return (
                <DataTableFacetedFilter
                  key={column.id}
                  column={column}
                  title={title}
                  options={options}
                />
              );
            })}

            {toolbar}
            {isFiltered ? (
              <ToolbarIconButton
                label={t("table.reset_filters")}
                variant="ghost"
                onClick={() => {
                  table.resetColumnFilters();
                  table.resetGlobalFilter();
                }}
              >
                <X className="size-4" />
              </ToolbarIconButton>
            ) : null}
          </div>

          <div className="flex shrink-0 items-center gap-1.5 sm:flex-wrap sm:justify-end [&>*]:shrink-0">
            {selectedCount > 0 ? (
              <span className="text-muted-foreground me-1 text-xs">
                {t("table.selected", { count: selectedCount })}
              </span>
            ) : null}
            {bulkNode}

            {onReorderModeChange ? (
              <ToolbarIconButton
                label={
                  reorderMode
                    ? t("table.reorder_done")
                    : t("table.reorder_mode")
                }
                variant={reorderMode ? "default" : "outline"}
                aria-pressed={reorderMode}
                onClick={() => onReorderModeChange(!reorderMode)}
              >
                <GripVertical className="size-4" />
              </ToolbarIconButton>
            ) : null}

            {features.viewMode && !forceMobileCards && !reorderMode ? (
              <div className="border-border flex rounded-md border p-0.5">
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className={cn("size-8", viewMode === "table" && "bg-muted")}
                  onClick={() => setViewMode("table")}
                  aria-label={t("table.view_table")}
                >
                  <List className="size-4" />
                </Button>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className={cn("size-8", viewMode === "grid" && "bg-muted")}
                  onClick={() => setViewMode("grid")}
                  aria-label={t("table.view_grid")}
                >
                  <LayoutGrid className="size-4" />
                </Button>
              </div>
            ) : null}

            {features.density && !forceMobileCards ? (
              <DropdownMenu>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <DropdownMenuTrigger asChild>
                      <Button
                        type="button"
                        variant="outline"
                        size="icon"
                        className="size-8"
                        aria-label={t("table.density")}
                      >
                        <Rows3 className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {t("table.density")}
                  </TooltipContent>
                </Tooltip>
                <DropdownMenuContent align="end">
                  <DropdownMenuLabel>{t("table.density")}</DropdownMenuLabel>
                  <DropdownMenuRadioGroup
                    value={density}
                    onValueChange={(v) => setDensity(v as TableDensity)}
                  >
                    <DropdownMenuRadioItem value="compact">
                      {t("table.density_compact")}
                    </DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value="comfortable">
                      {t("table.density_comfortable")}
                    </DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value="spacious">
                      {t("table.density_spacious")}
                    </DropdownMenuRadioItem>
                  </DropdownMenuRadioGroup>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}

            {features.columnVisibility ? (
              <DropdownMenu>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <DropdownMenuTrigger asChild>
                      <Button
                        type="button"
                        variant="outline"
                        size="icon"
                        className="size-8"
                        aria-label={t("table.columns")}
                      >
                        <Columns3 className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                  </TooltipTrigger>
                  <TooltipContent side="bottom">
                    {t("table.columns")}
                  </TooltipContent>
                </Tooltip>
                <DropdownMenuContent align="end" className="w-52">
                  <DropdownMenuLabel>{t("table.columns")}</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  {table
                    .getAllColumns()
                    .filter((col) => col.getCanHide())
                    .map((col) => {
                      const meta = col.columnDef.meta;
                      const label = meta?.labelKey
                        ? t(meta.labelKey)
                        : (meta?.label ?? col.id);
                      return (
                        <DropdownMenuCheckboxItem
                          key={col.id}
                          checked={col.getIsVisible()}
                          onCheckedChange={(value) =>
                            col.toggleVisibility(Boolean(value))
                          }
                        >
                          {label}
                        </DropdownMenuCheckboxItem>
                      );
                    })}
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}

            {onExport ? (
              <ToolbarIconButton label={t("table.export")} onClick={onExport}>
                <Download className="size-4" />
              </ToolbarIconButton>
            ) : null}

            <DropdownMenu>
              <Tooltip>
                <TooltipTrigger asChild>
                  <DropdownMenuTrigger asChild>
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      className="size-8"
                      aria-label={t("table.reset")}
                    >
                      <RotateCcw className="size-4" />
                    </Button>
                  </DropdownMenuTrigger>
                </TooltipTrigger>
                <TooltipContent side="bottom">
                  {t("table.reset")}
                </TooltipContent>
              </Tooltip>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => table.resetSorting()}>
                  {t("table.reset_sort")}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => {
                    table.resetColumnFilters();
                    table.resetGlobalFilter();
                  }}
                >
                  {t("table.reset_filters")}
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => {
                    table.resetColumnVisibility();
                    table.resetColumnOrder();
                    table.resetColumnPinning();
                    table.resetColumnSizing();
                  }}
                >
                  {t("table.reset_columns")}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={resetAll}>
                  {t("table.reset_all")}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </div>
    </div>
  );
}
