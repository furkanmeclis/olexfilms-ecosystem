import type { ColumnDef, RowData } from "@tanstack/react-table";

import type {
  ColumnEditVariant,
  ColumnFilterVariant,
  ColumnParamFormat,
  DataTableColumnMeta,
} from "@/components/tables/types";

type CreateColumnOptions<TData extends RowData, TValue = unknown> = ColumnDef<
  TData,
  TValue
> & {
  /** i18n key — resolved in DataTable header */
  labelKey: string;
  /** Optional fallback label */
  label?: string;
  filterVariant?: ColumnFilterVariant;
  filterOptions?: DataTableColumnMeta["filterOptions"];
  /** Double-click editor when `features.inlineEdit` is on */
  editVariant?: ColumnEditVariant;
  editOptions?: DataTableColumnMeta["editOptions"];
  gridPrimary?: boolean;
  gridSecondary?: boolean;
  defaultHidden?: boolean;
  /** Server query param for this column's filter (server mode) */
  param?: string;
  /** Filter → param format override (default derived from filterVariant) */
  paramFormat?: ColumnParamFormat;
  /** Backend sort field when it differs from the column id */
  sortParam?: string;
};

/**
 * Column factory with i18n-first headers.
 * Header text is resolved from `labelKey` via ColumnHeader + `t()`.
 */
export function createColumn<TData extends RowData, TValue = unknown>(
  options: CreateColumnOptions<TData, TValue>,
): ColumnDef<TData, TValue> {
  const {
    labelKey,
    label,
    filterVariant,
    filterOptions,
    editVariant,
    editOptions,
    gridPrimary,
    gridSecondary,
    defaultHidden,
    param,
    paramFormat,
    sortParam,
    meta,
    enableHiding,
    header,
    ...rest
  } = options;

  const filterFn =
    rest.filterFn ??
    (filterVariant === "faceted"
      ? (row, columnId, filterValue: unknown) => {
          const selected = filterValue as string[] | undefined;
          if (!selected?.length) return true;
          const cell = row.getValue(columnId);
          return selected.includes(String(cell));
        }
      : undefined);

  return {
    ...rest,
    filterFn,
    enableHiding: enableHiding ?? true,
    // Only columns with an explicit filterVariant are filterable.
    // TanStack defaults `undefined` → true, which shows dead inputs on
    // server-driven lists (`manual.filtering`) that never map those filters.
    enableColumnFilter: rest.enableColumnFilter ?? Boolean(filterVariant),
    header:
      header ??
      (({ column }) => {
        const m = column.columnDef.meta as DataTableColumnMeta | undefined;
        return m?.label ?? m?.labelKey ?? column.id;
      }),
    meta: {
      ...meta,
      labelKey,
      label,
      filterVariant,
      filterOptions,
      editVariant,
      editOptions,
      gridPrimary,
      gridSecondary,
      defaultHidden,
      ...(param !== undefined ? { param } : {}),
      ...(paramFormat !== undefined ? { paramFormat } : {}),
      ...(sortParam !== undefined ? { sortParam } : {}),
      enableHiding: meta?.enableHiding ?? enableHiding ?? true,
    },
  } as ColumnDef<TData, TValue>;
}
