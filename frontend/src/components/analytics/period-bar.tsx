"use client";

import { useState, type ReactNode } from "react";
import { CalendarRange } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useFormatter, useLocale } from "@/providers/locale-provider";

import {
  COMPARE_MODES,
  GRANULARITIES,
  PERIOD_PRESETS,
  type CompareMode,
  type PeriodGranularity,
  type PeriodPreset,
  type PeriodState,
} from "./period";

export type PeriodBarProps = {
  value: PeriodState;
  onChange: (next: PeriodState) => void;
  presets?: readonly PeriodPreset[];
  /** Show the custom range picker (default true). */
  allowCustom?: boolean;
  /** Show the granularity select (default true). */
  showGranularity?: boolean;
  /** Show the compare select (default true). */
  showCompare?: boolean;
  /** Extra filters rendered on the same row (filters sit above charts). */
  children?: ReactNode;
  className?: string;
};

const AUTO = "__auto";

/**
 * Period filter row: presets (today, 7/30/90 days, 12 months), custom
 * range, granularity and comparison. Controlled; pair with usePeriodState()
 * to keep it in the URL.
 */
export function PeriodBar({
  value,
  onChange,
  presets = PERIOD_PRESETS,
  allowCustom = true,
  showGranularity = true,
  showCompare = true,
  children,
  className,
}: PeriodBarProps) {
  const { t } = useLocale();
  const fmt = useFormatter();
  const [open, setOpen] = useState(false);
  const [from, setFrom] = useState(value.from ?? "");
  const [to, setTo] = useState(value.to ?? "");

  const customLabel =
    value.preset === "custom" && value.from && value.to
      ? `${fmt.date(value.from)} – ${fmt.date(value.to)}`
      : t("chart.period.custom");

  return (
    <div
      role="toolbar"
      aria-label={t("chart.period.label")}
      data-slot="period-bar"
      className={cn(
        "flex flex-nowrap items-center gap-2 overflow-x-auto pb-1",
        className,
      )}
    >
      <div
        role="radiogroup"
        aria-label={t("chart.period.label")}
        className="bg-muted/60 flex shrink-0 items-center gap-0.5 rounded-lg p-0.5"
      >
        {presets.map((p) => {
          const active = value.preset === p;
          return (
            <button
              key={p}
              type="button"
              role="radio"
              aria-checked={active}
              data-preset={p}
              onClick={() =>
                onChange({
                  ...value,
                  preset: p,
                  from: undefined,
                  to: undefined,
                })
              }
              className={cn(
                "h-7 rounded-md px-2.5 text-xs font-medium whitespace-nowrap transition-colors",
                active
                  ? "bg-background text-foreground shadow-xs"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {t(`chart.period.${p}`)}
            </button>
          );
        })}
      </div>

      {allowCustom ? (
        <Popover
          open={open}
          onOpenChange={(next) => {
            setOpen(next);
            if (next) {
              setFrom(value.from ?? "");
              setTo(value.to ?? "");
            }
          }}
        >
          <PopoverTrigger asChild>
            <Button
              type="button"
              size="sm"
              variant={value.preset === "custom" ? "secondary" : "outline"}
              className="h-8 shrink-0"
              data-preset="custom"
            >
              <CalendarRange aria-hidden />
              {customLabel}
            </Button>
          </PopoverTrigger>
          <PopoverContent align="start" className="w-72">
            <form
              className="flex flex-col gap-3"
              onSubmit={(e) => {
                e.preventDefault();
                if (!from || !to) return;
                const [a, b] = from <= to ? [from, to] : [to, from];
                onChange({ ...value, preset: "custom", from: a, to: b });
                setOpen(false);
              }}
            >
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="period-from">{t("chart.period.from")}</Label>
                <DatePicker id="period-from" value={from} onChange={setFrom} />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="period-to">{t("chart.period.to")}</Label>
                <DatePicker id="period-to" value={to} onChange={setTo} />
              </div>
              <div className="flex justify-end gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setOpen(false)}
                >
                  {t("chart.period.cancel")}
                </Button>
                <Button type="submit" size="sm" disabled={!from || !to}>
                  {t("chart.period.apply")}
                </Button>
              </div>
            </form>
          </PopoverContent>
        </Popover>
      ) : null}

      {showGranularity ? (
        <Select
          value={value.granularity ?? AUTO}
          onValueChange={(g) =>
            onChange({
              ...value,
              granularity: g === AUTO ? undefined : (g as PeriodGranularity),
            })
          }
        >
          <SelectTrigger
            size="sm"
            className="h-8 w-auto shrink-0"
            aria-label={t("chart.period.granularity")}
            data-testid="period-granularity"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={AUTO}>
              {t("chart.period.granularity_auto")}
            </SelectItem>
            {GRANULARITIES.map((g) => (
              <SelectItem key={g} value={g}>
                {t(`chart.period.granularity_${g}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : null}

      {showCompare ? (
        <Select
          value={value.compare}
          onValueChange={(c) =>
            onChange({ ...value, compare: c as CompareMode })
          }
        >
          <SelectTrigger
            size="sm"
            className="h-8 w-auto shrink-0"
            aria-label={t("chart.period.compare")}
            data-testid="period-compare"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {COMPARE_MODES.map((c) => (
              <SelectItem key={c} value={c}>
                {t(`chart.period.compare_${c}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : null}

      {children}
    </div>
  );
}
