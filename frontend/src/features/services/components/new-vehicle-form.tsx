"use client";

import { useMutation } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { VinField } from "@/features/services/components/vin-field";
import { normalizeVin, validateVin } from "@/features/services/lib/vin";
import {
  serviceWizardService,
  type Vehicle,
} from "@/features/services/services/service-wizard.service";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { vehicleCatalogService } from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

async function loadBrands(q: string): Promise<ComboboxOption[]> {
  const page = await vehicleCatalogService.listBrands({
    limit: 20,
    offset: 0,
    q: q || undefined,
    active: "true",
  });
  return page.items.map((b) => ({ value: b.uuid, label: b.name }));
}

type Errors = Partial<Record<"plate" | "brand" | "model" | "year", string>>;

/**
 * Vehicle create for the picked customer (POST /v1/vehicles, TEC-160). The
 * service needs a car brand and model (TEC-179), so both are required here;
 * the VIN is optional with the backend rules.
 */
export function NewVehicleForm({
  customerUuid,
  onCreated,
  onCancel,
}: {
  customerUuid: string;
  onCreated: (vehicle: Vehicle) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const [plate, setPlate] = useState("");
  const [plateCountry, setPlateCountry] = useState("");
  const [brand, setBrand] = useState<ComboboxOption | null>(null);
  const [model, setModel] = useState("");
  const [year, setYear] = useState("");
  const [vin, setVin] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const [errors, setErrors] = useState<Errors>({});
  const [serverVinError, setServerVinError] = useState<string | null>(null);
  const [brandOptions, setBrandOptions] = useState<ComboboxOption[]>([]);

  const brandUuid = brand?.value ?? "";
  const loadBrandOptions = useCallback(async (q: string) => {
    const options = await loadBrands(q);
    setBrandOptions((prev) => {
      const seen = new Set(prev.map((o) => o.value));
      return [...prev, ...options.filter((o) => !seen.has(o.value))];
    });
    return options;
  }, []);
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

  const create = useMutation({
    mutationFn: () => {
      const normalizedVin = normalizeVin(vin);
      return serviceWizardService.createVehicle({
        customer_uuid: customerUuid,
        plate: plate.trim(),
        ...(plateCountry.trim()
          ? { plate_country: plateCountry.trim().toUpperCase() }
          : {}),
        car_brand_uuid: brandUuid,
        car_model_uuid: model,
        model_year: year.trim() ? Number(year) : null,
        ...(normalizedVin ? { vin: normalizedVin } : {}),
      });
    },
    onSuccess: (vehicle) => {
      appToast.success(t("services.vehicle.created"));
      for (const w of vehicle.warnings ?? []) {
        appToast.warning(t(`services.vehicle.warnings.${w}`));
      }
      onCreated(vehicle);
    },
    onError: (error: unknown) => {
      if (isApiError(error)) {
        const fields = error.fieldErrors();
        if (fields.vin) setServerVinError(t("services.vin.errors.server"));
        if (fields.plate || error.code === "INVALID_PLATE") {
          setErrors((e) => ({
            ...e,
            plate: t("services.vehicle.plate_invalid"),
          }));
        }
        appToast.error(error.message);
        return;
      }
      appToast.error(t("services.wizard.save_failed"));
    },
  });

  const submit = () => {
    setSubmitted(true);
    const next: Errors = {};
    if (!plate.trim()) next.plate = t("services.vehicle.plate_required");
    if (!brandUuid) next.brand = t("services.vehicle.brand_required");
    if (!model) next.model = t("services.vehicle.model_required");
    const y = year.trim();
    const maxYear = new Date().getFullYear() + 1;
    if (y && (!/^\d{4}$/.test(y) || Number(y) < 1900 || Number(y) > maxYear)) {
      next.year = t("services.vehicle.year_invalid", { max: maxYear });
    }
    setErrors(next);
    if (Object.keys(next).length > 0 || validateVin(vin)) return;
    create.mutate();
  };

  const errorText = (key: keyof Errors) =>
    errors[key] ? (
      <p className="text-destructive text-xs">{errors[key]}</p>
    ) : null;

  return (
    <div
      className="bg-muted/30 space-y-4 rounded-lg border p-4"
      data-testid="new-vehicle-form"
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="new-vehicle-brand">
            {t("services.vehicle.brand")}
            <span className="text-destructive ms-1">*</span>
          </Label>
          <div className="flex items-center gap-2">
            <VehicleBrandLogo
              uuid={brand?.value}
              name={brand?.label}
              height={36}
            />
            <AsyncCombobox
              id="new-vehicle-brand"
              className="flex-1"
              value={brandUuid}
              onValueChange={(value) => {
                const picked =
                  brandOptions.find((o) => o.value === value) ?? null;
                setBrand(value ? (picked ?? { value, label: "" }) : null);
                setModel("");
              }}
              loadOptions={loadBrandOptions}
              placeholder={t("services.vehicle.brand_placeholder")}
              searchPlaceholder={t("services.vehicle.search")}
              emptyText={t("services.vehicle.none_found")}
              aria-invalid={errors.brand ? true : undefined}
            />
          </div>
          {errorText("brand")}
        </div>
        <div className="space-y-2">
          <Label htmlFor="new-vehicle-model">
            {t("services.vehicle.model")}
            <span className="text-destructive ms-1">*</span>
          </Label>
          <AsyncCombobox
            key={brandUuid}
            id="new-vehicle-model"
            value={model}
            onValueChange={setModel}
            loadOptions={loadModels}
            disabled={!brandUuid}
            placeholder={t("services.vehicle.model_placeholder")}
            searchPlaceholder={t("services.vehicle.search")}
            emptyText={t("services.vehicle.none_found")}
            aria-invalid={errors.model ? true : undefined}
          />
          {errorText("model")}
        </div>
        <div className="space-y-2">
          <Label htmlFor="new-vehicle-plate">
            {t("services.vehicle.plate")}
            <span className="text-destructive ms-1">*</span>
          </Label>
          <Input
            id="new-vehicle-plate"
            name="plate"
            dir="ltr"
            className="uppercase"
            maxLength={20}
            value={plate}
            aria-invalid={errors.plate ? true : undefined}
            onChange={(e) => setPlate(e.target.value)}
          />
          {errorText("plate")}
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="new-vehicle-country">
              {t("services.vehicle.plate_country")}
            </Label>
            <Input
              id="new-vehicle-country"
              name="plate_country"
              dir="ltr"
              className="uppercase"
              maxLength={2}
              placeholder="TR"
              value={plateCountry}
              onChange={(e) => setPlateCountry(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-vehicle-year">
              {t("services.vehicle.year")}
            </Label>
            <Input
              id="new-vehicle-year"
              name="model_year"
              inputMode="numeric"
              dir="ltr"
              maxLength={4}
              value={year}
              aria-invalid={errors.year ? true : undefined}
              onChange={(e) => setYear(e.target.value)}
            />
            {errorText("year")}
          </div>
        </div>
      </div>
      <VinField
        value={vin}
        onChange={(v) => {
          setVin(v);
          setServerVinError(null);
        }}
        showError={submitted}
        serverError={serverVinError}
      />
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          data-testid="new-vehicle-submit"
          disabled={create.isPending}
          onClick={submit}
        >
          {t("services.vehicle.create")}
        </Button>
      </div>
    </div>
  );
}
