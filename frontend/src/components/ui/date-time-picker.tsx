"use client";

import { format, isValid, parse } from "date-fns";
import { CalendarClock } from "lucide-react";
import * as React from "react";

import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { TimeColumns } from "@/components/ui/time-picker";
import { dateFnsLocale, dayPickerLocale } from "@/lib/utils/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const VALUE_FORMAT = "yyyy-MM-dd'T'HH:mm";

/** Parses a `yyyy-MM-ddTHH:mm` wall-clock value (seconds ignored). */
function parseDateTimeValue(value: string | undefined): Date | undefined {
  if (!value) return undefined;
  const parsed = parse(value.slice(0, 16), VALUE_FORMAT, new Date());
  return isValid(parsed) ? parsed : undefined;
}

export type DateTimePickerProps = {
  id?: string;
  name?: string;
  value?: string;
  onChange?: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  /** Display pattern for the trigger label. Defaults to locale-aware `PPP p`. */
  displayFormat?: string;
  minuteStep?: number;
  /** Shows a clear action inside the popover. Defaults to true. */
  clearable?: boolean;
  align?: React.ComponentProps<typeof PopoverContent>["align"];
  "aria-invalid"?: boolean;
  "aria-label"?: string;
  "aria-describedby"?: string;
  "data-testid"?: string;
};

/**
 * Shared date + time picker (Popover + Calendar + hour/minute columns).
 * Value is a zone-less wall-clock `yyyy-MM-ddTHH:mm` (empty string when
 * cleared), exactly like `<input type="datetime-local">`; callers keep doing
 * their own timezone conversion. Picking a day keeps the chosen time (or
 * `00:00`); picking a time first uses today's date.
 */
export function DateTimePicker({
  id,
  name,
  value,
  onChange,
  placeholder,
  disabled,
  className,
  displayFormat,
  minuteStep,
  clearable = true,
  align = "start",
  "aria-invalid": ariaInvalid,
  "aria-label": ariaLabel,
  "aria-describedby": ariaDescribedBy,
  "data-testid": testId,
}: DateTimePickerProps) {
  const { locale, t } = useLocale();
  const selected = parseDateTimeValue(value);
  const dfLocale = dateFnsLocale(locale);
  const time = selected ? format(selected, "HH:mm") : "";

  return (
    <Popover>
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
          <CalendarClock className="text-muted-foreground size-4" />
          {selected
            ? format(selected, displayFormat ?? "PPP p", { locale: dfLocale })
            : (placeholder ?? t("form.pick_date_time"))}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align={align}>
        <div className="flex flex-col sm:flex-row">
          <Calendar
            mode="single"
            locale={dayPickerLocale(locale)}
            selected={selected}
            onSelect={(date) => {
              if (!date) {
                onChange?.("");
                return;
              }
              onChange?.(`${format(date, "yyyy-MM-dd")}T${time || "00:00"}`);
            }}
            defaultMonth={selected}
          />
          <TimeColumns
            className="border-t sm:border-s sm:border-t-0"
            value={time}
            minuteStep={minuteStep}
            onChange={(next) =>
              onChange?.(
                `${format(selected ?? new Date(), "yyyy-MM-dd")}T${next}`,
              )
            }
          />
        </div>
        {clearable && selected ? (
          <div className="border-t p-1">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="w-full"
              onClick={() => onChange?.("")}
            >
              {t("form.clear")}
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}
