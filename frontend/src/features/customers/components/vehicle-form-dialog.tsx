"use client";

import { useMutation } from "@tanstack/react-query";
import { useCallback, useState } from "react";
import { toast } from "sonner";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { customerErrorMessage } from "@/features/customers/lib/errors";
import {
  EMPTY_VEHICLE_FORM,
  PLATE_MAX,
  buildVehicleCreate,
  buildVehicleUpdate,
  plateError,
  validateVehicleForm,
  vehicleFormFromVehicle,
  type VehicleFormValues,
} from "@/features/customers/lib/form";
import {
  customersService,
  type Vehicle,
} from "@/features/customers/services/customers.service";
import { PlateBadge } from "@/features/geo/components/plate-badge";
import { usePlateFormats } from "@/features/geo/hooks/use-geo";
import { VinField } from "@/features/services/components/vin-field";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { vehicleCatalogService } from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

const DEFAULT_PLATE_COUNTRY = "TR";

async function loadBrands(q: string): Promise<ComboboxOption[]> {
  const page = await vehicleCatalogService.listBrands({
    limit: 20,
    offset: 0,
    q: q || undefined,
    active: "true",
  });
  return page.items.map((b) => ({ value: b.uuid, label: b.name }));
}

/**
 * Vehicle create / edit of a customer (TEC-163): car brand (with its logo)
 * and model from the vehicle catalog, plate checked against the plate
 * country's format like geo.ValidatePlate, optional year and VIN.
 */
