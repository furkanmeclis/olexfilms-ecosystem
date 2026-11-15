"use client";

import { useState } from "react";
import { Filter } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { GridFilterKind } from "../types";
import type { RangeFilterValue, SelectFilterValue } from "./grid-utils";

export type FilterOption = { value: string; label: string };

type Props = {
  label: string;
  kind: GridFilterKind;
  value: unknown;
  options: FilterOption[];
  onChange: (value: unknown) => void;
};

function isActive(kind: GridFilterKind, value: unknown): boolean {
  if (value === undefined || value === null) return false;
  if (kind === "text") return String(value).trim() !== "";
  if (kind === "select") return (value as SelectFilterValue).length > 0;
  if (kind === "range") {
    const r = value as RangeFilterValue;
    return r.min != null || r.max != null;
  }
  return false;
}

function parseNum(v: string): number | null {
  if (v.trim() === "") return null;
  const n = Number(v.replace(",", "."));
  return Number.isFinite(n) ? n : null;
}

/** Per-column filter popover: text contains / number range / multi select. */
export function ColumnFilter({ label, kind, value, options, onChange }: Props) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  if (kind === "none") return null;
  const active = isActive(kind, value);
  const range = (value as RangeFilterValue | undefined) ?? {};
  const selected = (value as SelectFilterValue | undefined) ?? [];

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={t("chart.grid.filter_column", { column: label })}
          data-active={active || undefined}
          className={cn(
            "text-muted-foreground shrink-0",
            active && "text-foreground bg-accent",
          )}
        >
          <Filter
            className="size-3.5"
            fill={active ? "currentColor" : "none"}
            aria-hidden
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64 p-3">
        <div className="flex flex-col gap-2">
          <p className="text-xs font-medium">{label}</p>
          {kind === "text" ? (
            <Input
              autoFocus
              value={(value as string | undefined) ?? ""}
              placeholder={t("chart.grid.contains")}
              onChange={(e) =>
                onChange(e.target.value === "" ? undefined : e.target.value)
              }
              className="h-8"
            />
          ) : null}
          {kind === "range" ? (
            <div className="flex items-center gap-2">
              <Input
                inputMode="decimal"
                aria-label={t("chart.grid.min")}
                placeholder={t("chart.grid.min")}
                defaultValue={range.min ?? ""}
                onChange={(e) => {
                  const next = { ...range, min: parseNum(e.target.value) };
                  onChange(
                    next.min == null && next.max == null ? undefined : next,
                  );
                }}
                className="h-8"
              />
              <span className="text-muted-foreground">–</span>
              <Input
                inputMode="decimal"
                aria-label={t("chart.grid.max")}
                placeholder={t("chart.grid.max")}
                defaultValue={range.max ?? ""}
                onChange={(e) => {
                  const next = { ...range, max: parseNum(e.target.value) };
                  onChange(
                    next.min == null && next.max == null ? undefined : next,
                  );
                }}
                className="h-8"
              />
            </div>
          ) : null}
          {kind === "select" ? (
            <ul className="flex max-h-56 flex-col gap-1 overflow-auto">
              {options.map((o) => {
                const checked = selected.includes(o.value);
                const id = `f-${label}-${o.value}`;
                return (
                  <li key={o.value} className="flex items-center gap-2">
                    <Checkbox
                      id={id}
                      checked={checked}
                      onCheckedChange={(c) => {
                        const next = c
                          ? [...selected, o.value]
                          : selected.filter((v) => v !== o.value);
                        onChange(next.length ? next : undefined);
                      }}
                    />
                    <label htmlFor={id} className="truncate text-sm">
                      {o.label}
                    </label>
                  </li>
                );
              })}
            </ul>
          ) : null}
          <div className="flex justify-end">
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={!active}
              onClick={() => {
                onChange(undefined);
                setOpen(false);
              }}
            >
              {t("chart.grid.filter_clear")}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
