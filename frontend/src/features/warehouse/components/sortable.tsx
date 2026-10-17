"use client";

import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { GripVertical } from "lucide-react";
import type { CSSProperties, ReactNode } from "react";

import { moveUuid } from "@/features/warehouse/lib/reorder";
import { cn } from "@/lib/utils";

/**
 * Drag and drop of warehouses, rooms and sibling locations (TEC-764),
 * on the project's dnd-kit: pointer drag on the grip, or keyboard (focus
 * the grip, Space to lift, arrows to move, Space to drop).
 *
 * `SortableArea` is one DndContext; every `SortableGroup` inside is a list
 * of siblings. A drop only reorders inside one group; `onMove` receives the
 * group key and the group's new uuid order.
 */
export function SortableArea({
  onMove,
  children,
}: {
  onMove: (group: string, order: string[]) => void;
  children: ReactNode;
}) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over) return;
    const from = active.data.current?.sortable as
      { containerId: string; items: string[] } | undefined;
    const to = over.data.current?.sortable as
      { containerId: string } | undefined;
    if (!from || !to || from.containerId !== to.containerId) return;
    const order = moveUuid(from.items, String(active.id), String(over.id));
    if (order) onMove(from.containerId, order);
  };
  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragEnd={onDragEnd}
    >
      {children}
    </DndContext>
  );
}

/** One sibling list; `group` comes back in SortableArea's onMove. */
export function SortableGroup({
  group,
  ids,
  children,
}: {
  group: string;
  ids: string[];
  children: ReactNode;
}) {
  return (
    <SortableContext
      id={group}
      items={ids}
      strategy={verticalListSortingStrategy}
    >
      {children}
    </SortableContext>
  );
}

export type SortableRowProps = {
  setNodeRef: (el: HTMLElement | null) => void;
  style: CSSProperties;
  isDragging: boolean;
  /** The grip; null when sorting is off. */
  handle: ReactNode;
};

/** A sortable row; `children` renders it with the ref, style and grip. */
export function SortableRow({
  id,
  disabled,
  handleLabel,
  children,
}: {
  id: string;
  disabled?: boolean;
  handleLabel: string;
  children: (row: SortableRowProps) => ReactNode;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id, disabled });
  return children({
    setNodeRef,
    style: {
      transform: CSS.Translate.toString(transform),
      transition,
    },
    isDragging,
    handle: disabled ? null : (
      <button
        type="button"
        ref={setActivatorNodeRef}
        className={cn(
          "text-muted-foreground hover:text-foreground shrink-0 cursor-grab touch-none rounded p-0.5",
          isDragging && "cursor-grabbing",
        )}
        aria-label={handleLabel}
        title={handleLabel}
        data-testid="sort-handle"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="size-4" />
      </button>
    ),
  });
}
