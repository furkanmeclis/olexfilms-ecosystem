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
import { VehicleHeroImage } from "@/features/vehicle-catalog/components/vehicle-hero-image";
import { VehicleImageField } from "@/features/vehicle-catalog/components/vehicle-image-field";
import { useVehicleModelMutations } from "@/features/vehicle-catalog/hooks/use-vehicle-catalog";
import type {
  VehicleModel,
  VehicleModelInput,
} from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import { useLocale } from "@/providers/locale-provider";

type ModelFormValues = {
  name: string;
  external_id: string;
  body_type: string;
  powertrain: string;
  year_start: string;
  year_stop: string;
  active: boolean;
};

const YEAR_RE = /^\d{4}$/;

function optionalYear(value: string) {
  const v = value.trim();
  return v ? Number(v) : null;
}

export function modelFormToInput(values: ModelFormValues): VehicleModelInput {
  return {
    name: values.name.trim(),
    external_id: values.external_id.trim() || null,
    body_type: values.body_type.trim() || null,
    powertrain: values.powertrain.trim() || null,
    year_start: optionalYear(values.year_start),
    year_stop: optionalYear(values.year_stop),
    active: values.active,
  };
}

/** Create / edit form of a car model; the hero upload shows when editing. */
export function VehicleModelDialog({
  open,
  onOpenChange,
  brandUuid,
  model,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  brandUuid: string;
  /** Edit this model; create a new one under brandUuid when omitted. */
  model?: VehicleModel | null;
}) {
  const { t } = useLocale();
  const { create, update, uploadHero, deleteHero } = useVehicleModelMutations();
  const pending = create.isPending || update.isPending;

  const schema = useMemo(() => {
    const year = z
      .string()
      .trim()
      .refine(
        (v) =>
          v === "" ||
          (YEAR_RE.test(v) && Number(v) >= 1900 && Number(v) <= 2100),
        t("vehicles.validation.year"),
      );
    return z
      .object({
        name: z.string().trim().min(1, t("form.required")).max(200),
        external_id: z.string().trim().max(64),
        body_type: z.string().trim().max(64),
        powertrain: z.string().trim().max(64),
        year_start: year,
        year_stop: year,
        active: z.boolean(),
      })
      .refine(
        (v) =>
          !v.year_start.trim() ||
          !v.year_stop.trim() ||
          Number(v.year_stop) >= Number(v.year_start),
        {
          path: ["year_stop"],
          message: t("vehicles.validation.year_range"),
        },
      );
  }, [t]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {t(model ? "vehicles.model.edit_title" : "vehicles.model.create")}
          </DialogTitle>
          <DialogDescription>{t("vehicles.model.form_hint")}</DialogDescription>
        </DialogHeader>
        <AppForm<ModelFormValues>
          key={open ? (model?.uuid ?? "new") : "closed"}
          schema={schema}
          defaultValues={{
            name: model?.name ?? "",
            external_id: model?.external_id ?? "",
            body_type: model?.body_type ?? "",
            powertrain: model?.powertrain ?? "",
            year_start: model?.year_start ? String(model.year_start) : "",
            year_stop: model?.year_stop ? String(model.year_stop) : "",
            active: model?.active ?? true,
          }}
          onSubmit={async (values) => {
            const body = modelFormToInput(values);
            if (model) {
              await update.mutateAsync({ uuid: model.uuid, body });
            } else {
              await create.mutateAsync({ ...body, brand_uuid: brandUuid });
            }
            onOpenChange(false);
          }}
        >
          <FieldGroup className="gap-4">
            <AppInput
              name="name"
              label={t("vehicles.fields.model_name")}
              required
              autoFocus
            />
            <AppInput
              name="external_id"
              label={t("vehicles.fields.external_id")}
              description={t("vehicles.fields.external_id_hint")}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <AppInput
                name="body_type"
                label={t("vehicles.fields.body_type")}
              />
              <AppInput
                name="powertrain"
                label={t("vehicles.fields.powertrain")}
              />
              <AppInput
                name="year_start"
                inputMode="numeric"
                label={t("vehicles.fields.year_start")}
              />
              <AppInput
                name="year_stop"
                inputMode="numeric"
                label={t("vehicles.fields.year_stop")}
              />
            </div>
            <AppSwitch name="active" label={t("vehicles.fields.active")} />
          </FieldGroup>
          {model ? (
            <div className="mt-6">
              <VehicleImageField
                kind="hero"
                label={t("vehicles.fields.hero")}
                hasImage={model.has_hero}
                pending={uploadHero.isPending || deleteHero.isPending}
                preview={
                  <VehicleHeroImage
                    src={model.hero_url}
                    alt={model.name}
                    inherited={!model.has_hero}
                  />
                }
                onUpload={(file) =>
                  uploadHero.mutate({ uuid: model.uuid, file })
                }
                onRemove={() => deleteHero.mutate(model.uuid)}
              />
            </div>
          ) : null}
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
