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
import { restrictToVerticalAxis } from "@dnd-kit/modifiers";
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  flexRender,
  type Cell,
  type Row,
  type Table,
} from "@tanstack/react-table";

import { ColumnFilter } from "@/components/tables/column-filter";
import { ColumnHeader } from "@/components/tables/column-header";
import {
  EditableCell,
  type CellEditPayload,
} from "@/components/tables/editable-cell";
import { RowDragProvider } from "@/components/tables/reorder-column";
import { isRowClickIgnored } from "@/components/tables/row-click";
import type {
  DataTableColumnMeta,
  TableDensity,
} from "@/components/tables/types";
import { cn } from "@/lib/utils";

const densityCell: Record<TableDensity, string> = {
  compact: "px-2 py-1.5",
  comfortable: "px-3 py-2.5",
  spacious: "px-4 py-3.5",
};

type EditingCell = { rowId: string; columnId: string } | null;

type TableEmptyMessage = {
  title: string;
  description?: string;
};

type TableViewProps<TData> = {
  table: Table<TData>;
  density: TableDensity;
  showColumnFilters?: boolean;
  emptyMessage?: TableEmptyMessage | null;
  columnCount?: number;
  reorderMode?: boolean;
  onReorder?: (activeId: string, overId: string) => void;
  inlineEdit?: boolean;
  editingCell?: EditingCell;
  onEditingCellChange?: (cell: EditingCell) => void;
  onCellEdit?: (payload: CellEditPayload<TData>) => void;
  onRowClick?: (row: TData) => void;
};

