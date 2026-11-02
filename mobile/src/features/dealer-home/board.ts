import type { components } from '@/core/api/schema';
import type { IconName } from '@/ui/icons';

type ServiceStatus = components['schemas']['ServiceStatus'];

/**
 * Dealer home "board": the services a dealer works on, in three groups.
 * Pure logic only (grouping, ordering, tag de-duplication, KPI icons) so it
 * runs under node:test without React Native.
 */

export type BoardGroupKey = 'next' | 'active' | 'done';

export type BoardGroup = {
  key: BoardGroupKey;
  statuses: ServiceStatus[];
  /** `oldest`: the job waiting longest first; `newest`: the latest finished first. */
  order: 'oldest' | 'newest';
  /** API sort (docs/list-contract.md), same direction as `order`. */
  sort: string;
  /** Rows shown on the home screen; the header shows the full `total`. */
  limit: number;
};

export const BOARD_GROUPS: readonly BoardGroup[] = [
  { key: 'next', statuses: ['draft', 'pending'], order: 'oldest', sort: 'created_at', limit: 5 },
  { key: 'active', statuses: ['processing', 'ready'], order: 'oldest', sort: 'created_at', limit: 5 },
  { key: 'done', statuses: ['completed'], order: 'newest', sort: '-completed_at', limit: 3 },
];

/** Comma separated `status` filter of GET /v1/services (any of). */
export function statusParam(group: BoardGroup) {
  return group.statuses.join(',');
}

/**
 * Status the services list can be opened with: the list screen filters by one
 * status, so only single-status groups get a filtered "see all".
 */
export function listFilterOf(group: BoardGroup): ServiceStatus | undefined {
  return group.statuses.length === 1 ? group.statuses[0] : undefined;
}

/** Board group of a status; cancelled services are not on the board. */
export function groupOf(status: string): BoardGroupKey | undefined {
  return BOARD_GROUPS.find((g) => (g.statuses as string[]).includes(status))?.key;
}

type Sortable = { uuid: string; status: string; created_at: string; completed_at?: string | null };

function time(iso: string | null | undefined) {
  const t = iso ? Date.parse(iso) : NaN;
  return Number.isNaN(t) ? 0 : t;
}

/** Orders rows for a group; the uuid breaks ties so the order is stable. */
export function sortForGroup<T extends Sortable>(rows: readonly T[], order: BoardGroup['order']): T[] {
  const tie = (a: T, b: T) => (a.uuid < b.uuid ? -1 : a.uuid > b.uuid ? 1 : 0);
  if (order === 'oldest') return [...rows].sort((a, b) => time(a.created_at) - time(b.created_at) || tie(a, b));
  return [...rows].sort((a, b) => time(b.completed_at ?? b.created_at) - time(a.completed_at ?? a.created_at) || tie(a, b));
}

/** Rows of one group: only its statuses, in its order, at most `limit`. */
export function boardRows<T extends Sortable>(rows: readonly T[], group: BoardGroup): T[] {
  return sortForGroup(
    rows.filter((r) => groupOf(r.status) === group.key),
    group.order,
  ).slice(0, group.limit);
}

/** Splits any list of services into the three groups (cancelled dropped). */
export function groupServices<T extends Sortable>(rows: readonly T[]): Record<BoardGroupKey, T[]> {
  return {
    next: boardRows(rows, BOARD_GROUPS[0]),
    active: boardRows(rows, BOARD_GROUPS[1]),
    done: boardRows(rows, BOARD_GROUPS[2]),
  };
}

/**
 * Category names of the applied products (`item_categories` of GET /v1/services),
 * each once (trimmed, case-insensitive), first spelling kept, in the API order.
 * An older backend omits the field: then the row simply has no tags.
 */
export function uniqueCategories(service: { item_categories?: readonly (string | null | undefined)[] | null }): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  const names = Array.isArray(service.item_categories) ? service.item_categories : [];
  for (const raw of names) {
    const name = typeof raw === 'string' ? raw.trim() : '';
    if (!name) continue;
    const key = name.toLocaleLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(name);
  }
  return out;
}

/** Tags that fit on a row, plus how many are folded into "+N". */
export function fitTags(labels: readonly string[], max = 3): { shown: string[]; more: number } {
  if (labels.length <= max) return { shown: [...labels], more: 0 };
  // Keep a slot for the "+N" chip so the row never grows past `max` chips.
  const keep = Math.max(1, max - 1);
  return { shown: labels.slice(0, keep), more: labels.length - keep };
}

/** Icon of an overview card (backend reports/usecase/builders.go card keys). */
const KPI_ICON_TABLE: Record<string, IconName> = {
  services_created: 'service',
  services_completed: 'check',
  services_open: 'clock',
  orders_created: 'order',
  orders_open: 'cart',
  customers_total: 'customers',
  customers_new: 'lead',
  warranties_active: 'warranty',
  warranties_expiring: 'calendar',
  warranties_started: 'shield',
  warranties_expired: 'clock',
  warranties_void: 'blocked',
  stock_products: 'package',
  stock_quantity: 'stock',
  stock_meters: 'measure',
  measurements_total: 'gauge',
  measurements_linked: 'gauge',
  measurements_vehicles: 'vehicle',
  dealers_total: 'network',
  dealers_active: 'building',
};
// A Map, so inherited names like "toString" never match.
const KPI_ICONS = new Map(Object.entries(KPI_ICON_TABLE));

/** Neutral chart icon for a card key the app does not know yet. */
export function kpiIcon(key: string): IconName {
  return KPI_ICONS.get(key) ?? 'chart';
}
