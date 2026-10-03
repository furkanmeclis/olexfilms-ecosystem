"use client";

import { useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import { LeafletMap, type MapMarker } from "@/components/common/leaflet-map";
import { FormFieldShell } from "@/components/forms/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { mapConfig } from "@/config/map";
import {
  parseCoordinate,
  roundCoordinate,
  type SettingsFormValues,
} from "@/features/io/schemas/settings-form";
import { useLocale } from "@/providers/locale-provider";

/**
 * Organization map position (TEC-242) in the tenant settings form: typed
 * latitude / longitude or a tap on the map. The dealer finder lists only
 * organizations with a position (TEC-240). Errors are i18n keys from the
 * form schema.
 */
export function CoordinatesField({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const {
    control,
    register,
    setValue,
    formState: { errors },
  } = useFormContext<SettingsFormValues>();
  const latitude = useWatch({ control, name: "latitude" });
  const longitude = useWatch({ control, name: "longitude" });

  const lat = parseCoordinate(latitude);
  const lng = parseCoordinate(longitude);
  const valid =
    lat !== null &&
    lng !== null &&
    Number.isFinite(lat) &&
    Number.isFinite(lng) &&
    Math.abs(lat) <= 90 &&
    Math.abs(lng) <= 180;

  // The map recentres only when these numbers change (not on identity).
  const center = valid ? { lat: lat!, lng: lng! } : mapConfig.defaultCenter;
  const markers = useMemo<MapMarker[]>(
    () =>
      valid
        ? [
            {
              id: "position",
              lat: lat!,
              lng: lng!,
              kind: "point",
              label: t("settings.coordinates.marker"),
            },
          ]
        : [],
    [valid, lat, lng, t],
  );

  const errorText = (name: "latitude" | "longitude") => {
    const message = errors[name]?.message;
    return message ? t(String(message)) : undefined;
  };

  const pick = (point: { lat: number; lng: number }) => {
    if (disabled) return;
    const opts = { shouldDirty: true, shouldValidate: true } as const;
    setValue("latitude", String(roundCoordinate(point.lat)), opts);
    setValue("longitude", String(roundCoordinate(point.lng)), opts);
  };

  const clear = () => {
    const opts = { shouldDirty: true, shouldValidate: true } as const;
    setValue("latitude", "", opts);
    setValue("longitude", "", opts);
  };

  return (
    <div className="space-y-3 sm:col-span-2" data-testid="coordinates-field">
      <div>
        <p className="text-sm font-medium">{t("settings.coordinates.title")}</p>
        <p className="text-muted-foreground text-xs">
          {t("settings.coordinates.hint")}
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <FormFieldShell
          name="latitude"
          label={t("settings.coordinates.latitude")}
          error={errorText("latitude")}
        >
          <Input
            id="latitude"
            inputMode="decimal"
            autoComplete="off"
            placeholder="41.0082"
            disabled={disabled}
            aria-invalid={Boolean(errors.latitude)}
            {...register("latitude")}
          />
        </FormFieldShell>
        <FormFieldShell
          name="longitude"
          label={t("settings.coordinates.longitude")}
          error={errorText("longitude")}
        >
          <Input
            id="longitude"
            inputMode="decimal"
            autoComplete="off"
            placeholder="28.9784"
            disabled={disabled}
            aria-invalid={Boolean(errors.longitude)}
            {...register("longitude")}
          />
        </FormFieldShell>
      </div>
      <LeafletMap
        center={center}
        zoom={valid ? mapConfig.pointZoom : mapConfig.defaultZoom}
        markers={markers}
        onMapClick={pick}
        ariaLabel={t("settings.coordinates.map_label")}
        testId="coordinates-map"
      />
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground text-xs">
          {t("settings.coordinates.pick_hint")}
        </p>
        {!disabled && (latitude || longitude) ? (
          <Button type="button" variant="outline" size="sm" onClick={clear}>
            {t("settings.coordinates.clear")}
          </Button>
        ) : null}
      </div>
    </div>
  );
}
