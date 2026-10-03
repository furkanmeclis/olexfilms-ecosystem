import type {
  WarehouseLocation,
  WarehouseLocationType,
} from "@/features/warehouse/services/warehouse.service";

export type LocationNode = WarehouseLocation & { children: LocationNode[] };

export const LOCATION_TYPES: WarehouseLocationType[] = [
  "aisle",
  "shelf",
  "bin",
];

/**
 * Types a new location may have under `parent` (null = the room root),
 * mirroring the TEC-201 rule: aisle at the room root; shelf at the room
 * root or under an aisle; bin under a shelf. A bin takes no children.
 */
export function allowedChildTypes(
  parent: WarehouseLocationType | null,
): WarehouseLocationType[] {
  switch (parent) {
    case null:
      return ["aisle", "shelf"];
    case "aisle":
      return ["shelf"];
    case "shelf":
      return ["bin"];
    default:
      return [];
  }
}

const bySortThenCode = (a: WarehouseLocation, b: WarehouseLocation) =>
  a.sort_order - b.sort_order || a.code.localeCompare(b.code);

/**
 * Builds the tree from the flat room list (parent_uuid, siblings by
 * sort_order). A node whose parent is missing from the list is kept at the
 * root so nothing disappears from the view.
 */
export function buildLocationTree(items: WarehouseLocation[]): LocationNode[] {
  const nodes = new Map<string, LocationNode>();
  for (const item of items) nodes.set(item.uuid, { ...item, children: [] });
  const roots: LocationNode[] = [];
  for (const node of nodes.values()) {
    const parent = node.parent_uuid ? nodes.get(node.parent_uuid) : undefined;
    if (parent && parent !== node) parent.children.push(node);
    else roots.push(node);
  }
  const sort = (list: LocationNode[]) => {
    list.sort(bySortThenCode);
    for (const n of list) sort(n.children);
  };
  sort(roots);
  return roots;
}

/** Number of nodes below `node`. */
export function countDescendants(node: LocationNode): number {
  return node.children.reduce((sum, c) => sum + 1 + countDescendants(c), 0);
}

/**
 * Keeps the nodes whose code, full_code or name contains `query` and
 * every ancestor of such a node (case-insensitive). An empty query keeps
 * the whole tree.
 */
export function filterLocationTree(
  roots: LocationNode[],
  query: string,
): LocationNode[] {
  const q = query.trim().toLowerCase();
  if (!q) return roots;
  const walk = (list: LocationNode[]): LocationNode[] =>
    list.flatMap((n) => {
      const children = walk(n.children);
      const hit = [n.code, n.full_code, n.name].some((v) =>
        v.toLowerCase().includes(q),
      );
      return hit || children.length > 0 ? [{ ...n, children }] : [];
    });
  return walk(roots);
}

/** Uuids of every node that has children (to expand all). */
export function branchUuids(roots: LocationNode[]): string[] {
  const out: string[] = [];
  const walk = (list: LocationNode[]) => {
    for (const n of list) {
      if (n.children.length > 0) {
        out.push(n.uuid);
        walk(n.children);
      }
    }
  };
  walk(roots);
  return out;
}
