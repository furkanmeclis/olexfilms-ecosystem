import type {
  ColumnDef,
  ColumnFiltersState,
  SortingState,
} from "@tanstack/react-table";

import type {
  ColumnFilterVariant,
  ColumnParamFormat,
} from "@/components/tables/types";

/**
 * Declarative column filter → query param mapping (list contract:
 * `_tools/datatable/CONTRACT.md`, summarised in AGENTS.md).
 */
export type FilterParamSpec = {
  /** Base query param name (`status`, `created` → `created_from/_to`…) */
  param: string;
  /** Explicit format; derived from `filterVariant` when omitted */
  format?: ColumnParamFormat;
  /** Column filter UI kind, used to derive the default format */
  filterVariant?: ColumnFilterVariant;
};

/** Column id → filter param spec */
export type FilterParamSpecs = Record<string, FilterParamSpec>;

/** Query params produced from column filters (only defined values). */
export type FilterQueryParams = Record<string, string>;

function columnIdOf<TData>(column: ColumnDef<TData, unknown>) {
  if (column.id) return column.id;
  if ("accessorKey" in column && column.accessorKey != null) {
    return String(column.accessorKey);
  }
  return undefined;
}

/** Collect `meta.param` / `meta.paramFormat` from column defs. */
export function filterParamSpecsFromColumns<TData>(
  columns: ColumnDef<TData, unknown>[] | undefined,
): FilterParamSpecs {
  const specs: FilterParamSpecs = {};
  for (const column of columns ?? []) {
    const id = columnIdOf(column);
    const param = column.meta?.param;
    if (!id || !param) continue;
    specs[id] = {
      param,
      format: column.meta?.paramFormat,
      filterVariant: column.meta?.filterVariant,
    };
  }
  return specs;
}

/** Column id → backend sort field, for columns with `meta.sortParam`. */
export function sortFieldsFromColumns<TData>(
  columns: ColumnDef<TData, unknown>[] | undefined,
): Record<string, string> {
  const fields: Record<string, string> = {};
  for (const column of columns ?? []) {
    const id = columnIdOf(column);
    const field = column.meta?.sortParam;
    if (id && field) fields[id] = field;
  }
  return fields;
}

function defaultFormat(
  variant: ColumnFilterVariant | undefined,
): Exclude<ColumnParamFormat, (value: unknown) => unknown> {
  switch (variant) {
    case "multi-select":
    case "faceted":
      return "csv";
    case "boolean":
      return "boolean";
    case "date-range":
      return "date-range";
    case "number-range":
      return "number-range";
    default:
      return "string";
  }
}

function scalar(value: unknown): string | undefined {
  if (value == null) return undefined;
  const text = String(value).trim();
  return text ? text : undefined;
}

function list(value: unknown): string[] {
  const raw = Array.isArray(value) ? value : value == null ? [] : [value];
  return raw.map(scalar).filter((item): item is string => Boolean(item));
}

function range(value: unknown): [string | undefined, string | undefined] {
  if (!Array.isArray(value)) return [undefined, undefined];
  return [scalar(value[0]), scalar(value[1])];
}

/** Map one filter value to query params according to its spec. */
export function filterValueToParams(
  spec: FilterParamSpec,
  value: unknown,
): FilterQueryParams {
  const format = spec.format ?? defaultFormat(spec.filterVariant);
  const out: FilterQueryParams = {};
  const set = (key: string, next: string | undefined) => {
    if (next !== undefined && next !== "") out[key] = next;
  };

  if (typeof format === "function") {
    for (const [key, next] of Object.entries(format(value))) set(key, next);
    return out;
  }

  switch (format) {
    case "csv":
      set(spec.param, list(value).join(",") || undefined);
      break;
    case "boolean":
      if (value === true || value === "true") set(spec.param, "true");
      else if (value === false || value === "false") set(spec.param, "false");
      break;
    case "date-range": {
      const [from, to] = range(value);
      set(`${spec.param}_from`, from);
      set(`${spec.param}_to`, to);
      break;
    }
    case "number-range": {
      const [min, max] = range(value);
      set(`${spec.param}_min`, min);
      set(`${spec.param}_max`, max);
      break;
    }
    default:
      // A stray array (e.g. faceted UI on a "string" param) sends its first
      // value rather than "a,b" to a single-value param.
      set(spec.param, Array.isArray(value) ? list(value)[0] : scalar(value));
  }
  return out;
}

/** Map TanStack column filters to query params (unmapped columns skipped). */
export function columnFiltersToParams(
  filters: ColumnFiltersState,
  specs: FilterParamSpecs,
): FilterQueryParams {
  const out: FilterQueryParams = {};
  for (const filter of filters) {
    const spec = specs[filter.id];
    if (!spec) continue;
    Object.assign(out, filterValueToParams(spec, filter.value));
  }
  return out;
}

/**
 * Single-field sort param (`field` / `-field`). Only the first sorting entry
 * is sent: list endpoints apply one primary field plus an id tiebreak.
 */
export function sortingToSortParam(
  sorting: SortingState,
  sortFields: Record<string, string> = {},
): string | undefined {
  const first = sorting[0];
  if (!first) return undefined;
  const field = sortFields[first.id] ?? first.id;
  return first.desc ? `-${field}` : field;
}

/** Parse `field` / `-field` (first token of a legacy CSV) into sorting. */
export function sortParamToSorting(
  sort: string | null | undefined,
  sortFields: Record<string, string> = {},
): SortingState {
  const token = (sort ?? "").split(",")[0]?.trim();
  if (!token) return [];
  const desc = token.startsWith("-");
  const field = desc ? token.slice(1) : token;
  if (!field) return [];
  const columnId =
    Object.entries(sortFields).find(([, value]) => value === field)?.[0] ??
    field;
  return [{ id: columnId, desc }];
}
