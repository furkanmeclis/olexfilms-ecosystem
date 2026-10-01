"use client";

import { useQuery } from "@tanstack/react-query";

import { geoService } from "@/features/geo/services/geo.service";
import type { Country } from "@/features/geo/types";

// Geography changes rarely; keep it for the session.
const GEO_STALE_MS = 30 * 60 * 1000;

export const geoKeys = {
  all: ["geo"] as const,
  countries: ["geo", "countries"] as const,
  provinces: (iso2: string) => ["geo", "provinces", iso2] as const,
  districts: (provinceId: number) => ["geo", "districts", provinceId] as const,
  territories: ["geo", "territories"] as const,
  plateFormats: ["geo", "plate-formats"] as const,
  platformPlateFormats: ["geo", "platform-plate-formats"] as const,
};

export function useCountries() {
  return useQuery({
    queryKey: geoKeys.countries,
    queryFn: () => geoService.countries(),
    staleTime: GEO_STALE_MS,
    select: (data) => data.items,
  });
}

export function useProvinces(iso2: string | undefined) {
  return useQuery({
    queryKey: geoKeys.provinces(iso2 ?? ""),
    queryFn: () => geoService.provinces(iso2 ?? ""),
    enabled: Boolean(iso2),
    staleTime: GEO_STALE_MS,
    select: (data) => data.items,
  });
}

export function useDistricts(provinceId: number | undefined) {
  return useQuery({
    queryKey: geoKeys.districts(provinceId ?? 0),
    queryFn: () => geoService.districts(provinceId ?? 0),
    enabled: Boolean(provinceId),
    staleTime: GEO_STALE_MS,
    select: (data) => data.items,
  });
}

export function usePlateFormats() {
  return useQuery({
    queryKey: geoKeys.plateFormats,
    queryFn: () => geoService.plateFormats(),
    staleTime: GEO_STALE_MS,
    select: (data) => data.items,
  });
}

/** Country name in the UI language (Turkish names for tr, English otherwise). */
export function countryName(
  country: Pick<Country, "name_en" | "name_tr">,
  locale: string,
) {
  return locale === "tr" ? country.name_tr : country.name_en;
}
