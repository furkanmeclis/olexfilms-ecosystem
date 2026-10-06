"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import {
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";

export type FilterOption = { value: string; label: string; labelKey?: string };

/** Filter options of an enum column (labels from `<prefix>.<value>`). */
export function enumFilterOptions(
  values: readonly string[],
  prefix: string,
): FilterOption[] {
  return values.map((value) => ({
    value,
    label: value,
    labelKey: `${prefix}.${value}`,
  }));
}

/**
 * Warehouse filter options ("CODE · Name", value = uuid) from the full
 * warehouse list (no paging, TEC-201); shares the form pickers' cache.
 */
export function useWarehouseFilterOptions(enabled: boolean): FilterOption[] {
  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
    enabled,
    staleTime: 5 * 60_000,
  });
  return useMemo(
    () =>
      (warehouses.data?.items ?? []).map((w) => ({
        value: w.uuid,
        label: `${w.code} · ${w.name}`,
      })),
    [warehouses.data?.items],
  );
}
