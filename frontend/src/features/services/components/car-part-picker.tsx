"use client";

import { X } from "lucide-react";
import { type KeyboardEvent } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  PART_GROUPS,
  applyPreset,
  isKnownPart,
  isPresetActive,
  partGroup,
  togglePart,
  visiblePresets,
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
 *
 * TEC-864: one-click presets per group (body for PPF, windows for window
 * film; only the groups the category offers) come first. TEC-1076 (revize
 * 9.1): the presets combine (front two windows + windshield), the vehicle
 * drawing is always shown with the selection highlighted, and the picked
 * parts are listed as removable chips under it.
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
  const other = available.filter((p) => !isKnownPart(p));
  const groups: { id: PartGroup | "other"; parts: readonly string[] }[] = [
    { id: "body", parts: PART_GROUPS.body },
    { id: "window", parts: PART_GROUPS.window },
    ...(other.length > 0 ? [{ id: "other" as const, parts: other }] : []),
  ];

  const presets = visiblePresets(available);
  const presetGroups = (["body", "window"] as const)
    .map((id) => ({ id, presets: presets.filter((p) => p.group === id) }))
    .filter((g) => g.presets.length > 0);

  return (
    <div className="space-y-4" data-testid="car-part-picker">
      <div
        className="space-y-3"
        role="group"
        aria-label={t("services.parts.presets")}
      >
        {presetGroups.map((group) => (
          <div
            key={group.id}
            className="flex flex-wrap items-center gap-2"
            data-preset-group={group.id}
          >
            <span className="text-muted-foreground w-full text-xs font-semibold tracking-wide uppercase sm:w-auto sm:min-w-16">
              {t(`services.parts.groups.${group.id}`)}
            </span>
            {group.presets.map((preset) => {
              const active = isPresetActive(selected, preset, available);
              return (
                <Button
                  key={preset.id}
                  type="button"
                  size="sm"
                  variant={active ? "default" : "outline"}
                  aria-pressed={active}
                  disabled={disabled}
                  data-preset={preset.id}
                  onClick={() =>
                    onChange(applyPreset(selected, preset, available))
                  }
                >
                  {t(`services.parts.preset.${preset.id}`)}
                </Button>
              );
            })}
          </div>
        ))}
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
        <CarPartDrawing
          selected={selected}
          canPick={canPick}
          onToggle={toggle}
          label={t("services.parts.drawing_label")}
          partLabel={label}
        />

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

      <SelectedParts
        selected={selected}
        label={label}
        disabled={disabled}
        onRemove={(part) => onChange(selected.filter((p) => p !== part))}
      />
    </div>
  );
}

/**
 * The vehicle drawing (TEC-182 shapes): selected parts are filled, the
 * pickable ones are focusable ARIA checkboxes toggled by click, Space or
 * Enter. Shared by the parts step and the product form of the stock step
 * (TEC-1076), which limits `canPick` to the product's parts.
 */
export function CarPartDrawing({
  selected,
  canPick,
  onToggle,
  label,
  partLabel,
  className,
}: {
  selected: readonly string[];
  canPick: (part: string) => boolean;
  onToggle: (part: string) => void;
  label: string;
  partLabel: (part: string) => string;
  className?: string;
}) {
  const onKey = (e: KeyboardEvent, part: string) => {
    if (e.key === " " || e.key === "Enter") {
      e.preventDefault();
      onToggle(part);
    }
  };
  return (
    <div
      dir="ltr"
      className={cn("bg-muted/30 rounded-lg border p-2", className)}
      data-testid="car-part-drawing"
    >
      <svg
        viewBox={CAR_VIEWBOX}
        className="h-auto w-full"
        role="group"
        aria-label={label}
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
              aria-label={partLabel(part)}
              tabIndex={enabled ? 0 : -1}
              data-part={part}
              className={shapeClass(part, on, enabled)}
              onClick={() => {
                if (enabled) onToggle(part);
              }}
              onKeyDown={(e) => {
                if (enabled) onKey(e, part);
              }}
            >
              <title>{partLabel(part)}</title>
            </path>
          );
        })}
      </svg>
    </div>
  );
}

/** The picked parts as removable chips (TEC-1076). */
function SelectedParts({
  selected,
  label,
  disabled,
  onRemove,
}: {
  selected: readonly string[];
  label: (part: string) => string;
  disabled: boolean;
  onRemove: (part: string) => void;
}) {
  const { t } = useLocale();
  return (
    <section className="space-y-2" data-testid="parts-selected">
      <h3 className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
        {t("services.parts.selected_title")}
      </h3>
      {selected.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("services.parts.selected_none")}
        </p>
      ) : (
        <ul className="flex flex-wrap gap-1.5">
          {selected.map((part) => (
            <li key={part}>
              <Badge
                variant="secondary"
                className="gap-1 py-1 ps-2 pe-1"
                data-selected-part={part}
              >
                {label(part)}
                {!disabled ? (
                  <button
                    type="button"
                    className="hover:bg-muted-foreground/20 rounded-sm p-0.5"
                    aria-label={t("services.parts.remove", {
                      name: label(part),
                    })}
                    onClick={() => onRemove(part)}
                  >
                    <X className="size-3" aria-hidden="true" />
                  </button>
                ) : null}
              </Badge>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
