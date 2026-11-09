/**
 * Warehouse scope of the depo screens (TEC-1049). The warehouse picked on the
 * device must reach every list that the API can filter by warehouse, and its
 * uuid must be part of each query key: a key without it keeps serving the
 * previous warehouse's cache after a switch. Pure, no `@/` imports, so
 * node --test runs it directly.
 */

type Pickable = { uuid: string; is_primary?: boolean | null };

/**
 * Current warehouse: the saved one while it is still in the active list,
 * else the primary one, else the first.
 */
export function pickWarehouse<T extends Pickable>(list: readonly T[], saved: string | undefined): T | undefined {
  return list.find((w) => w.uuid === saved) ?? list.find((w) => w.is_primary) ?? list[0];
}

/** Direction of a transfer seen from the current warehouse. */
export type TransferDirection = 'out' | 'in';

/** Query keys of the warehouse feature; every scoped list carries the warehouse uuid. */
export const wk = {
  all: ['warehouse'] as const,
  warehouses: () => ['warehouse', 'warehouses'] as const,
  rooms: (warehouse: string) => ['warehouse', 'rooms', warehouse] as const,
  transfers: (status: string, warehouse = '', direction: TransferDirection = 'out') =>
    ['warehouse', 'transfers', warehouse, direction, status] as const,
  transfer: (uuid: string) => ['warehouse', 'transfer', uuid] as const,
  counts: (status: string, warehouse = '') => ['warehouse', 'counts', warehouse, status] as const,
  count: (uuid: string) => ['warehouse', 'count', uuid] as const,
  countScans: (uuid: string) => ['warehouse', 'count', uuid, 'scans'] as const,
  countReport: (uuid: string) => ['warehouse', 'count', uuid, 'report'] as const,
  entries: (status: string, warehouse = '') => ['warehouse', 'entries', warehouse, status] as const,
  entry: (uuid: string) => ['warehouse', 'entry', uuid] as const,
  summary: (warehouse = '') => ['warehouse', 'summary', warehouse] as const,
  /** Prefix of every warehouse's summary, for invalidation after a write. */
  summaries: () => ['warehouse', 'summary'] as const,
};

/** Stock query keys of the warehouse space (units that originate from one warehouse, TEC-1006). */
export const stockKeys = {
  warehouseUnits: (warehouse: string, q: string, product = '') => ['stock', 'warehouse-units', warehouse, product, q] as const,
};

/**
 * Whether a previous query key (from `stockKeys.warehouseUnits` or a scoped
 * `wk` list) belongs to `warehouse`: placeholder data may only come from the
 * same warehouse, else a switch would keep showing the old one's rows.
 */
export function sameWarehouse(key: readonly unknown[] | undefined, warehouse: string | undefined): boolean {
  return !!key && key[2] === (warehouse ?? '');
}

/** `warehouse_uuid` filter of a list call; nothing when no warehouse is known yet. */
export function warehouseFilter(warehouse: string | undefined): { warehouse_uuid?: string } {
  return warehouse ? { warehouse_uuid: warehouse } : {};
}

/** Transfer filter: outgoing lists `from_warehouse_uuid`, incoming `to_warehouse_uuid`. */
export function transferFilter(
  warehouse: string | undefined,
  direction: TransferDirection,
): { from_warehouse_uuid?: string; to_warehouse_uuid?: string } {
  if (!warehouse) return {};
  return direction === 'in' ? { to_warehouse_uuid: warehouse } : { from_warehouse_uuid: warehouse };
}
