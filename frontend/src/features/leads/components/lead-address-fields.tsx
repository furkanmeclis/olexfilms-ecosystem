"use client";

import { AsyncCombobox } from "@/components/ui/async-combobox";
import { Label } from "@/components/ui/label";
import {
  countryName,
  useCountries,
  useDistricts,
  useProvinces,
} from "@/features/geo/hooks/use-geo";
import type { LeadFormValues } from "@/features/leads/lib/leads";
import { useLocale } from "@/providers/locale-provider";

type AddressField = "country_id" | "province_id" | "district_id";

export function LeadAddressFields({
  values,
  onChange,
  errors,
  disabled,
}: {
  values: Pick<LeadFormValues, AddressField>;
  onChange: (patch: Partial<LeadFormValues>) => void;
  errors?: Record<string, string>;
  disabled?: boolean;
}) {
  const { t, locale } = useLocale();
  const countries = useCountries();
  const country = countries.data?.find(
    (c) => String(c.id) === values.country_id,
  );
  const provinces = useProvinces(country?.iso2);
  const districts = useDistricts(Number(values.province_id) || undefined);
  const fields = [
    {
      name: "country_id" as const,
      label: "country",
      options: (countries.data ?? []).map((c) => ({
        value: String(c.id),
        label: countryName(c, locale),
        description: c.iso2,
      })),
      unavailable: false,
      change: (value: string) =>
        onChange({ country_id: value, province_id: "", district_id: "" }),
    },
    {
      name: "province_id" as const,
      label: "province",
      options: (provinces.data ?? []).map((p) => ({
        value: String(p.id),
        label: p.name,
      })),
      unavailable: !country || !provinces.data?.length,
      change: (value: string) =>
        onChange({ province_id: value, district_id: "" }),
    },
    {
      name: "district_id" as const,
      label: "district",
      options: (districts.data ?? []).map((d) => ({
        value: String(d.id),
        label: d.name,
      })),
      unavailable: !values.province_id || !districts.data?.length,
      change: (value: string) => onChange({ district_id: value }),
    },
  ];
  return fields.map(({ name, label, options, unavailable, change }) => (
    <div key={name} className="space-y-1.5">
      <Label htmlFor={`lead-${name}`}>{t(`geo.address.${label}`)}</Label>
      <AsyncCombobox
        id={`lead-${name}`}
        data-testid={`lead-${name}`}
        value={values[name]}
        onValueChange={change}
        options={options}
        clearable
        disabled={disabled || unavailable}
        placeholder={t(`geo.address.${label}_placeholder`)}
        searchPlaceholder={t("geo.address.search")}
        emptyText={t("geo.address.empty")}
        aria-invalid={Boolean(errors?.[name])}
        aria-describedby={errors?.[name] ? `lead-${name}-error` : undefined}
      />
      {errors?.[name] ? (
        <p
          id={`lead-${name}-error`}
          className="text-destructive text-xs"
          data-error={name}
        >
          {errors[name]}
        </p>
      ) : null}
    </div>
  ));
}
