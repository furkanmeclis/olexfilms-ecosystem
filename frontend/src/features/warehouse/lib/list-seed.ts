import type { ColumnFiltersState } from "@tanstack/react-table";

/**
 * Seeds the `status` column filter of a warehouse list from `?status=a,b`
 * (TEC-1032: the hub KPIs link to filtered lists). Unknown values are
 * dropped; an empty result leaves the list unfiltered.
 */
export function statusFilterFromParam(
  param: string | null | undefined,
  allowed: readonly string[],
): ColumnFiltersState | undefined {
  if (!param) return undefined;
  const set = new Set<string>(allowed);
  const value = param
    .split(",")
    .map((v) => v.trim())
    .filter((v) => set.has(v));
  return value.length ? [{ id: "status", value }] : undefined;
}
