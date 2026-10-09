"use client";

import { format } from "date-fns";
import { CalendarIcon, ChevronLeft, ChevronRight } from "lucide-react";
import * as React from "react";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { dateFnsLocale } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Splits `yyyy-MM`; `undefined` when not a valid month. */
function parseMonthValue(
  value: string | undefined,
): { year: number; month: number } | undefined {
  const match = /^(\d{4})-(\d{2})$/.exec(value ?? "");
  if (!match) return undefined;
  const year = Number(match[1]);
  const month = Number(match[2]);
  if (month < 1 || month > 12) return undefined;
  return { year, month };
}

export type MonthPickerProps = {
  id?: string;
  name?: string;
  value?: string;
  onChange?: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  /** Shows a clear action inside the popover. Defaults to false. */
  clearable?: boolean;
  align?: React.ComponentProps<typeof PopoverContent>["align"];
  "aria-invalid"?: boolean;
  "aria-label"?: string;
  "aria-describedby"?: string;
  "data-testid"?: string;
};

/**
 * Shared month picker (Popover + year switcher + 12-month grid).
 * Value is always `yyyy-MM` (empty string when cleared), like
 * `<input type="month">`.
 */
export function MonthPicker({
  id,
  name,
  value,
  onChange,
  placeholder,
  disabled,
  className,
  clearable = false,
  align = "start",
  "aria-invalid": ariaInvalid,
  "aria-label": ariaLabel,
  "aria-describedby": ariaDescribedBy,
  "data-testid": testId,
}: MonthPickerProps) {
  const { locale, t } = useLocale();
  const dfLocale = dateFnsLocale(locale);
  const selected = parseMonthValue(value);
  const [open, setOpen] = React.useState(false);
  const [year, setYear] = React.useState(
    () => selected?.year ?? new Date().getFullYear(),
  );

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        if (next) setYear(selected?.year ?? new Date().getFullYear());
        setOpen(next);
      }}
    >
      <PopoverTrigger asChild>
        <Button
          id={id}
          name={name}
          value={value ?? ""}
          type="button"
          variant="outline"
          disabled={disabled}
          aria-invalid={ariaInvalid}
          aria-label={ariaLabel}
          aria-describedby={ariaDescribedBy}
          data-testid={testId}
          data-empty={!selected}
          className={cn(
            "data-[empty=true]:text-muted-foreground h-9 w-full justify-start gap-2 px-3 font-normal",
            ariaInvalid && "border-destructive",
            className,
          )}
        >
          <CalendarIcon className="text-muted-foreground size-4" />
          {selected
            ? format(
                new Date(selected.year, selected.month - 1, 1),
                "LLLL yyyy",
                {
                  locale: dfLocale,
                },
              )
            : (placeholder ?? t("form.pick_month"))}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64 p-2" align={align}>
        <div className="mb-2 flex items-center justify-between">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t("form.previous_year")}
            onClick={() => setYear((y) => y - 1)}
          >
            <ChevronLeft className="size-4 rtl:rotate-180" />
          </Button>
          <span className="text-sm font-medium tabular-nums">{year}</span>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t("form.next_year")}
            onClick={() => setYear((y) => y + 1)}
          >
            <ChevronRight className="size-4 rtl:rotate-180" />
          </Button>
        </div>
        <div
          role="listbox"
          aria-label={String(year)}
          className="grid grid-cols-3 gap-1"
        >
          {Array.from({ length: 12 }, (_, i) => {
            const isSelected =
              selected?.year === year && selected.month === i + 1;
            const monthValue = `${year}-${String(i + 1).padStart(2, "0")}`;
            return (
              <Button
                key={i}
                type="button"
                role="option"
                aria-selected={isSelected}
                data-value={monthValue}
                variant={isSelected ? "default" : "ghost"}
                size="sm"
                className="font-normal"
                onClick={() => {
                  onChange?.(monthValue);
                  setOpen(false);
                }}
              >
                {format(new Date(year, i, 1), "LLL", { locale: dfLocale })}
              </Button>
            );
          })}
        </div>
        {clearable && selected ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="mt-2 w-full"
            onClick={() => {
              onChange?.("");
              setOpen(false);
            }}
          >
            {t("form.clear")}
          </Button>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}
