"use client";

import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { restrictToVerticalAxis } from "@dnd-kit/modifiers";
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { Columns3, GripVertical, RotateCcw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Switch } from "@/components/ui/switch";
import { useLocale } from "@/providers/locale-provider";

type Item = { id: string; label: string };

type Props = {
  items: Item[];
  order: string[];
  hidden: string[];
  pinFirst: boolean;
  onOrder: (order: string[]) => void;
  onHidden: (hidden: string[]) => void;
  onPinFirst: (pin: boolean) => void;
  onReset: () => void;
};

function Row({
  item,
  visible,
  onToggle,
  disableHide,
}: {
  item: Item;
  visible: boolean;
  onToggle: (v: boolean) => void;
  disableHide: boolean;
}) {
  const { t } = useLocale();
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: item.id });
  const id = `col-${item.id}`;
  return (
    <li
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.6 : 1,
      }}
      className="bg-popover flex items-center gap-2 rounded-md py-1 pe-1"
    >
      <button
        type="button"
        className="text-muted-foreground hover:text-foreground cursor-grab touch-none rounded p-0.5"
        aria-label={t("chart.grid.drag", { column: item.label })}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="size-4" aria-hidden />
      </button>
      <Checkbox
        id={id}
        checked={visible}
        disabled={disableHide && visible}
        onCheckedChange={(c) => onToggle(Boolean(c))}
      />
      <label htmlFor={id} className="min-w-0 flex-1 truncate text-sm">
        {item.label}
      </label>
    </li>
  );
}

/** Columns popover: show / hide, drag to reorder, pin first column, reset. */
export function ColumnSettings({
  items,
  order,
  hidden,
  pinFirst,
  onOrder,
  onHidden,
  onPinFirst,
  onReset,
}: Props) {
  const { t } = useLocale();
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );
  const byId = new Map(items.map((i) => [i.id, i]));
  const ordered = order.map((id) => byId.get(id)).filter(Boolean) as Item[];
  const visibleCount = ordered.filter((i) => !hidden.includes(i.id)).length;

  const onDragEnd = (e: DragEndEvent) => {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const from = order.indexOf(String(active.id));
    const to = order.indexOf(String(over.id));
    if (from < 0 || to < 0) return;
    onOrder(arrayMove(order, from, to));
  };

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" className="h-8">
          <Columns3 aria-hidden />
          {t("chart.grid.columns")}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 p-3">
        <div className="flex flex-col gap-3">
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            modifiers={[restrictToVerticalAxis]}
            onDragEnd={onDragEnd}
          >
            <SortableContext
              items={order}
              strategy={verticalListSortingStrategy}
            >
              <ul
                className="flex max-h-72 flex-col overflow-auto"
                data-slot="grid-columns"
              >
                {ordered.map((item) => (
                  <Row
                    key={item.id}
                    item={item}
                    visible={!hidden.includes(item.id)}
                    disableHide={visibleCount <= 1}
                    onToggle={(v) =>
                      onHidden(
                        v
                          ? hidden.filter((h) => h !== item.id)
                          : [...hidden, item.id],
                      )
                    }
                  />
                ))}
              </ul>
            </SortableContext>
          </DndContext>
          <label className="flex items-center justify-between gap-2 text-sm">
            {t("chart.grid.pin_first")}
            <Switch checked={pinFirst} onCheckedChange={onPinFirst} />
          </label>
          <Button type="button" size="sm" variant="ghost" onClick={onReset}>
            <RotateCcw aria-hidden />
            {t("chart.grid.reset")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
