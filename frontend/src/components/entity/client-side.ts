import type { FilterFn } from "@tanstack/react-table";

import type { DataTableManual } from "@/components/tables/types";

/**
 * Small lists that return the full array (no paging/sort on the endpoint):
 * EntityTable sorts, filters and pages in the browser.
 */
export const CLIENT_SIDE_MANUAL: DataTableManual = {
  sorting: false,
  filtering: false,
  pagination: false,
};

/** Client-side filter for `filterVariant: "date-range"` ([from, to] dates). */
export const clientDateRangeFilter: FilterFn<unknown> = (
  row,
  columnId,
  filterValue: unknown,
) => {
  const [from, to] = Array.isArray(filterValue)
    ? (filterValue as (string | undefined)[])
    : [];
  if (!from && !to) return true;
  const raw = row.getValue(columnId);
  if (raw == null || raw === "") return false;
  const day = String(raw).slice(0, 10);
  if (from && day < from.slice(0, 10)) return false;
  if (to && day > to.slice(0, 10)) return false;
  return true;
};