export function TableView<TData>({
  table,
  density,
  showColumnFilters,
  emptyMessage,
  columnCount,
  reorderMode,
  onReorder,
  inlineEdit,
  editingCell,
  onEditingCellChange,
  onCellEdit,
  onRowClick,
}: TableViewProps<TData>) {
  const cellPad = densityCell[density];
  const leafColumns = table.getVisibleLeafColumns();
  const leafHeaders =
    table.getHeaderGroups()[table.getHeaderGroups().length - 1]?.headers ?? [];
  const rows = table.getRowModel().rows;
  const colSpan = columnCount ?? leafColumns.length;

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    onReorder?.(String(active.id), String(over.id));
  };

  const tableNode = (
    <div className="border-border w-full overflow-x-auto rounded-lg border">
      {/*
        table-auto so the actions column can grow with icon buttons.
        Explicit widths still apply for select/reorder and user-resized cols.
      */}
      <table className="w-full caption-bottom text-sm">
        <colgroup>
          {leafColumns.map((column) => {
            const sizing = table.getState().columnSizing;
            const hasCustomSize = column.id in sizing;
            const isActions = column.id === "actions";
            const fixed =
              column.id === "__select" || column.id === "__reorder"
                ? 44
                : undefined;
            return (
              <col
                key={column.id}
                style={{
                  // 1% + nowrap (on cell) = shrink-to-fit content under table-auto
                  width: isActions
                    ? "1%"
                    : hasCustomSize
                      ? column.getSize()
                      : fixed,
                }}
              />
            );
          })}
        </colgroup>

        <thead className="bg-muted/40">
          <tr className="border-border border-b">
            {leafHeaders.map((header) => {
              const meta = header.column.columnDef.meta as
                DataTableColumnMeta | undefined;
              const pinned = header.column.getIsPinned();
              return (
                <th
                  key={header.id}
                  className={cn(
                    "text-muted-foreground relative text-start align-middle font-medium",
                    cellPad,
                    meta?.headerClassName,
                    header.column.id === "actions" && "whitespace-nowrap",
                    pinned === "left" && "bg-muted/95 sticky start-0 z-20",
                    pinned === "right" && "bg-muted/95 sticky end-0 z-20",
                  )}
                  style={{
                    insetInlineStart:
                      pinned === "left"
                        ? `${header.column.getStart("left")}px`
                        : undefined,
                    insetInlineEnd:
                      pinned === "right"
                        ? `${header.column.getAfter("right")}px`
                        : undefined,
                  }}
                >
                  {header.isPlaceholder ? null : header.column.id ===
                      "__select" || header.column.id === "__reorder" ? (
                    flexRender(
                      header.column.columnDef.header,
                      header.getContext(),
                    )
                  ) : (
                    <ColumnHeader
                      column={header.column}
                      table={table}
                      disableSort={reorderMode}
                    />
                  )}
                  {header.column.getCanResize() && !reorderMode ? (
                    <div
                      onMouseDown={header.getResizeHandler()}
                      onTouchStart={header.getResizeHandler()}
                      className={cn(
                        "bg-border absolute end-0 top-0 h-full w-1 cursor-col-resize touch-none opacity-0 select-none hover:opacity-100",
                        header.column.getIsResizing() &&
                          "bg-primary opacity-100",
                      )}
                    />
                  ) : null}
                </th>
              );
            })}
          </tr>

          {showColumnFilters && !reorderMode ? (
            <tr className="border-border border-b">
              {leafHeaders.map((header) => {
                const pinned = header.column.getIsPinned();
                return (
                  <th
                    key={`filter-${header.id}`}
                    className={cn(
                      "bg-background align-top font-normal",
                      cellPad,
                      pinned === "left" && "bg-background sticky start-0 z-20",
                      pinned === "right" && "bg-background sticky end-0 z-20",
                    )}
                    style={{
                      insetInlineStart:
                        pinned === "left"
                          ? `${header.column.getStart("left")}px`
                          : undefined,
                      insetInlineEnd:
                        pinned === "right"
                          ? `${header.column.getAfter("right")}px`
                          : undefined,
                    }}
                  >
                    <div className="min-w-0">
                      <ColumnFilter column={header.column} />
                    </div>
                  </th>
                );
              })}
            </tr>
          ) : null}
        </thead>

        <tbody>
          {rows.length === 0 && emptyMessage ? (
            <tr>
              <td
                colSpan={Math.max(colSpan, 1)}
                className={cn("text-center", cellPad, "py-12")}
              >
                <div className="mx-auto flex max-w-sm flex-col items-center gap-1">
                  <p className="text-foreground text-sm font-medium">
                    {emptyMessage.title}
                  </p>
                  {emptyMessage.description ? (
                    <p className="text-muted-foreground text-sm">
                      {emptyMessage.description}
                    </p>
                  ) : null}
                </div>
              </td>
            </tr>
          ) : (
            rows.map((row) =>
              reorderMode ? (
                <SortableTableRow
                  key={row.id}
                  row={row}
                  table={table}
                  cellPad={cellPad}
                  inlineEdit={false}
                  editingCell={null}
                />
              ) : (
                <StaticTableRow
                  key={row.id}
                  row={row}
                  table={table}
                  cellPad={cellPad}
                  inlineEdit={inlineEdit}
                  editingCell={editingCell}
                  onEditingCellChange={onEditingCellChange}
                  onCellEdit={onCellEdit}
                  onRowClick={onRowClick}
                />
              ),
            )
          )}
        </tbody>
      </table>
    </div>
  );

  if (!reorderMode) {
    return tableNode;
  }

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      modifiers={[restrictToVerticalAxis]}
      onDragEnd={handleDragEnd}
    >
      <SortableContext
        items={rows.map((row) => row.id)}
        strategy={verticalListSortingStrategy}
      >
        {tableNode}
      </SortableContext>
    </DndContext>
  );
}

type RowProps<TData> = {
  row: Row<TData>;
  table: Table<TData>;
  cellPad: string;
  inlineEdit?: boolean;
  editingCell?: EditingCell;
  onEditingCellChange?: (cell: EditingCell) => void;
  onCellEdit?: (payload: CellEditPayload<TData>) => void;
  onRowClick?: (row: TData) => void;
};

