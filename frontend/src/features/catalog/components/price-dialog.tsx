"use client";

import { z } from "zod";

import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  CURRENCY_RE,
  normalizePriceInput,
  PRICE_AMOUNT_RE,
} from "@/features/catalog/lib/prices";
import { useLocale } from "@/providers/locale-provider";

export type PriceDialogField = {
  name: string;
  labelKey: string;
  /** Required fields reject an empty value; optional empty means "clear". */
  required?: boolean;
};

export type PriceDialogValues = Record<string, string>;

type PriceDialogProps = {
  open: boolean;
  title: string;
  description?: string;
  fields: PriceDialogField[];
  defaults: PriceDialogValues;
  /** Locks the currency when editing an existing row. */
  currencyLocked?: boolean;
  /** Optional picker shown first (distributor of a distributor price). */
  picker?: {
    name: string;
    labelKey: string;
    options: { value: string; label: string }[];
    locked?: boolean;
  };
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: PriceDialogValues) => Promise<unknown>;
};

/** Price form of one currency; the save triggers step-up when needed. */
export function PriceDialog({
  open,
  title,
  description,
  fields,
  defaults,
  currencyLocked,
  picker,
  pending,
  onOpenChange,
  onSubmit,
}: PriceDialogProps) {
  const { t } = useLocale();

  const shape: Record<string, z.ZodType<string>> = {
    currency: z
      .string()
      .trim()
      .transform((v) => v.toUpperCase())
      .refine((v) => CURRENCY_RE.test(v), t("catalog.validation.currency")),
  };
  if (picker) {
    shape[picker.name] = z.string().min(1, t("catalog.validation.required"));
  }
  for (const field of fields) {
    shape[field.name] = z
      .string()
      .transform(normalizePriceInput)
      .refine(
        (v) => (v === "" ? !field.required : PRICE_AMOUNT_RE.test(v)),
        t("catalog.validation.price"),
      );
  }
  const schema = z.object(shape) as unknown as z.ZodType<PriceDialogValues>;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {description ?? t("catalog.prices.step_up_hint")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <AppForm<PriceDialogValues>
            schema={schema}
            defaultValues={defaults}
            onSubmit={async (values) => {
              await onSubmit(values);
            }}
            className="space-y-4"
          >
            {picker ? (
              <AppSelect
                name={picker.name}
                label={t(picker.labelKey)}
                options={picker.options}
                disabled={picker.locked}
              />
            ) : null}
            <AppInput
              name="currency"
              label={t("catalog.prices.currency")}
              maxLength={3}
              placeholder="TRY"
              readOnly={currencyLocked}
            />
            {fields.map((field) => (
              <AppInput
                key={field.name}
                name={field.name}
                inputMode="decimal"
                label={t(field.labelKey)}
                placeholder="0.00"
              />
            ))}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={pending}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </AppForm>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
