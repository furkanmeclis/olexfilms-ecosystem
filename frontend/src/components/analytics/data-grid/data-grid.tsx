"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  getCoreRowModel,
  getExpandedRowModel,
  getFilteredRowModel,
  getGroupedRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type ColumnFiltersState,
  type ExpandedState,
  type Row,
  type SortingState,
} from "@tanstack/react-table";
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronDown,
  ChevronRight,
  Download,
  FilterX,
  Search,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuthStore } from "@/lib/auth/session-store";
import { cn, download } from "@/lib/utils";
import { useFormatter, useLocale } from "@/providers/locale-provider";

import { reportLinkHref } from "../adapters";
import type { GridColumn, GridRow } from "../types";
import { useDurationLabels } from "../use-duration-labels";
import { ColumnFilter, type FilterOption } from "./column-filter";
import {
  clearGridPrefs,
  gridPrefsKey,
  readGridPrefs,
  reconcileOrder,
  writeGridPrefs,
  type GridPrefs,
} from "./column-prefs";
import { ColumnSettings } from "./column-settings";
import {
  aggregate,
  canGroupBy,
  cellValue,
  columnAlign,
  columnTotals,
  defaultFilterKind,
  exportFileName,
  formatCell,
  formatTyped,
  isNumericColumn,
  matchesRange,
  matchesSelect,
  matchesText,
  numericValue,
  searchText,
  statusOf,
  toCsv,
  toXlsxBlob,
  virtualWindow,
  type CellFormatContext,
  type RangeFilterValue,
  type SelectFilterValue,
} from "./grid-utils";
import { StatusBadge } from "./status-badge";

export type DataGridProps<R extends GridRow = GridRow> = {
  columns: GridColumn<R>[];
  rows: R[];
  /** Unique id; enables per-user layout persistence (order, hidden, pin, sort, group). */
  persistKey?: string;
  /** Tenant slug, used to turn `_link` into detail hrefs. */
  slug?: string;
  /** Override the row link (default: `_link` → detail page). */
  rowHref?: (row: R) => string | null | undefined;
  /** Base name of client exports (default "report"). */
  exportName?: string;
  /** Adds "full report" export entries that call the backend export. */
  onServerExport?: (format: "csv" | "xlsx") => void | Promise<void>;
  /** Footer totals row (default: when a column has `aggregate`). */
  showTotals?: boolean;
  /** Scroll viewport height in px (sticky header, virtualisation). */
  height?: number;
  /** Virtualise from this many rendered rows (default 120). */
  virtualizeFrom?: number;
  /** Initial sort when no saved preference. */
  defaultSorting?: SortingState;
  /** Initial group-by column. */
  defaultGroupBy?: string;
  loading?: boolean;
  /** Extra toolbar content (start side). */
  toolbar?: ReactNode;
  emptyText?: string;
  className?: string;
  /** Accessible table name. */
  caption?: string;
};

const ROW_HEIGHT = 36;

/**
 * Excel-like analytics table (client side, @tanstack/react-table): multi
 * sort (shift+click), per-column filters, global search, column show/hide /
 * drag order / pin first, sticky header, grouping with subtotals, totals
 * footer, typed formatting, row links, virtualised body, CSV/XLSX export.
 */
