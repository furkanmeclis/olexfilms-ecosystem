"use client";

import type { Column } from "@tanstack/react-table";
import { Check, CirclePlus } from "lucide-react";
import type { ComponentType } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export type FacetedFilterOption = {
  label: string;
  value: string;
  icon?: ComponentType<{ className?: string }>;
};

type DataTableFacetedFilterProps<TData, TValue> = {
  column: Column<TData, TValue>;
  title: string;
  options: FacetedFilterOption[];
};

/**
 * Toolbar quick filter — shadcn-admin / users table pattern.
 * @see https://shadcn-admin.netlify.app/users
 */
export function DataTableFacetedFilter<TData, TValue>({
  column,
  title,
  options,
}: DataTableFacetedFilterProps<TData, TValue>) {
  const { t } = useLocale();
  const facets = column.getFacetedUniqueValues();
  const selectedValues = new Set(
    (column.getFilterValue() as string[] | undefined) ?? [],
  );

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-9 border-dashed"
        >
          <CirclePlus className="size-3.5" />
          {title}
          {selectedValues.size > 0 ? (
            <>
              <Separator orientation="vertical" className="mx-1 h-4" />
              <Badge
                variant="secondary"
                className="rounded-sm px-1 font-normal lg:hidden"
              >
                {selectedValues.size}
              </Badge>
              <div className="hidden gap-1 lg:flex">
                {selectedValues.size > 2 ? (
                  <Badge
                    variant="secondary"
                    className="rounded-sm px-1 font-normal"
                  >
                    {t("table.faceted_selected", {
                      count: selectedValues.size,
                    })}
                  </Badge>
                ) : (
                  options
                    .filter((option) => selectedValues.has(option.value))
                    .map((option) => (
                      <Badge
                        key={option.value}
                        variant="secondary"
                        className="rounded-sm px-1 font-normal"
                      >
                        {option.label}
                      </Badge>
                    ))
                )}
              </div>
            </>
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-56 p-0" align="start">
        <Command>
          <CommandInput placeholder={title} />
          <CommandList>
            <CommandEmpty>{t("table.faceted_empty")}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => {
                const isSelected = selectedValues.has(option.value);
                const count =
                  facets.get(option.value) ??
                  (option.value === "true"
                    ? facets.get(true)
                    : option.value === "false"
                      ? facets.get(false)
                      : undefined);
                return (
                  <CommandItem
                    key={option.value}
                    onSelect={() => {
                      const next = new Set(selectedValues);
                      if (isSelected) next.delete(option.value);
                      else next.add(option.value);
                      const values = Array.from(next);
                      column.setFilterValue(values.length ? values : undefined);
                    }}
                  >
                    <div
                      className={cn(
                        "border-primary flex size-4 items-center justify-center rounded-sm border",
                        isSelected
                          ? "bg-primary text-primary-foreground"
                          : "opacity-50 [&_svg]:invisible",
                      )}
                    >
                      <Check className="size-3" />
                    </div>
                    {option.icon ? (
                      <option.icon className="text-muted-foreground size-4" />
                    ) : null}
                    <span className="flex-1">{option.label}</span>
                    {count != null ? (
                      <span className="text-muted-foreground ms-auto font-mono text-xs tabular-nums">
                        {count}
                      </span>
                    ) : null}
                  </CommandItem>
                );
              })}
            </CommandGroup>
            {selectedValues.size > 0 ? (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem
                    onSelect={() => column.setFilterValue(undefined)}
                    className="justify-center text-center"
                  >
                    {t("table.clear_filter")}
                  </CommandItem>
                </CommandGroup>
              </>
            ) : null}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
