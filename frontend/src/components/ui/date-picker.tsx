"use client";

import { format, isValid, parseISO } from "date-fns";
import { CalendarIcon } from "lucide-react";
import * as React from "react";

import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { dateFnsLocale, dayPickerLocale } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type DatePickerProps = {
  id?: string;
  value?: string;
  onChange?: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  /** Display pattern for the trigger label. Defaults to locale-aware `PPP`. */
  displayFormat?: string;
  captionLayout?: React.ComponentProps<typeof Calendar>["captionLayout"];
  align?: React.ComponentProps<typeof PopoverContent>["align"];
  "aria-invalid"?: boolean;
  "aria-label"?: string;
  "aria-describedby"?: string;
  "data-testid"?: string;
  /** Earliest selectable day (`yyyy-MM-dd`); earlier days are disabled. */
  minDate?: string;
};

function parseDateValue(value: string | undefined): Date | undefined {
  if (!value) return undefined;
  const parsed = parseISO(value);
  return isValid(parsed) ? parsed : undefined;
}

/**
 * Shared date picker (Popover + Calendar).
 * Value is always `yyyy-MM-dd` (empty string when cleared).
 * Calendar labels follow the active app locale (`tr` / `en`).
 */
export function DatePicker({
  id,
  value,
  onChange,
  placeholder,
  disabled,
  className,
  displayFormat,
  captionLayout,
  align = "start",
  "aria-invalid": ariaInvalid,
  "aria-label": ariaLabel,
  "aria-describedby": ariaDescribedBy,
  "data-testid": testId,
  minDate,
}: DatePickerProps) {
  const { locale, t } = useLocale();
  const selected = parseDateValue(value);
  const min = parseDateValue(minDate);
  const dfLocale = dateFnsLocale(locale);

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          id={id}
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
            className,
          )}
        >
          <CalendarIcon className="text-muted-foreground size-4" />
          {selected
            ? format(selected, displayFormat ?? "PPP", { locale: dfLocale })
            : (placeholder ?? t("form.pick_date"))}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align={align}>
        <Calendar
          mode="single"
          locale={dayPickerLocale(locale)}
          selected={selected}
          onSelect={(date) => {
            onChange?.(date ? format(date, "yyyy-MM-dd") : "");
          }}
          defaultMonth={selected ?? min}
          captionLayout={captionLayout}
          disabled={min ? { before: min } : undefined}
        />
      </PopoverContent>
    </Popover>
  );
}
