"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FieldGroup } from "@/components/ui/field";
import { useVehicleBrandMutations } from "@/features/vehicle-catalog/hooks/use-vehicle-catalog";
import type {
  VehicleBrand,
  VehicleBrandInput,
} from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import { useLocale } from "@/providers/locale-provider";

type BrandFormValues = {
  name: string;
  external_id: string;
  logo_height: string;
  show_name: boolean;
  active: boolean;
};

export function brandFormToInput(values: BrandFormValues): VehicleBrandInput {
  const height = values.logo_height.trim();
  return {
    name: values.name.trim(),
    external_id: values.external_id.trim() || null,
    logo_height: height ? Number(height) : null,
    show_name: values.show_name,
    active: values.active,
  };
}

/** Create / edit form of a car brand (logo and hero live on the detail page). */
export function VehicleBrandDialog({
  open,
  onOpenChange,
  brand,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Edit this brand; create a new one when omitted. */
  brand?: VehicleBrand | null;
  onSaved?: (brand: VehicleBrand) => void;
}) {
  const { t } = useLocale();
  const { create, update } = useVehicleBrandMutations();
  const pending = create.isPending || update.isPending;

  const schema = useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("form.required")).max(150),
        external_id: z.string().trim().max(64),
        logo_height: z
          .string()
          .trim()
          .refine(
            (v) =>
              v === "" ||
              (/^\d+$/.test(v) && Number(v) >= 8 && Number(v) <= 512),
            t("vehicles.validation.logo_height"),
          ),
        show_name: z.boolean(),
        active: z.boolean(),
      }),
    [t],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t(brand ? "vehicles.brand.edit_title" : "vehicles.brand.create")}
          </DialogTitle>
          <DialogDescription>{t("vehicles.brand.form_hint")}</DialogDescription>
        </DialogHeader>
        <AppForm<BrandFormValues>
          key={open ? (brand?.uuid ?? "new") : "closed"}
          schema={schema}
          defaultValues={{
            name: brand?.name ?? "",
            external_id: brand?.external_id ?? "",
            logo_height: brand?.logo_height ? String(brand.logo_height) : "",
            show_name: brand?.show_name ?? false,
            active: brand?.active ?? true,
          }}
          onSubmit={async (values) => {
            const body = brandFormToInput(values);
            const saved = brand
              ? await update.mutateAsync({ uuid: brand.uuid, body })
              : await create.mutateAsync(body);
            onOpenChange(false);
            onSaved?.(saved);
          }}
        >
          <FieldGroup className="gap-4">
            <AppInput
              name="name"
              label={t("vehicles.fields.name")}
              required
              autoFocus
            />
            <AppInput
              name="external_id"
              label={t("vehicles.fields.external_id")}
              description={t("vehicles.fields.external_id_hint")}
            />
            <AppInput
              name="logo_height"
              inputMode="numeric"
              label={t("vehicles.fields.logo_height")}
              description={t("vehicles.fields.logo_height_hint")}
            />
            <AppSwitch
              name="show_name"
              label={t("vehicles.fields.show_name")}
            />
            <AppSwitch name="active" label={t("vehicles.fields.active")} />
          </FieldGroup>
          <DialogFooter className="mt-6">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending}>
              {pending ? t("common.saving") : t("common.save")}
            </Button>
          </DialogFooter>
        </AppForm>
      </DialogContent>
    </Dialog>
  );
}
