"use client";

import {
  useCallback,
  useMemo,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import type {
  ColumnDef,
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
  SortingState,
} from "@tanstack/react-table";

import {
  columnFiltersToParams,
  filterParamSpecsFromColumns,
  sortFieldsFromColumns,
  sortParamToSorting,
  sortingToSortParam,
  type FilterParamSpecs,
  type FilterQueryParams,
} from "@/components/entity/server-list-params";
import { readPersistedTableState } from "@/components/tables/use-data-table";
import { useDebounce } from "@/hooks/use-debounce";

export type ServerListParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
};

/** `ServerListParams` plus the mapped column filter params. */
export type ServerListQuery = ServerListParams &
  Record<string, string | number | undefined>;

const LEGACY_DEFAULT_SORT = "-created_at";

export type UseServerListStateOptions<TData = unknown> = {
  initialPageSize?: number;
  /**
   * Default sort (`field` / `-field`) used until the user picks one.
   * - omitted → legacy `-created_at`
   * - `null`  → no `sort` param (endpoint default)
   * May change over time (e.g. `meta?.default_sort` once `/meta` loads);
   * the list follows it while the user has not sorted.
   */
  initialSort?: string | null;
  searchDebounceMs?: number;
  /**
   * Column defs: filters of columns with `meta.param` become query params
   * (format from `meta.paramFormat` or `meta.filterVariant`) and
   * `meta.sortParam` renames the sort field.
   */
  columns?: ColumnDef<TData, unknown>[];
  /** Explicit column id → param specs (merged over `columns`). */
  filterParams?: FilterParamSpecs;
  /** Same key as `features.persistKey`; restores the persisted page size. */
  persistKey?: string;
};

function isTextSpec(spec: FilterParamSpecs[string]) {
  if (spec.format && spec.format !== "string") return false;
  return !spec.filterVariant || spec.filterVariant === "text";
}

function persistedPageSize(persistKey: string | undefined) {
  const size = readPersistedTableState(persistKey)?.pagination?.pageSize;
  return typeof size === "number" && size > 0 ? size : undefined;
}

/**
 * Controlled server-list state for DataTable + OpenAPI list endpoints
 * (`limit` / `offset` / `sort` / `q` + mapped column filters).
 */
export function useServerListState<TData = unknown>(
  options: UseServerListStateOptions<TData> = {},
) {
  const {
    initialPageSize = 20,
    initialSort,
    searchDebounceMs = 300,
    columns,
    filterParams,
    persistKey,
  } = options;

  const defaultSort =
    initialSort === undefined
      ? LEGACY_DEFAULT_SORT
      : (initialSort ?? undefined);

  const sortFields = useMemo(() => sortFieldsFromColumns(columns), [columns]);
  const specs = useMemo<FilterParamSpecs>(
    () => ({ ...filterParamSpecsFromColumns(columns), ...filterParams }),
    [columns, filterParams],
  );

  const [pagination, setPagination] = useState<PaginationState>(() => ({
    pageIndex: 0,
    pageSize: persistedPageSize(persistKey) ?? initialPageSize,
  }));
  // null = the user has not sorted yet → follow `initialSort`.
  const [userSorting, setUserSorting] = useState<SortingState | null>(null);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [globalFilter, setGlobalFilter] = useState("");
  const debouncedSearch = useDebounce(globalFilter, searchDebounceMs);

  const defaultSorting = useMemo(
    () => sortParamToSorting(defaultSort, sortFields),
    [defaultSort, sortFields],
  );
  const sorting = userSorting ?? defaultSorting;

  const onPaginationChange: OnChangeFn<PaginationState> = useCallback(
    (updater) => {
      setPagination((prev) =>
        typeof updater === "function" ? updater(prev) : updater,
      );
    },
    [],
  );

  const onSortingChange: OnChangeFn<SortingState> = useCallback(
    (updater) => {
      setUserSorting((prev) => {
        const base = prev ?? defaultSorting;
        return typeof updater === "function" ? updater(base) : updater;
      });
      setPagination((prev) => ({ ...prev, pageIndex: 0 }));
    },
    [defaultSorting],
  );

  const onColumnFiltersChange: OnChangeFn<ColumnFiltersState> = useCallback(
    (updater) => {
      setColumnFilters((prev) =>
        typeof updater === "function" ? updater(prev) : updater,
      );
      setPagination((prev) => ({ ...prev, pageIndex: 0 }));
    },
    [],
  );

  const onGlobalFilterChange: OnChangeFn<string> = useCallback((updater) => {
    // TanStack `resetGlobalFilter()` may pass `undefined` — keep a string.
    setGlobalFilter((prev) => {
      const next = typeof updater === "function" ? updater(prev) : updater;
      return next ?? "";
    });
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  }, []);

  // Free-text column filters are debounced like `q`; selects, facets,
  // booleans and ranges apply immediately.
  const { textSpecs, otherSpecs } = useMemo(() => {
    const text: FilterParamSpecs = {};
    const other: FilterParamSpecs = {};
    for (const [id, spec] of Object.entries(specs)) {
      if (isTextSpec(spec)) text[id] = spec;
      else other[id] = spec;
    }
    return { textSpecs: text, otherSpecs: other };
  }, [specs]);
  const textKey = JSON.stringify(
    columnFiltersToParams(columnFilters, textSpecs),
  );
  const otherKey = JSON.stringify(
    columnFiltersToParams(columnFilters, otherSpecs),
  );
  const debouncedTextKey = useDebounce(textKey, searchDebounceMs);

  const filterQuery = useMemo<FilterQueryParams>(
    () => ({
      ...(JSON.parse(debouncedTextKey) as FilterQueryParams),
      ...(JSON.parse(otherKey) as FilterQueryParams),
    }),
    [debouncedTextKey, otherKey],
  );

  const params: ServerListQuery = useMemo(() => {
    const q = (debouncedSearch ?? "").trim();
    const sort = sortingToSortParam(sorting, sortFields) ?? defaultSort;
    return {
      ...filterQuery,
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
      ...(sort ? { sort } : {}),
      ...(q ? { q } : {}),
    };
  }, [
    debouncedSearch,
    defaultSort,
    filterQuery,
    pagination,
    sortFields,
    sorting,
  ]);

  const resetListState = useCallback(() => {
    setPagination({ pageIndex: 0, pageSize: initialPageSize });
    setUserSorting(null);
    setColumnFilters([]);
    setGlobalFilter("");
  }, [initialPageSize]);

  return {
    params,
    /** Only the mapped column filter params (e.g. for bulk/export queries). */
    filterParams: filterQuery,
    pagination,
    setPagination: setPagination as Dispatch<SetStateAction<PaginationState>>,
    sorting,
    columnFilters,
    globalFilter,
    onPaginationChange,
    onSortingChange,
    onColumnFiltersChange,
    onGlobalFilterChange,
    resetListState,
    tableState: {
      pagination,
      onPaginationChange,
      sorting,
      onSortingChange,
      columnFilters,
      onColumnFiltersChange,
      globalFilter,
      onGlobalFilterChange,
    },
  };
}