export function DataGrid<R extends GridRow = GridRow>({
  columns,
  rows,
  persistKey,
  slug = "",
  rowHref,
  exportName = "report",
  onServerExport,
  showTotals,
  height = 520,
  virtualizeFrom = 120,
  defaultSorting = [],
  defaultGroupBy,
  loading,
  toolbar,
  emptyText,
  className,
  caption,
}: DataGridProps<R>) {
  const { t } = useLocale();
  const fmt = useFormatter();
  const durations = useDurationLabels();
  const router = useRouter();
  const userId = useAuthStore((s) => s.user?.uuid);
  const ctx = useMemo<CellFormatContext>(
    () => ({ fmt, durations }),
    [fmt, durations],
  );

  const ids = useMemo(() => columns.map((c) => c.key), [columns]);
  const byKey = useMemo(
    () => new Map(columns.map((c) => [c.key, c] as const)),
    [columns],
  );
  const prefsKey = persistKey ? gridPrefsKey(persistKey, userId) : null;

  const [prefs, setPrefs] = useState<GridPrefs>(
    () => readGridPrefs(prefsKey) ?? {},
  );
  // Another user (or the persisted key) became known after mount.
  const loadedKey = useRef(prefsKey);
  useEffect(() => {
    if (loadedKey.current === prefsKey) return;
    loadedKey.current = prefsKey;
    setPrefs(readGridPrefs(prefsKey) ?? {});
  }, [prefsKey]);
  const updatePrefs = useCallback(
    (patch: Partial<GridPrefs>) => {
      setPrefs((prev) => {
        const next = { ...prev, ...patch };
        writeGridPrefs(prefsKey, next);
        return next;
      });
    },
    [prefsKey],
  );

  const order = useMemo(
    () => reconcileOrder(prefs.order, ids),
    [prefs.order, ids],
  );
  const hidden = useMemo(
    () => (prefs.hidden ?? []).filter((id) => ids.includes(id)),
    [prefs.hidden, ids],
  );
  const pinFirst = prefs.pinFirst ?? true;
  const sorting = prefs.sorting ?? defaultSorting;
  const groupBy =
    prefs.groupBy === undefined ? (defaultGroupBy ?? null) : prefs.groupBy;
  const grouping = useMemo(
    () => (groupBy && byKey.has(groupBy) ? [groupBy] : []),
    [groupBy, byKey],
  );

  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [globalFilter, setGlobalFilter] = useState("");
  const [expanded, setExpanded] = useState<ExpandedState>(true);

  const columnDefs = useMemo<ColumnDef<R>[]>(
    () =>
      columns.map((col) => {
        const kind = defaultFilterKind(col);
        const numeric = isNumericColumn(col);
        return {
          id: col.key,
          accessorFn: (row: R) => cellValue(row, col),
          header: col.label,
          enableGrouping: canGroupBy(col),
          sortUndefined: "last",
          sortingFn: numeric
            ? (a: Row<R>, b: Row<R>, id: string) =>
                (numericValue(a.getValue(id)) ?? -Infinity) -
                (numericValue(b.getValue(id)) ?? -Infinity)
            : (a: Row<R>, b: Row<R>, _id: string) =>
                formatCell(a.original, col, ctx).localeCompare(
                  formatCell(b.original, col, ctx),
                  fmt.intlLocale,
                  { numeric: true, sensitivity: "base" },
                ),
          filterFn: (row: Row<R>, id: string, value: unknown) => {
            if (kind === "range")
              return matchesRange(row.getValue(id), value as RangeFilterValue);
            if (kind === "select")
              return matchesSelect(
                row.getValue(id),
                value as SelectFilterValue,
              );
            return matchesText(
              formatCell(row.original, col, ctx),
              String(value ?? ""),
              fmt.locale,
            );
          },
          aggregationFn: (id: string, leafRows: Row<R>[]) =>
            aggregate(
              leafRows.map((r) => r.getValue(id)),
              col.aggregate,
            ),
        } satisfies ColumnDef<R>;
      }),
    [columns, ctx, fmt],
  );

  const table = useReactTable<R>({
    data: rows,
    columns: columnDefs,
    state: {
      sorting,
      columnFilters,
      globalFilter,
      columnOrder: order,
      columnVisibility: Object.fromEntries(hidden.map((h) => [h, false])),
      grouping,
      expanded,
    },
    onSortingChange: (updater) => {
      const next = typeof updater === "function" ? updater(sorting) : updater;
      updatePrefs({ sorting: next });
    },
    onColumnFiltersChange: setColumnFilters,
    onGlobalFilterChange: setGlobalFilter,
    onExpandedChange: setExpanded,
    globalFilterFn: (row, columnId, value) => {
      const col = byKey.get(columnId);
      return col
        ? matchesText(
            searchText(row.original, col, ctx),
            String(value ?? ""),
            fmt.locale,
          )
        : false;
    },
    getColumnCanGlobalFilter: () => true,
    enableMultiSort: true,
    maxMultiSortColCount: 4,
    groupedColumnMode: false,
    autoResetExpanded: false,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getGroupedRowModel: getGroupedRowModel(),
    getExpandedRowModel: getExpandedRowModel(),
  });

  const visibleColumns = useMemo(
    () =>
      order
        .filter((id) => !hidden.includes(id))
        .map((id) => byKey.get(id))
        .filter(Boolean) as GridColumn<R>[],
    [order, hidden, byKey],
  );

  const filteredLeaves = table
    .getFilteredRowModel()
    .rows.map((r) => r.original);
  const sortedLeaves = table
    .getSortedRowModel()
    .flatRows.filter((r) => !r.getIsGrouped())
    .map((r) => r.original);
  const totals = useMemo(
    () => columnTotals(filteredLeaves, visibleColumns),
    [filteredLeaves, visibleColumns],
  );
  const hasAggregates = visibleColumns.some(
    (c) => c.aggregate && c.aggregate !== "none",
  );
  const totalsOn = (showTotals ?? hasAggregates) && filteredLeaves.length > 0;

  const selectOptions = useMemo(() => {
    const out = new Map<string, FilterOption[]>();
    for (const col of columns) {
      if (defaultFilterKind(col) !== "select") continue;
      if (col.options) {
        out.set(col.key, col.options);
        continue;
      }
      const seen = new Map<string, string>();
      for (const row of rows) {
        const raw = cellValue(row, col);
        const value = raw === null || raw === undefined ? "" : String(raw);
        if (seen.has(value)) continue;
        seen.set(
          value,
          col.type === "status"
            ? (statusOf(row, col)?.label ?? value)
            : formatCell(row, col, ctx),
        );
      }
      out.set(
        col.key,
        [...seen.entries()]
          .map(([value, label]) => ({ value, label }))
          .sort((a, b) => a.label.localeCompare(b.label, fmt.intlLocale)),
      );
    }
    return out;
  }, [columns, rows, ctx, fmt.intlLocale]);

  // ---- virtualisation ------------------------------------------------------
  const scrollRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const renderRows = table.getRowModel().rows;
  const virtual = renderRows.length >= virtualizeFrom;
  const win = virtual
    ? virtualWindow(renderRows.length, scrollTop, height, ROW_HEIGHT)
    : { start: 0, end: renderRows.length, padTop: 0, padBottom: 0 };
  const slice = renderRows.slice(win.start, win.end);

  const hrefOf = useCallback(
    (row: R) =>
      (rowHref ? rowHref(row) : reportLinkHref(row._link ?? null, slug)) ??
      null,
    [rowHref, slug],
  );

  // ---- export --------------------------------------------------------------
  const exportTotals = totalsOn
    ? { label: t("chart.grid.total"), values: totals }
    : undefined;
  const exportCsv = () => {
    const csv = toCsv(visibleColumns, sortedLeaves, ctx, exportTotals);
    download(
      new Blob([csv], { type: "text/csv;charset=utf-8" }),
      exportFileName(exportName, "csv"),
    );
  };
  const exportXlsx = async () => {
    const blob = await toXlsxBlob(
      visibleColumns,
      sortedLeaves,
      ctx,
      caption ?? exportName,
      exportTotals,
    );
    download(blob, exportFileName(exportName, "xlsx"));
  };

  const groupable = columns.filter(canGroupBy);
  const filtersActive = columnFilters.length > 0 || globalFilter.trim() !== "";

  const cellClass = (col: GridColumn<R>, index: number) =>
    cn(
      "px-3 whitespace-nowrap",
      columnAlign(col) === "right" ? "text-end tabular-nums" : "text-start",
      pinFirst &&
        index === 0 &&
        "sticky start-0 z-[1] bg-[var(--viz-surface)] shadow-[inset_-1px_0_0_var(--viz-grid)] rtl:shadow-[inset_1px_0_0_var(--viz-grid)]",
    );

  const renderCell = (row: R, col: GridColumn<R>, index: number) => {
    if (col.render) return col.render(row);
    const text = formatCell(row, col, ctx);
    if (col.type === "status") {
      const s = statusOf(row, col);
      return s ? <StatusBadge status={s} /> : text;
    }
    if (col.type === "link") {
      const v = cellValue(row, col);
      const href =
        typeof v === "string" ? v : (v as { href?: string } | null)?.href;
      return href ? (
        <Link
          href={href}
          className="text-primary underline-offset-4 hover:underline"
        >
          {text === href ? href : text}
        </Link>
      ) : (
        text
      );
    }
    const href = index === 0 ? hrefOf(row) : null;
    return href ? (
      <Link
        href={href}
        className="text-foreground font-medium underline-offset-4 hover:underline"
        onClick={(e) => e.stopPropagation()}
      >
        {text}
      </Link>
    ) : (
      text
    );
  };

  const colWidth = (col: GridColumn<R>) =>
    col.width ??
    (isNumericColumn(col) ? 128 : col.type === "status" ? 150 : 180);
  const tableWidth = visibleColumns.reduce((acc, c) => acc + colWidth(c), 0);

  return (
    <div
      className={cn("flex min-w-0 flex-col gap-2", className)}
      data-slot="data-grid"
    >
      <div className="flex flex-wrap items-center gap-2">
        {toolbar}
        <div className="relative min-w-48 flex-1 sm:max-w-xs">
          <Search
            className="text-muted-foreground pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2"
            aria-hidden
          />
          <Input
            type="search"
            value={globalFilter}
            onChange={(e) => setGlobalFilter(e.target.value)}
            placeholder={t("chart.grid.search")}
            aria-label={t("chart.grid.search")}
            className="h-8 ps-8"
          />
        </div>
        {groupable.length ? (
          <Select
            value={groupBy ?? "__none"}
            onValueChange={(v) => {
              setExpanded(true);
              updatePrefs({ groupBy: v === "__none" ? null : v });
            }}
          >
            <SelectTrigger
              size="sm"
              className="h-8 w-auto"
              aria-label={t("chart.grid.group_by")}
              data-testid="grid-group-by"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__none">
                {t("chart.grid.group_none")}
              </SelectItem>
              {groupable.map((c) => (
                <SelectItem key={c.key} value={c.key}>
                  {t("chart.grid.group_by_column", { column: c.label })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
        {filtersActive ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-8"
            onClick={() => {
              setColumnFilters([]);
              setGlobalFilter("");
            }}
          >
            <FilterX aria-hidden />
            {t("chart.grid.clear_filters")}
          </Button>
        ) : null}
        <div className="ms-auto flex items-center gap-2">
          <span
            className="text-muted-foreground text-xs tabular-nums"
            aria-live="polite"
          >
            {t("chart.grid.row_count", {
              count: fmt.number(filteredLeaves.length),
              total: fmt.number(rows.length),
            })}
          </span>
          <ColumnSettings
            items={columns.map((c) => ({ id: c.key, label: c.label }))}
            order={order}
            hidden={hidden}
            pinFirst={pinFirst}
            onOrder={(o) => updatePrefs({ order: o })}
            onHidden={(h) => updatePrefs({ hidden: h })}
            onPinFirst={(p) => updatePrefs({ pinFirst: p })}
            onReset={() => {
              clearGridPrefs(prefsKey);
              setPrefs({});
            }}
          />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" variant="outline" size="sm" className="h-8">
                <Download aria-hidden />
                {t("chart.grid.export")}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>
                {t("chart.grid.export_view")}
              </DropdownMenuLabel>
              <DropdownMenuItem onSelect={exportCsv} data-export="csv">
                CSV
              </DropdownMenuItem>
              <DropdownMenuItem
                onSelect={() => void exportXlsx()}
                data-export="xlsx"
              >
                Excel (XLSX)
              </DropdownMenuItem>
              {onServerExport ? (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuLabel>
                    {t("chart.grid.export_full")}
                  </DropdownMenuLabel>
                  <DropdownMenuItem
                    onSelect={() => void onServerExport("csv")}
                    data-export="server-csv"
                  >
                    CSV
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() => void onServerExport("xlsx")}
                    data-export="server-xlsx"
                  >
                    Excel (XLSX)
                  </DropdownMenuItem>
                </>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <div
        ref={scrollRef}
        onScroll={
          virtual ? (e) => setScrollTop(e.currentTarget.scrollTop) : undefined
        }
        className="relative overflow-auto rounded-xl border"
        style={{ maxHeight: height, background: "var(--viz-surface)" }}
        data-virtual={virtual || undefined}
      >
        <table
          className="w-full table-fixed border-separate border-spacing-0 text-sm"
          style={{ minWidth: tableWidth }}
          aria-rowcount={renderRows.length + 1}
        >
          {caption ? <caption className="sr-only">{caption}</caption> : null}
          <colgroup>
            {visibleColumns.map((c) => (
              <col key={c.key} style={{ width: colWidth(c) }} />
            ))}
          </colgroup>
          <thead className="sticky top-0 z-[2]">
            <tr>
              {visibleColumns.map((col, i) => {
                const column = table.getColumn(col.key);
                const sorted = column?.getIsSorted();
                const sortIndex = column?.getSortIndex() ?? -1;
                const SortIcon =
                  sorted === "asc"
                    ? ArrowUp
                    : sorted === "desc"
                      ? ArrowDown
                      : ArrowUpDown;
                const right = columnAlign(col) === "right";
                return (
                  <th
                    key={col.key}
                    scope="col"
                    aria-sort={
                      sorted === "asc"
                        ? "ascending"
                        : sorted === "desc"
                          ? "descending"
                          : undefined
                    }
                    className={cn(
                      cellClass(col, i),
                      "bg-muted h-9 border-b font-medium",
                      pinFirst && i === 0 && "bg-muted z-[3]",
                    )}
                  >
                    <div
                      className={cn(
                        "flex items-center gap-0.5",
                        right && "flex-row-reverse",
                      )}
                    >
                      <button
                        type="button"
                        onClick={column?.getToggleSortingHandler()}
                        title={t("chart.grid.sort_hint")}
                        className={cn(
                          "hover:text-foreground flex min-w-0 items-center gap-1 rounded",
                          right && "flex-row-reverse",
                          sorted ? "text-foreground" : "text-muted-foreground",
                        )}
                      >
                        <span className="truncate">{col.label}</span>
                        <SortIcon
                          className={cn(
                            "size-3.5 shrink-0",
                            !sorted && "opacity-40",
                          )}
                          aria-hidden
                        />
                        {sorted && sorting.length > 1 ? (
                          <span className="text-[10px] tabular-nums">
                            {sortIndex + 1}
                          </span>
                        ) : null}
                      </button>
                      <ColumnFilter
                        label={col.label}
                        kind={defaultFilterKind(col)}
                        value={column?.getFilterValue()}
                        options={selectOptions.get(col.key) ?? []}
                        onChange={(v) => column?.setFilterValue(v)}
                      />
                    </div>
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {loading ? (
              Array.from({ length: 6 }, (_, i) => (
                <tr key={`s${i}`}>
                  {visibleColumns.map((c) => (
                    <td key={c.key} className="h-9 border-b px-3">
                      <Skeleton className="h-4 w-full" />
                    </td>
                  ))}
                </tr>
              ))
            ) : renderRows.length === 0 ? (
              <tr>
                <td
                  colSpan={visibleColumns.length || 1}
                  className="text-muted-foreground h-24 text-center text-sm"
                >
                  {emptyText ?? t("chart.grid.empty")}
                </td>
              </tr>
            ) : (
              <>
                {win.padTop > 0 ? (
                  <tr aria-hidden style={{ height: win.padTop }}>
                    <td colSpan={visibleColumns.length} />
                  </tr>
                ) : null}
                {slice.map((row) => {
                  if (row.getIsGrouped()) {
                    const groupCol = byKey.get(String(row.groupingColumnId));
                    const label = groupCol
                      ? formatCell(row.original, groupCol, ctx)
                      : String(row.groupingValue ?? "");
                    const open = row.getIsExpanded();
                    return (
                      <tr
                        key={row.id}
                        className="bg-muted/50 h-9 font-medium"
                        data-row="group"
                        aria-rowindex={row.index + 2}
                      >
                        {visibleColumns.map((col, i) => {
                          if (i === 0) {
                            return (
                              <td
                                key={col.key}
                                className={cn(
                                  cellClass(col, i),
                                  "border-b bg-[color-mix(in_oklab,var(--muted)_60%,var(--viz-surface))]",
                                )}
                              >
                                <button
                                  type="button"
                                  onClick={row.getToggleExpandedHandler()}
                                  aria-expanded={open}
                                  className="flex min-w-0 items-center gap-1"
                                  style={{ paddingInlineStart: row.depth * 12 }}
                                >
                                  {open ? (
                                    <ChevronDown
                                      className="size-4 shrink-0"
                                      aria-hidden
                                    />
                                  ) : (
                                    <ChevronRight
                                      className="size-4 shrink-0 rtl:rotate-180"
                                      aria-hidden
                                    />
                                  )}
                                  <span className="truncate">{label}</span>
                                  <span className="text-muted-foreground text-xs font-normal">
                                    ({fmt.number(row.getLeafRows().length)})
                                  </span>
                                </button>
                              </td>
                            );
                          }
                          const agg =
                            col.aggregate && col.aggregate !== "none"
                              ? row.getValue(col.key)
                              : null;
                          return (
                            <td
                              key={col.key}
                              className={cn(cellClass(col, i), "border-b")}
                              title={
                                agg !== null
                                  ? col.aggregate === "avg"
                                    ? t("chart.grid.average")
                                    : t("chart.grid.subtotal")
                                  : undefined
                              }
                            >
                              {agg !== null && agg !== undefined
                                ? formatTyped(agg, col, ctx)
                                : null}
                            </td>
                          );
                        })}
                      </tr>
                    );
                  }
                  const href = hrefOf(row.original);
                  return (
                    <tr
                      key={row.id}
                      className={cn(
                        "hover:bg-muted/40 h-9",
                        href && "cursor-pointer",
                      )}
                      onClick={
                        href
                          ? (e) => {
                              if ((e.target as HTMLElement).closest("a,button"))
                                return;
                              router.push(href);
                            }
                          : undefined
                      }
                      data-row="leaf"
                    >
                      {visibleColumns.map((col, i) => (
                        <td
                          key={col.key}
                          className={cn(cellClass(col, i), "truncate border-b")}
                          style={
                            i === 0 && row.depth > 0
                              ? { paddingInlineStart: 12 + row.depth * 12 }
                              : undefined
                          }
                        >
                          {renderCell(row.original, col, i)}
                        </td>
                      ))}
                    </tr>
                  );
                })}
                {win.padBottom > 0 ? (
                  <tr aria-hidden style={{ height: win.padBottom }}>
                    <td colSpan={visibleColumns.length} />
                  </tr>
                ) : null}
              </>
            )}
          </tbody>
          {totalsOn && !loading ? (
            <tfoot className="sticky bottom-0 z-[2]">
              <tr data-row="totals">
                {visibleColumns.map((col, i) => {
                  const v = totals[col.key];
                  const has = v !== undefined && v !== null;
                  return (
                    <td
                      key={col.key}
                      className={cn(
                        cellClass(col, i),
                        "bg-muted h-9 border-t font-semibold",
                        pinFirst && i === 0 && "bg-muted",
                      )}
                      title={
                        has
                          ? col.aggregate === "avg"
                            ? t("chart.grid.average")
                            : t("chart.grid.total")
                          : undefined
                      }
                    >
                      {has
                        ? formatTyped(v, col, ctx)
                        : i === 0
                          ? t("chart.grid.total")
                          : null}
                    </td>
                  );
                })}
              </tr>
            </tfoot>
          ) : null}
        </table>
      </div>
    </div>
  );
}
