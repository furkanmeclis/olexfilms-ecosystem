import { arrayMove } from "@dnd-kit/sortable";

/**
 * Drag and drop order of warehouses, rooms and sibling locations
 * (TEC-764). The backend sets sort_order to the position in the uuid list
 * (POST /v1/warehouse/{warehouses,rooms,locations}/reorder).
 */

/**
 * The uuid order after moving `activeId` onto `overId`; null when nothing
 * moves (same spot or an id outside the list).
 */
export function moveUuid(
  uuids: readonly string[],
  activeId: string,
  overId: string,
): string[] | null {
  if (activeId === overId) return null;
  const from = uuids.indexOf(activeId);
  const to = uuids.indexOf(overId);
  if (from < 0 || to < 0) return null;
  return arrayMove([...uuids], from, to);
}

/**
 * Applies a saved order to a cached list (optimistic update): the listed
 * rows get sort_order = position and take, in the new order, the array
 * slots the listed rows held; other rows stay where they are.
 */
export function withSortOrder<T extends { uuid: string; sort_order: number }>(
  items: readonly T[],
  order: readonly string[],
): T[] {
  const pos = new Map(order.map((uuid, i) => [uuid, i]));
  const moved = order.flatMap((uuid) => {
    const it = items.find((x) => x.uuid === uuid);
    return it ? [{ ...it, sort_order: pos.get(uuid) ?? it.sort_order }] : [];
  });
  let k = 0;
  return items.map((it) => (pos.has(it.uuid) ? moved[k++] : it));
}
