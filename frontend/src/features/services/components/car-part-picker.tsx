"use client";

import type { KeyboardEvent } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  PART_GROUPS,
  PART_PRESETS,
  applyPreset,
  isKnownPart,
  isPresetActive,
  partGroup,
  presetParts,
  togglePart,
  type PartGroup,
} from "@/features/services/lib/car-parts";
import { CAR_SHAPES, CAR_VIEWBOX } from "@/features/services/lib/car-shapes";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type CarPartPickerProps = {
  /** Parts the dealer may pick (category available_parts). */
  available: readonly string[];
  selected: readonly string[];
  onChange: (parts: string[]) => void;
  disabled?: boolean;
};

function shapeClass(part: string, selected: boolean, enabled: boolean) {
  const glass = partGroup(part) === "window";
  return cn(
    "outline-none transition-colors [stroke:#737373] [stroke-width:1.5]",
    "focus-visible:[stroke:var(--color-ring)] focus-visible:[stroke-width:5]",
    selected
      ? glass
        ? "fill-sky-500"
        : "fill-emerald-500"
      : glass
        ? "fill-[#bababa]"
        : "fill-[#efefef]",
    enabled
      ? cn(
          "cursor-pointer",
          !selected &&
            (glass ? "hover:fill-sky-200" : "hover:fill-emerald-200"),
        )
      : "cursor-not-allowed opacity-35",
  );
}

/**
 * SVG part picker (TEC-182; legacy CarPartPicker): click or press Space /
 * Enter on a part to toggle it; every part is a focusable ARIA checkbox,
 * mirrored by a checkbox list (also the only list for category parts the
 * drawing has no shape for). The drawing keeps its geometry in RTL (left /
 * right are the vehicle's sides); the list and buttons use logical layout.
 */
export function CarPartPicker({
  available,
  selected,
  onChange,
  disabled = false,
}: CarPartPickerProps) {
  const { t } = useLocale();
  const label = (part: string) =>
    isKnownPart(part) ? t(`services.parts.names.${part}`) : part;
  const canPick = (part: string) => !disabled && available.includes(part);
  const toggle = (part: string) => {
    if (canPick(part)) onChange(togglePart(selected, part));
  };
  const onKey = (e: KeyboardEvent, part: string) => {
    if (e.key === " " || e.key === "Enter") {
      e.preventDefault();
      toggle(part);
    }
  };
  const other = available.filter((p) => !isKnownPart(p));
  const groups: { id: PartGroup | "other"; parts: readonly string[] }[] = [
    { id: "body", parts: PART_GROUPS.body },
    { id: "window", parts: PART_GROUPS.window },
    ...(other.length > 0 ? [{ id: "other" as const, parts: other }] : []),
  ];

  return (
    <div className="space-y-4" data-testid="car-part-picker">
      <div
        className="flex flex-wrap gap-2"
        role="group"
        aria-label={t("services.parts.presets")}
      >
        {PART_PRESETS.map((preset) => {
          const usable = presetParts(preset, available).length > 0;
          const active = isPresetActive(selected, preset, available);
          return (
            <Button
              key={preset.id}
              type="button"
              size="sm"
              variant={active ? "default" : "outline"}
              aria-pressed={active}
              disabled={disabled || !usable}
              data-preset={preset.id}
              onClick={() => onChange(applyPreset(selected, preset, available))}
            >
              {t(`services.parts.preset.${preset.id}`)}
            </Button>
          );
        })}
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={disabled || selected.length === 0}
          data-testid="parts-clear"
          onClick={() => onChange([])}
        >
          {t("services.parts.clear")}
        </Button>
      </div>

      <div className="grid gap-6 lg:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <div dir="ltr" className="bg-muted/30 rounded-lg border p-2">
          <svg
            viewBox={CAR_VIEWBOX}
            className="h-auto w-full"
            role="group"
            aria-label={t("services.parts.drawing_label")}
          >
            {CAR_SHAPES.map((shape) => {
              const part = shape.part;
              if (!part) {
                return (
                  <path
                    key={shape.id}
                    d={shape.d}
                    fillRule={shape.evenOdd ? "evenodd" : undefined}
                    aria-hidden="true"
                    className={cn(
                      "pointer-events-none [stroke:#737373] [stroke-width:1.5]",
                      shape.tone === "glass" ? "fill-[#bababa]" : "fill-none",
                    )}
                  />
                );
              }
              const on = selected.includes(part);
              const enabled = canPick(part);
              return (
                <path
                  key={shape.id}
                  d={shape.d}
                  fillRule={shape.evenOdd ? "evenodd" : undefined}
                  role="checkbox"
                  aria-checked={on}
                  aria-disabled={!enabled || undefined}
                  aria-label={label(part)}
                  tabIndex={enabled ? 0 : -1}
                  data-part={part}
                  className={shapeClass(part, on, enabled)}
                  onClick={() => toggle(part)}
                  onKeyDown={(e) => onKey(e, part)}
                >
                  <title>{label(part)}</title>
                </path>
              );
            })}
          </svg>
        </div>

        <div className="space-y-4">
          {groups.map((group) => (
            <fieldset key={group.id} className="space-y-2">
              <legend className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
                {t(`services.parts.groups.${group.id}`)}
              </legend>
              <ul className="grid gap-1.5 sm:grid-cols-2">
                {group.parts.map((part) => {
                  const id = `part-${part}`;
                  const enabled = canPick(part);
                  return (
                    <li key={part} className="flex items-center gap-2">
                      <Checkbox
                        id={id}
                        checked={selected.includes(part)}
                        disabled={!enabled}
                        data-part-check={part}
                        onCheckedChange={() => toggle(part)}
                      />
                      <label
                        htmlFor={id}
                        className={cn(
                          "text-sm",
                          !enabled && "text-muted-foreground",
                        )}
                      >
                        {label(part)}
                      </label>
                    </li>
                  );
                })}
              </ul>
            </fieldset>
          ))}
        </div>
      </div>
    </div>
  );
}
