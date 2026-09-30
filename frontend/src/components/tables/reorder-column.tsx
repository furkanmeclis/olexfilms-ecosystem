"use client";

import type { ColumnDef, RowData } from "@tanstack/react-table";
import { GripVertical } from "lucide-react";
import {
  createContext,
  useContext,
  type HTMLAttributes,
  type ReactNode,
} from "react";

import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export const REORDER_COLUMN_ID = "__reorder";

type RowDragContextValue = {
  attributes: HTMLAttributes<HTMLElement>;
  listeners: Record<string, unknown> | undefined;
  isDragging: boolean;
};

const RowDragContext = createContext<RowDragContextValue | null>(null);

export function RowDragProvider({
  value,
  children,
}: {
  value: RowDragContextValue;
  children: ReactNode;
}) {
  return (
    <RowDragContext.Provider value={value}>{children}</RowDragContext.Provider>
  );
}

export function createReorderColumnDef<TData extends RowData>(): ColumnDef<
  TData,
  unknown
> {
  return {
    id: REORDER_COLUMN_ID,
    size: 36,
    enableSorting: false,
    enableHiding: false,
    enableColumnFilter: false,
    enableResizing: false,
    enablePinning: false,
    meta: {
      labelKey: "table.reorder",
      enableHiding: false,
      cellClassName: "w-9 px-1",
      headerClassName: "w-9 px-1",
    },
    header: () => <ReorderHeader />,
    cell: () => <RowDragHandle />,
  };
}

function RowDragHandle() {
  const { t } = useLocale();
  const drag = useContext(RowDragContext);

  return (
    <button
      type="button"
      className={cn(
        "inline-flex size-7 items-center justify-center rounded-md border-0 bg-transparent p-0",
        "text-muted-foreground/70 transition-colors outline-none",
        "hover:text-foreground hover:bg-transparent",
        "focus-visible:ring-ring focus-visible:ring-2",
        "cursor-grab touch-none active:cursor-grabbing",
        "disabled:cursor-not-allowed disabled:opacity-40",
        drag?.isDragging && "text-foreground cursor-grabbing",
      )}
      aria-label={t("table.reorder_handle")}
      disabled={!drag}
      {...(drag?.attributes as object)}
      {...(drag?.listeners as object)}
    >
      <GripVertical className="size-4" strokeWidth={1.75} aria-hidden />
    </button>
  );
}

function ReorderHeader() {
  const { t } = useLocale();
  return <span className="sr-only">{t("table.reorder")}</span>;
}
