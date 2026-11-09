"use client";

import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useLocale } from "@/providers/locale-provider";

/**
 * TEC-1058 (RV-5.6): the "sent plotter cut" switch of an order line. It is
 * information only (nothing else changes); the monthly plotter report
 * counts the shipped lines carrying it. Off by default.
 */
export function PlotterToggle({
  id,
  checked,
  onChange,
  disabled,
}: {
  id: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  return (
    <div className="flex items-center gap-2 pt-1">
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
  );
}
