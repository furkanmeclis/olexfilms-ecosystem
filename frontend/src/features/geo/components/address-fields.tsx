"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { useFormContext, useWatch } from "react-hook-form";

import { AppCombobox, type ComboboxOption } from "@/components/forms";
import {
  countryName,
  geoKeys,
  useCountries,
  useDistricts,
  useProvinces,
} from "@/features/geo/hooks/use-geo";
import { geoService } from "@/features/geo/services/geo.service";
import { useLocale } from "@/providers/locale-provider";

type AddressFieldsProps = {
  countryName?: string;
  provinceName?: string;
  districtName?: string;
  /** Shows which distributor's territory covers the chosen address. */
  showTerritory?: boolean;
};

function toNumber(value: unknown): number | undefined {
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? n : undefined;
}

/**
 * Country > province > district pickers for react-hook-form. Values are
 * string ids ("" = none); changing a level clears the levels below it.
 */
export function AddressFields({
  countryName: countryField = "country_id",
  provinceName: provinceField = "province_id",
  districtName: districtField = "district_id",
  showTerritory = false,
}: AddressFieldsProps) {
  const { t, locale } = useLocale();
  const form = useFormContext();
  const [countryValue, provinceValue, districtValue] = useWatch({
    control: form.control,
    name: [countryField, provinceField, districtField],
  });
  const countryId = toNumber(countryValue);
  const provinceId = toNumber(provinceValue);
  const districtId = toNumber(districtValue);

  const countries = useCountries();
  const country = countries.data?.find((c) => c.id === countryId);
  const provinces = useProvinces(country?.iso2);
  const districts = useDistricts(provinceId);

  const countryOptions = useMemo<ComboboxOption[]>(
    () =>
      (countries.data ?? [])
        .map((c) => ({
          value: String(c.id),
          label: countryName(c, locale),
          description: c.iso2,
        }))
        .sort((a, b) => a.label.localeCompare(b.label, locale)),
    [countries.data, locale],
  );
  const provinceOptions = useMemo<ComboboxOption[]>(
    () =>
      (provinces.data ?? []).map((p) => ({
        value: String(p.id),
        label: p.name,
        description: p.code,
      })),
    [provinces.data],
  );
  const districtOptions = useMemo<ComboboxOption[]>(
    () =>
      (districts.data ?? []).map((d) => ({
        value: String(d.id),
        label: d.name,
      })),
    [districts.data],
  );

  const territory = useQuery({
    queryKey: [
      ...geoKeys.territories,
      "resolve",
      countryId,
      provinceId,
      districtId,
    ],
    queryFn: () =>
      geoService.resolveTerritory(countryId ?? 0, provinceId, districtId),
    enabled: showTerritory && Boolean(countryId),
  });

  const clear = (...names: string[]) => {
    for (const name of names) {
      form.setValue(name, "", { shouldDirty: true });
    }
  };

  return (
    <>
      <AppCombobox
        name={countryField}
        label={t("geo.address.country")}
        placeholder={t("geo.address.country_placeholder")}
        searchPlaceholder={t("geo.address.search")}
        emptyText={t("geo.address.empty")}
        options={countryOptions}
        clearable
        onValueChange={() => clear(provinceField, districtField)}
      />
      <AppCombobox
        name={provinceField}
        label={t("geo.address.province")}
        placeholder={t("geo.address.province_placeholder")}
        searchPlaceholder={t("geo.address.search")}
        emptyText={t("geo.address.empty")}
        options={provinceOptions}
        disabled={!country || provinceOptions.length === 0}
        clearable
        onValueChange={() => clear(districtField)}
      />
      <AppCombobox
        name={districtField}
        label={t("geo.address.district")}
        placeholder={t("geo.address.district_placeholder")}
        searchPlaceholder={t("geo.address.search")}
        emptyText={t("geo.address.empty")}
        options={districtOptions}
        disabled={!provinceId || districtOptions.length === 0}
        clearable
      />
      {showTerritory && countryId ? (
        <p className="text-muted-foreground self-end pb-2 text-sm">
          {territory.data?.match
            ? t("geo.address.territory_match", {
                name: territory.data.match.organization_name,
              })
            : t("geo.address.territory_none")}
        </p>
      ) : null}
    </>
  );
}

/** Converts the picker strings to API ids (undefined when empty). */
export function addressIds(values: {
  country_id?: string;
  province_id?: string;
  district_id?: string;
}) {
  const country = toNumber(values.country_id);
  return {
    country_id: country ?? null,
    province_id: country ? (toNumber(values.province_id) ?? null) : null,
    district_id: country ? (toNumber(values.district_id) ?? null) : null,
  };
}