function StaticTableRow<TData>({
  row,
  table,
  cellPad,
  inlineEdit,
  editingCell,
  onEditingCellChange,
  onCellEdit,
  onRowClick,
}: RowProps<TData>) {
  return (
    <tr
      className={cn(
        "border-border hover:bg-muted/30 data-[state=selected]:bg-muted/50 border-b last:border-0",
        onRowClick && "cursor-pointer",
      )}
      data-state={row.getIsSelected() ? "selected" : undefined}
      onClick={
        onRowClick
          ? (event) => {
              if (isRowClickIgnored(event)) return;
              onRowClick(row.original);
            }
          : undefined
      }
    >
      {row.getVisibleCells().map((cell) => (
        <TableCell
          key={cell.id}
          cell={cell}
          table={table}
          cellPad={cellPad}
          inlineEdit={inlineEdit}
          editingCell={editingCell}
          onEditingCellChange={onEditingCellChange}
          onCellEdit={onCellEdit}
          row={row}
        />
      ))}
    </tr>
  );
}

function SortableTableRow<TData>({ row, table, cellPad }: RowProps<TData>) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: row.id });

  // Lock visual movement to Y axis (belt + modifiers)
  const style = {
    transform: CSS.Transform.toString(
      transform ? { ...transform, x: 0 } : null,
    ),
    transition,
  };

  return (
    <tr
      ref={setNodeRef}
      style={style}
      className={cn(
        "border-border bg-background border-b last:border-0",
        isDragging
          ? "bg-muted/40 relative z-20 shadow-sm"
          : "hover:bg-muted/30",
      )}
      data-state={row.getIsSelected() ? "selected" : undefined}
    >
      <RowDragProvider value={{ attributes, listeners, isDragging }}>
        {row.getVisibleCells().map((cell) => (
          <TableCell
            key={cell.id}
            cell={cell}
            table={table}
            cellPad={cellPad}
            inlineEdit={false}
            row={row}
          />
        ))}
      </RowDragProvider>
    </tr>
  );
}

function TableCell<TData>({
  cell,
  table,
  cellPad,
  inlineEdit,
  editingCell,
  onEditingCellChange,
  onCellEdit,
  row,
}: {
  cell: Cell<TData, unknown>;
  table: Table<TData>;
  cellPad: string;
  inlineEdit?: boolean;
  editingCell?: EditingCell;
  onEditingCellChange?: (cell: EditingCell) => void;
  onCellEdit?: (payload: CellEditPayload<TData>) => void;
  row: Row<TData>;
}) {
  const meta = cell.column.columnDef.meta as DataTableColumnMeta | undefined;
  const pinned = cell.column.getIsPinned();
  const isEditing =
    editingCell?.rowId === row.id && editingCell?.columnId === cell.column.id;

  return (
    <td
      className={cn(
        "align-middle",
        cellPad,
        meta?.cellClassName,
        cell.column.id === "actions" && "whitespace-nowrap",
        pinned === "left" && "bg-background sticky start-0 z-10",
        pinned === "right" && "bg-background sticky end-0 z-10",
      )}
      style={{
        insetInlineStart:
          pinned === "left" ? `${cell.column.getStart("left")}px` : undefined,
        insetInlineEnd:
          pinned === "right" ? `${cell.column.getAfter("right")}px` : undefined,
      }}
    >
      <div
        className={cn(
          cell.column.id === "actions"
            ? "overflow-visible"
            : "min-w-0 overflow-hidden",
        )}
      >
        <EditableCell
          cell={cell}
          table={table}
          enabled={Boolean(inlineEdit)}
          editing={Boolean(isEditing)}
          onStartEdit={() =>
            onEditingCellChange?.({
              rowId: row.id,
              columnId: cell.column.id,
            })
          }
          onCancelEdit={() => onEditingCellChange?.(null)}
          onCommit={(value) => {
            onCellEdit?.({
              rowId: row.id,
              columnId: cell.column.id,
              value,
              row: row.original,
            });
            onEditingCellChange?.(null);
          }}
        >
          {flexRender(cell.column.columnDef.cell, cell.getContext())}
        </EditableCell>
      </div>
    </td>
  );
}
