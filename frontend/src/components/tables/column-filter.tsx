"use client";

import type { Column } from "@tanstack/react-table";
import { X } from "lucide-react";

import type { DataTableColumnMeta } from "@/components/tables/types";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLocale } from "@/providers/locale-provider";

type ColumnFilterProps<TData> = {
  column: Column<TData, unknown>;
};

export function ColumnFilter<TData>({ column }: ColumnFilterProps<TData>) {
  const { t } = useLocale();
  if (!column.getCanFilter()) return null;

  const meta = column.columnDef.meta as DataTableColumnMeta | undefined;
  const variant = meta?.filterVariant ?? "text";
  // Faceted filters live in the toolbar (quick filters)
  if (variant === "faceted") return null;

  const value = column.getFilterValue();

  if (variant === "select") {
    type FilterOption = {
      value: string;
      label: string;
      labelKey?: string;
    };
    const options = (meta?.filterOptions ?? []) as FilterOption[];
    const faceted = Array.from(column.getFacetedUniqueValues().keys()).map(
      String,
    );
    const list: FilterOption[] =
      options.length > 0
        ? options
        : faceted.map((v) => ({ value: v, label: v }));

    return (
      <Select
        value={(value as string) ?? "__all__"}
        onValueChange={(next) =>
          column.setFilterValue(next === "__all__" ? undefined : next)
        }
      >
        <SelectTrigger className="h-8 w-full min-w-0">
          <SelectValue placeholder={t("table.filter")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__all__">{t("table.all")}</SelectItem>
          {list.map((opt) => (
            <SelectItem key={opt.value} value={opt.value}>
              {opt.labelKey ? t(opt.labelKey) : opt.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  if (variant === "number-range") {
    const range = (value as [number | undefined, number | undefined]) ?? [
      undefined,
      undefined,
    ];
    return (
      <div className="grid min-w-0 grid-cols-2 gap-1">
        <Input
          type="number"
          className="h-8 min-w-0 px-2"
          placeholder={t("table.min")}
          value={range[0] ?? ""}
          onChange={(e) => {
            const min = e.target.value ? Number(e.target.value) : undefined;
            column.setFilterValue((old: [number?, number?] | undefined) => [
              min,
              old?.[1],
            ]);
          }}
        />
        <Input
          type="number"
          className="h-8 min-w-0 px-2"
          placeholder={t("table.max")}
          value={range[1] ?? ""}
          onChange={(e) => {
            const max = e.target.value ? Number(e.target.value) : undefined;
            column.setFilterValue((old: [number?, number?] | undefined) => [
              old?.[0],
              max,
            ]);
          }}
        />
      </div>
    );
  }

  if (variant === "boolean") {
    return (
      <Select
        value={value === true ? "true" : value === false ? "false" : "__all__"}
        onValueChange={(next) => {
          if (next === "__all__") column.setFilterValue(undefined);
          else column.setFilterValue(next === "true");
        }}
      >
        <SelectTrigger className="h-8 w-full min-w-0">
          <SelectValue placeholder={t("table.filter")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__all__">{t("table.all")}</SelectItem>
          <SelectItem value="true">{t("table.true")}</SelectItem>
          <SelectItem value="false">{t("table.false")}</SelectItem>
        </SelectContent>
      </Select>
    );
  }

  if (variant === "date-range") {
    const range = (value as
      [string | undefined, string | undefined] | undefined) ?? [
      undefined,
      undefined,
    ];
    return (
      <div className="grid min-w-[11rem] grid-cols-1 gap-1 xl:min-w-[14rem] xl:grid-cols-2">
        <DatePicker
          value={range[0] ?? ""}
          onChange={(next) =>
            column.setFilterValue((old: [string?, string?] | undefined) => [
              next || undefined,
              old?.[1],
            ])
          }
          placeholder={t("table.min")}
          className="h-8 w-full"
          displayFormat="dd.MM.yy"
        />
        <DatePicker
          value={range[1] ?? ""}
          onChange={(next) =>
            column.setFilterValue((old: [string?, string?] | undefined) => [
              old?.[0],
              next || undefined,
            ])
          }
          placeholder={t("table.max")}
          className="h-8 w-full"
          displayFormat="dd.MM.yy"
        />
      </div>
    );
  }

  const text = String(value ?? "");

  return (
    <InputGroup className="h-8 w-full min-w-0">
      <InputGroupInput
        className="h-8"
        placeholder={t("table.filter")}
        value={text}
        onChange={(e) => column.setFilterValue(e.target.value || undefined)}
      />
      {text ? (
        <InputGroupAddon align="inline-end">
          <InputGroupButton
            size="icon-xs"
            aria-label={t("table.clear_filter")}
            onClick={() => column.setFilterValue(undefined)}
          >
            <X className="size-3.5" />
          </InputGroupButton>
        </InputGroupAddon>
      ) : null}
    </InputGroup>
  );
}
