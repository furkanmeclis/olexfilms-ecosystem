"use client";

import { Clock } from "lucide-react";
import * as React from "react";

import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const HOURS = Array.from({ length: 24 }, (_, i) => pad(i));

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

/** Splits `HH:mm` (seconds ignored); `undefined` when not a valid time. */
export function parseTimeValue(
  value: string | undefined,
): { hour: string; minute: string } | undefined {
  const match = /^(\d{2}):(\d{2})/.exec(value ?? "");
  if (!match) return undefined;
  const [, hour, minute] = match;
  if (Number(hour) > 23 || Number(minute) > 59) return undefined;
  return { hour, minute };
}

function TimeColumn({
  label,
  options,
  selected,
  onSelect,
}: {
  label: string;
  options: string[];
  selected: string | undefined;
  onSelect: (value: string) => void;
}) {
  const selectedRef = React.useRef<HTMLButtonElement>(null);

  React.useEffect(() => {
    selectedRef.current?.scrollIntoView?.({ block: "center" });
  }, []);

  return (
    <div
      role="listbox"
      aria-label={label}
      className="flex max-h-56 w-14 flex-col gap-0.5 overflow-y-auto p-1"
    >
      {options.map((option) => {
        const isSelected = option === selected;
        return (
          <Button
            key={option}
            ref={isSelected ? selectedRef : undefined}
            type="button"
            role="option"
            aria-selected={isSelected}
            variant={isSelected ? "default" : "ghost"}
            size="sm"
            className="h-8 shrink-0 font-normal tabular-nums"
            onClick={() => onSelect(option)}
          >
            {option}
          </Button>
        );
      })}
    </div>
  );
}

export type TimeColumnsProps = {
  value?: string;
  onChange: (value: string) => void;
  /** Minute granularity of the minute column. Defaults to 1. */
  minuteStep?: number;
  className?: string;
};

/**
 * Hour + minute columns. Picking one part while the other is empty fills it
 * with `00`, so the emitted value is always a complete `HH:mm`.
 */
export function TimeColumns({
  value,
  onChange,
  minuteStep = 1,
  className,
}: TimeColumnsProps) {
  const { t } = useLocale();
  const parsed = parseTimeValue(value);
  const minutes = React.useMemo(() => {
    const step = Math.max(1, Math.floor(minuteStep));
    const list = Array.from({ length: Math.ceil(60 / step) }, (_, i) =>
      pad(i * step),
    );
    // Keep an off-step stored value selectable/visible.
    if (parsed && !list.includes(parsed.minute)) {
      list.push(parsed.minute);
      list.sort();
    }
    return list;
  }, [minuteStep, parsed]);

  return (
    <div dir="ltr" className={cn("flex divide-x", className)}>
      <TimeColumn
        label={t("form.hour")}
        options={HOURS}
        selected={parsed?.hour}
        onSelect={(hour) => onChange(`${hour}:${parsed?.minute ?? "00"}`)}
      />
      <TimeColumn
        label={t("form.minute")}
        options={minutes}
        selected={parsed?.minute}
        onSelect={(minute) => onChange(`${parsed?.hour ?? "00"}:${minute}`)}
      />
    </div>
  );
}

export type TimePickerProps = {
  id?: string;
  name?: string;
  value?: string;
  onChange?: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
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
 * Shared time picker (Popover + hour/minute columns).
 * Value is always `HH:mm` (empty string when cleared), like `<input type="time">`.
 */
export function TimePicker({
  id,
  name,
  value,
  onChange,
  placeholder,
  disabled,
  className,
  minuteStep,
  clearable = true,
  align = "start",
  "aria-invalid": ariaInvalid,
  "aria-label": ariaLabel,
  "aria-describedby": ariaDescribedBy,
  "data-testid": testId,
}: TimePickerProps) {
  const { t } = useLocale();
  const parsed = parseTimeValue(value);

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
          data-empty={!parsed}
          className={cn(
            "data-[empty=true]:text-muted-foreground h-9 w-full justify-start gap-2 px-3 font-normal tabular-nums",
            ariaInvalid && "border-destructive",
            className,
          )}
        >
          <Clock className="text-muted-foreground size-4" />
          {parsed
            ? `${parsed.hour}:${parsed.minute}`
            : (placeholder ?? t("form.pick_time"))}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align={align}>
        <TimeColumns
          value={value}
          onChange={(next) => onChange?.(next)}
          minuteStep={minuteStep}
        />
        {clearable && parsed ? (
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