export function VehicleFormDialog({
  open,
  customerUuid,
  vehicle,
  onClose,
  onSaved,
}: {
  open: boolean;
  customerUuid: string;
  vehicle: Vehicle | null;
  onClose: () => void;
  onSaved: (vehicle: Vehicle) => void;
}) {
  const { t } = useLocale();
  return (
    <Dialog open={open} onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {vehicle
              ? t("customers.vehicle.edit_title")
              : t("customers.vehicle.create_title")}
          </DialogTitle>
          <DialogDescription>
            {t("customers.vehicle.description")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <VehicleForm
            key={vehicle?.uuid ?? "new"}
            customerUuid={customerUuid}
            vehicle={vehicle}
            onCancel={onClose}
            onSaved={onSaved}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

export function VehicleForm({
  customerUuid,
  vehicle,
  onCancel,
  onSaved,
}: {
  customerUuid: string;
  vehicle: Vehicle | null;
  onCancel: () => void;
  onSaved: (vehicle: Vehicle) => void;
}) {
  const { t } = useLocale();
  const formats = usePlateFormats();
  const original = vehicle ? vehicleFormFromVehicle(vehicle) : null;
  const [values, setValues] = useState<VehicleFormValues>(
    () => original ?? { ...EMPTY_VEHICLE_FORM, plate_country: "" },
  );
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitted, setSubmitted] = useState(false);
  const [serverVinError, setServerVinError] = useState<string | null>(null);
  const [brandOptions, setBrandOptions] = useState<ComboboxOption[]>(
    original?.brand_uuid
      ? [{ value: original.brand_uuid, label: original.brand_name }]
      : [],
  );

  // New vehicles default to TR when that format is active; an explicit
  // country keeps the client check identical to the server's.
  const items = formats.data;
  const country =
    values.plate_country ||
    (vehicle
      ? ""
      : (items?.find((f) => f.country_iso2 === DEFAULT_PLATE_COUNTRY)
          ?.country_iso2 ??
        items?.[0]?.country_iso2 ??
        ""));
  const current = { ...values, plate_country: country };
  const format = items?.find((f) => f.country_iso2 === country);

  const patch = (p: Partial<VehicleFormValues>) => {
    setValues((v) => ({ ...v, plate_country: country, ...p }));
    setErrors((e) => {
      const next = { ...e };
      for (const k of Object.keys(p)) delete next[k];
      return next;
    });
  };

  const loadBrandOptions = useCallback(async (q: string) => {
    const options = await loadBrands(q);
    setBrandOptions((prev) => {
      const seen = new Set(prev.map((o) => o.value));
      return [...prev, ...options.filter((o) => !seen.has(o.value))];
    });
    return options;
  }, []);
  const brandUuid = values.brand_uuid;
  const loadModels = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      if (!brandUuid) return [];
      const page = await vehicleCatalogService.listModels({
        brand_uuid: brandUuid,
        limit: 50,
        offset: 0,
        q: q || undefined,
        active: "true",
      });
      return page.items.map((m) => ({ value: m.uuid, label: m.name }));
    },
    [brandUuid],
  );

  const save = useMutation({
    mutationFn: () =>
      vehicle && original
        ? customersService.updateVehicle(
            vehicle.uuid,
            buildVehicleUpdate(current, original),
          )
        : customersService.createVehicle(
            buildVehicleCreate(customerUuid, current),
          ),
    onSuccess: (saved) => {
      toast.success(
        vehicle ? t("customers.vehicle.saved") : t("customers.vehicle.created"),
      );
      for (const w of saved.warnings ?? []) {
        toast.warning(t(`customers.vehicle.warnings.${w}`));
      }
      onSaved(saved);
    },
    onError: (err) => {
      if (isApiError(err)) {
        const fields = err.fieldErrors();
        if (fields.vin)
          setServerVinError(t("customers.validation.vin_invalid"));
        if (fields.plate || err.code === "INVALID_PLATE") {
          setErrors((e) => ({
            ...e,
            plate: t("customers.validation.plate_invalid", {
              example: format?.example ?? "",
            }),
          }));
        }
      }
      toast.error(customerErrorMessage(err, t, t("customers.vehicle.failed")));
    },
  });

  const submit = () => {
    setSubmitted(true);
    const next = validateVehicleForm(current, items);
    const localized: Record<string, string> = {};
    for (const [k, code] of Object.entries(next)) {
      if (k === "vin") continue; // VinField shows its own message.
      localized[k] = t(`customers.validation.${code}`, {
        example: format?.example ?? "",
        max: new Date().getFullYear() + 1,
      });
    }
    setErrors(localized);
    if (Object.keys(next).length === 0) save.mutate();
  };

  const err = (key: string) =>
    errors[key] ? (
      <p className="text-destructive text-xs" data-error={key}>
        {errors[key]}
      </p>
    ) : null;
  const livePlateError =
    current.plate.trim() && plateError(current.plate, country, items);

  return (
    <div className="space-y-4" data-testid="vehicle-form">
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="vehicle-brand">{t("customers.vehicle.brand")}</Label>
          <div className="flex items-center gap-2">
            <VehicleBrandLogo
              uuid={current.brand_uuid || undefined}
              name={current.brand_name || undefined}
              height={36}
            />
            <AsyncCombobox
              id="vehicle-brand"
              className="flex-1"
              value={current.brand_uuid}
              initialOptions={brandOptions}
              onValueChange={(value) => {
                const picked = brandOptions.find((o) => o.value === value);
                patch({
                  brand_uuid: value,
                  brand_name: picked?.label ?? "",
                  model_uuid: "",
                  model_name: "",
                });
              }}
              loadOptions={loadBrandOptions}
              placeholder={t("customers.vehicle.brand_placeholder")}
              searchPlaceholder={t("customers.vehicle.search")}
              emptyText={t("customers.vehicle.none_found")}
              clearable
            />
          </div>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="vehicle-model">{t("customers.vehicle.model")}</Label>
          <AsyncCombobox
            key={current.brand_uuid}
            id="vehicle-model"
            value={current.model_uuid}
            initialOptions={
              current.model_uuid && current.model_name
                ? [{ value: current.model_uuid, label: current.model_name }]
                : undefined
            }
            onValueChange={(value) => patch({ model_uuid: value })}
            loadOptions={loadModels}
            disabled={!current.brand_uuid}
            placeholder={t("customers.vehicle.model_placeholder")}
            searchPlaceholder={t("customers.vehicle.search")}
            emptyText={t("customers.vehicle.none_found")}
            clearable
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="vehicle-plate-country">
            {t("customers.vehicle.plate_country")}
            <span className="text-destructive ms-1">*</span>
          </Label>
          <select
            id="vehicle-plate-country"
            name="plate_country"
            className="border-input bg-background h-9 w-full rounded-md border px-2 text-sm"
            value={country}
            aria-invalid={errors.plate_country ? true : undefined}
            onChange={(e) => patch({ plate_country: e.target.value })}
          >
            {country && !format ? (
              <option value={country}>{country}</option>
            ) : null}
            {(items ?? []).map((f) => (
              <option key={f.country_iso2} value={f.country_iso2}>
                {f.country_iso2} · {f.country_name_en}
              </option>
            ))}
          </select>
          {err("plate_country")}
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="vehicle-plate">
            {t("customers.vehicle.plate")}
            <span className="text-destructive ms-1">*</span>
          </Label>
          <Input
            id="vehicle-plate"
            name="plate"
            dir="ltr"
            className="uppercase"
            maxLength={PLATE_MAX + 4}
            placeholder={format?.example}
            value={current.plate}
            aria-invalid={errors.plate ? true : undefined}
            onChange={(e) => patch({ plate: e.target.value })}
          />
          {err("plate") ??
            (livePlateError === "plate_invalid" ? (
              <p className="text-muted-foreground text-xs" data-hint="plate">
                {t("customers.validation.plate_invalid", {
                  example: format?.example ?? "",
                })}
              </p>
            ) : null)}
          {format && current.plate.trim() ? (
            <PlateBadge plate={current.plate} format={format} />
          ) : null}
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="vehicle-year">{t("customers.vehicle.year")}</Label>
          <Input
            id="vehicle-year"
            name="model_year"
            inputMode="numeric"
            dir="ltr"
            maxLength={4}
            value={current.model_year}
            aria-invalid={errors.model_year ? true : undefined}
            onChange={(e) => patch({ model_year: e.target.value })}
          />
          {err("model_year")}
        </div>
      </div>
      <VinField
        value={current.vin}
        onChange={(v) => {
          patch({ vin: v });
          setServerVinError(null);
        }}
        showError={submitted}
        serverError={serverVinError}
      />
      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          data-testid="vehicle-submit"
          disabled={save.isPending}
          onClick={submit}
        >
          {vehicle
            ? t("customers.vehicle.save")
            : t("customers.vehicle.create")}
        </Button>
      </DialogFooter>
    </div>
  );
}
