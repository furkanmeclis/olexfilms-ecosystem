import type { MyWarehouses } from "@/features/warehouse/services/warehouse.service";

/** localStorage key of the panel's active warehouse (per organization). */
export function activeWarehouseKey(slug: string): string {
  return `olex.warehouse.active.${slug}`;
}

/**
 * The effective active warehouse (TEC-769): the stored pick when the
 * caller may still write there, else the member default, else the first
 * writable warehouse; null when there is none.
 */
export function resolveActiveWarehouse(
  ctx: MyWarehouses | undefined,
  stored: string | null,
): string | null {
  if (!ctx) return null;
  const writable = ctx.items.filter((w) => w.member);
  if (stored && writable.some((w) => w.uuid === stored)) return stored;
  if (
    ctx.default_warehouse_uuid &&
    writable.some((w) => w.uuid === ctx.default_warehouse_uuid)
  ) {
    return ctx.default_warehouse_uuid;
  }
  return writable[0]?.uuid ?? null;
}
