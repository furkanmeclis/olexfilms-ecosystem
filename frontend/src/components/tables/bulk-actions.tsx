"use client";

import type { Table } from "@tanstack/react-table";
import { ChevronDown, Layers } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export type DataTableBulkAction<TData> = {
  id: string;
  label: string;
  icon?: LucideIcon;
  onClick: (selected: TData[], table: Table<TData>) => void;
  destructive?: boolean;
  disabled?: boolean | ((selected: TData[]) => boolean);
};

export function resolveBulkActions<TData>(
  bulkActions:
    | DataTableBulkAction<TData>[]
    | ((
        selected: TData[],
        table: Table<TData>,
      ) => DataTableBulkAction<TData>[] | ReactNode)
    | undefined,
  selected: TData[],
  table: Table<TData>,
): DataTableBulkAction<TData>[] | ReactNode | null {
  if (!bulkActions) return null;
  if (typeof bulkActions === "function") {
    return bulkActions(selected, table);
  }
  return bulkActions;
}

type BulkActionsProps<TData> = {
  actions: DataTableBulkAction<TData>[];
  selected: TData[];
  table: Table<TData>;
};

/**
 * 1 action → labeled "Bulk" button (optional icon).
 * 2+ actions → dropdown with icons.
 */
export function DataTableBulkActions<TData>({
  actions,
  selected,
  table,
}: BulkActionsProps<TData>) {
  const { t } = useLocale();
  if (!selected.length || !actions.length) return null;

  const isDisabled = (action: DataTableBulkAction<TData>) =>
    typeof action.disabled === "function"
      ? action.disabled(selected)
      : Boolean(action.disabled);

  if (actions.length === 1) {
    const action = actions[0]!;
    const Icon = action.icon ?? Layers;
    return (
      <Button
        type="button"
        size="sm"
        variant={action.destructive ? "destructive" : "secondary"}
        disabled={isDisabled(action)}
        onClick={() => action.onClick(selected, table)}
        className="h-8 gap-1.5"
      >
        <Icon className="size-3.5" />
        {t("table.bulk")}
      </Button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="secondary"
          className="h-8 gap-1.5"
        >
          <Layers className="size-3.5" />
          {t("table.bulk")}
          <ChevronDown className="size-3.5 opacity-70" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-44">
        {actions.map((action) => {
          const Icon = action.icon;
          return (
            <DropdownMenuItem
              key={action.id}
              disabled={isDisabled(action)}
              className={cn(
                action.destructive && "text-destructive focus:text-destructive",
              )}
              onClick={() => action.onClick(selected, table)}
            >
              {Icon ? <Icon className="size-4" /> : null}
              {action.label}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
