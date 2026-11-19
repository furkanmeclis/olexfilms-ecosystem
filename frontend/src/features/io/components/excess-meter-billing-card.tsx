"use client";

import { Ruler } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { useLocale } from "@/providers/locale-provider";

export type ExcessMeterBilling = "free" | "charge";

export const EXCESS_METER_BILLING_MODES: readonly ExcessMeterBilling[] = [
  "free",
  "charge",
];

export type ExcessMeterBillingCardProps = {
  value: ExcessMeterBilling;
  canWrite: boolean;
  isSaving: boolean;
  onSave: (value: ExcessMeterBilling) => void;
};

/**
 * TEC-1250 (K7, 19.11): when a meter line takes the whole roll or box
 * because the rest would be shorter than the minimum cut, the extra meters
 * are free (default) or added to the invoice. The mode is frozen on the
 * line when the warehouse assigns the roll; changing it here never touches
 * existing orders.
 */
export function ExcessMeterBillingCard({
  value,
  canWrite,
  isSaving,
  onSave,
}: ExcessMeterBillingCardProps) {
  const { t } = useLocale();
  const [mode, setMode] = useState<ExcessMeterBilling>(value);

  return (
    <Card data-testid="excess-meter-billing-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Ruler className="size-4" />
          {t("settings.excess_meter_billing.title")}
        </CardTitle>
        <CardDescription>
          {t("settings.excess_meter_billing.hint")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <RadioGroup
          value={mode}
          disabled={!canWrite}
          onValueChange={(v) => setMode(v as ExcessMeterBilling)}
          className="space-y-2"
        >
          {EXCESS_METER_BILLING_MODES.map((m) => (
            <div key={m} className="flex items-start gap-3">
              <RadioGroupItem
                value={m}
                id={`excess-meter-billing-${m}`}
                className="mt-1"
              />
              <Label
                htmlFor={`excess-meter-billing-${m}`}
                className="flex flex-col items-start gap-1"
              >
                <span>{t(`settings.excess_meter_billing.${m}`)}</span>
                <span className="text-muted-foreground text-sm font-normal">
                  {t(`settings.excess_meter_billing.${m}_hint`)}
                </span>
              </Label>
            </div>
          ))}
        </RadioGroup>
        <p className="text-muted-foreground text-sm">
          {t("settings.excess_meter_billing.frozen")}
        </p>
        {canWrite ? (
          <Button
            type="button"
            disabled={mode === value || isSaving}
            onClick={() => onSave(mode)}
          >
            {t("settings.save")}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  );
}
