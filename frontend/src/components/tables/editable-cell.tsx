"use client";

import type { Cell, Table } from "@tanstack/react-table";
import { useEffect, useRef, useState, type ReactNode } from "react";

import type {
  ColumnEditVariant,
  DataTableColumnMeta,
} from "@/components/tables/types";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export type CellEditPayload<TData> = {
  rowId: string;
  columnId: string;
  value: unknown;
  row: TData;
};

type EditableCellProps<TData> = {
  cell: Cell<TData, unknown>;
  table?: Table<TData>;
  children: ReactNode;
  enabled: boolean;
  editing: boolean;
  onStartEdit: () => void;
  onCancelEdit: () => void;
  onCommit: (value: unknown) => void;
};

function resolveEditVariant(
  meta: DataTableColumnMeta | undefined,
): ColumnEditVariant | undefined {
  if (!meta?.editVariant) return undefined;
  return meta.editVariant;
}

export function EditableCell<TData>({
  cell,
  children,
  enabled,
  editing,
  onStartEdit,
  onCancelEdit,
  onCommit,
}: EditableCellProps<TData>) {
  const { t } = useLocale();
  const meta = cell.column.columnDef.meta as DataTableColumnMeta | undefined;
  const variant = resolveEditVariant(meta);
  const editable = enabled && Boolean(variant);

  if (!editable) {
    return <>{children}</>;
  }

  if (!editing) {
    return (
      <div
        role="button"
        tabIndex={0}
        className="focus-visible:ring-ring min-w-0 cursor-text rounded-sm outline-none focus-visible:ring-2"
        onDoubleClick={(e) => {
          e.stopPropagation();
          onStartEdit();
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            onStartEdit();
          }
        }}
        title={t("table.double_click_to_edit")}
      >
        {children}
      </div>
    );
  }

  return (
    <CellEditor
      variant={variant!}
      options={meta?.editOptions}
      initialValue={cell.getValue()}
      onCancel={onCancelEdit}
      onCommit={onCommit}
    />
  );
}

function CellEditor({
  variant,
  options,
  initialValue,
  onCancel,
  onCommit,
}: {
  variant: ColumnEditVariant;
  options?: DataTableColumnMeta["editOptions"];
  initialValue: unknown;
  onCancel: () => void;
  onCommit: (value: unknown) => void;
}) {
  const { t } = useLocale();
  const [value, setValue] = useState(initialValue);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  if (variant === "boolean") {
    return (
      <div className="flex items-center gap-2 py-0.5">
        <Checkbox
          checked={Boolean(value)}
          onCheckedChange={(checked) => {
            const next = Boolean(checked);
            setValue(next);
            onCommit(next);
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape") onCancel();
          }}
        />
        <span className="text-muted-foreground text-xs">
          {Boolean(value) ? t("table.true") : t("table.false")}
        </span>
      </div>
    );
  }

  if (variant === "select") {
    return (
      <Select
        value={String(value ?? "")}
        onValueChange={(next) => {
          setValue(next);
          onCommit(next);
        }}
        open
        onOpenChange={(open) => {
          if (!open) onCancel();
        }}
      >
        <SelectTrigger className="h-8 w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {(options ?? []).map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  return (
    <Input
      ref={inputRef}
      type={variant === "number" ? "number" : "text"}
      className={cn("h-8")}
      value={value == null ? "" : String(value)}
      onChange={(e) => {
        if (variant === "number") {
          const n = e.target.value === "" ? "" : Number(e.target.value);
          setValue(n);
        } else {
          setValue(e.target.value);
        }
      }}
      onBlur={() => {
        const next =
          variant === "number"
            ? value === "" || value == null
              ? 0
              : Number(value)
            : value;
        onCommit(next);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          (e.target as HTMLInputElement).blur();
        }
        if (e.key === "Escape") {
          e.preventDefault();
          onCancel();
        }
      }}
    />
  );
}
