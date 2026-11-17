"use client";

import { useState } from "react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  plotterCreditsValid,
  plotterCreditsValue,
} from "@/features/orders/lib/form";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-1221: the usage credits field of a plotter cut line — the credits of
 * the barcodes shipped on it, empty = the product default.
 */
export type PlotterCreditsProps = {
  value: string;
  onChange: (value: string) => void;
  invalid?: boolean;
  /** Blur or Enter: the order detail saves the typed credits. */
  onCommit?: () => void;
};

function PlotterCreditsField({
  id,
  disabled,
  credits,
}: {
  id: string;
  disabled?: boolean;
  credits: PlotterCreditsProps;
}) {
  const { t } = useLocale();
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={`${id}-credits`} className="text-xs font-normal">
        {t("orders.plotter.credits_label")}
      </Label>
      <Input
        id={`${id}-credits`}
        name="plotter_credits"
        inputMode="numeric"
        dir="ltr"
        autoComplete="off"
        maxLength={4}
        className="h-8 w-24"
        data-testid="line-plotter-credits"
        title={t("orders.plotter.credits_hint")}
        placeholder={t("orders.plotter.credits_placeholder")}
        value={credits.value}
        disabled={disabled}
        aria-invalid={credits.invalid || undefined}
        onChange={(e) => credits.onChange(e.target.value)}
        onBlur={() => credits.onCommit?.()}
        onKeyDown={(e) => {
          if (e.key === "Enter" && credits.onCommit) {
            e.preventDefault();
            credits.onCommit();
          }
        }}
      />
    </div>
  );
}

/**
 * TEC-1058 (RV-5.6): the "sent plotter cut" switch of an order line. It is
 * information only (nothing else changes); the monthly plotter report
 * counts the shipped lines carrying it. Off by default. TEC-1221: with
 * `credits` a switched-on line also shows its usage credits field.
 */
export function PlotterToggle({
  id,
  checked,
  onChange,
  disabled,
  credits,
}: {
  id: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  credits?: PlotterCreditsProps;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-1 pt-1">
      <div className="flex items-center gap-2">
        <Switch
          id={id}
          checked={checked}
          disabled={disabled}
          data-testid="line-plotter"
          onCheckedChange={(v) => onChange(v === true)}
        />
        <Label htmlFor={id} className="text-xs font-normal">
          {t("orders.plotter.label")}
        </Label>
      </div>
      {checked && credits ? (
        <PlotterCreditsField id={id} disabled={disabled} credits={credits} />
      ) : null}
    </div>
  );
}

/**
 * TEC-1221: the plotter mark and credits of a saved line (order detail).
 * The switch saves at once; the credits save on blur or Enter when the
 * typed value is valid and changed (empty = the product default).
 */
export function PlotterLineControl({
  id,
  checked,
  credits,
  disabled,
  onSave,
}: {
  id: string;
  checked: boolean;
  credits: number | null | undefined;
  disabled?: boolean;
  onSave: (v: { on: boolean; credits?: number }) => void;
}) {
  const saved = credits != null ? String(credits) : "";
  const [draft, setDraft] = useState(saved);
  const invalid = !plotterCreditsValid(draft);
  return (
    <PlotterToggle
      id={id}
      checked={checked}
      disabled={disabled}
      onChange={(on) => onSave({ on })}
      credits={{
        value: draft,
        invalid,
        onChange: setDraft,
        onCommit: () => {
          if (invalid || draft.trim() === saved) return;
          onSave({ on: true, credits: plotterCreditsValue(draft) });
        },
      }}
    />
  );
}
