import { arrayMove } from "@dnd-kit/sortable";

/** Reorder a data array by moving `activeId` to the position of `overId`. */
export function reorderRowsById<TData>(
  data: TData[],
  getRowId: (row: TData, index: number) => string,
  activeId: string,
  overId: string,
): TData[] {
  if (activeId === overId) return data;
  const oldIndex = data.findIndex(
    (row, index) => getRowId(row, index) === activeId,
  );
  const newIndex = data.findIndex(
    (row, index) => getRowId(row, index) === overId,
  );
  if (oldIndex < 0 || newIndex < 0) return data;
  return arrayMove(data, oldIndex, newIndex);
}
