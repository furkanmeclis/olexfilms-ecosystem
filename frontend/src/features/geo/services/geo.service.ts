import type {
  AssignTerritoryRequest,
  Country,
  District,
  PlateCheck,
  PlateFormat,
  PlateFormatRequest,
  Province,
  Territory,
  TerritoryMatch,
} from "@/features/geo/types";
import { platformRequest } from "@/lib/api/platform-request";

const enc = encodeURIComponent;

export const geoService = {
  countries() {
    return platformRequest<{ items: Country[] }>("GET", "/v1/geo/countries");
  },

  provinces(iso2: string) {
    return platformRequest<{ items: Province[] }>(
      "GET",
      `/v1/geo/countries/${enc(iso2)}/provinces`,
    );
  },

  districts(provinceId: number) {
    return platformRequest<{ items: District[] }>(
      "GET",
      `/v1/geo/provinces/${provinceId}/districts`,
    );
  },

  createDistrict(provinceId: number, name: string) {
    return platformRequest<District>(
      "POST",
      `/v1/platform/geo/provinces/${provinceId}/districts`,
      { body: { name } },
    );
  },

  territories(distributorUuid?: string) {
    return platformRequest<{ items: Territory[] }>(
      "GET",
      "/v1/platform/territories",
      { query: { distributor_uuid: distributorUuid } },
    );
  },

  assignTerritory(body: AssignTerritoryRequest) {
    return platformRequest<Territory>("POST", "/v1/platform/territories", {
      body,
    });
  },

  deleteTerritory(uuid: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/platform/territories/${enc(uuid)}`,
    );
  },

  resolveTerritory(
    countryId: number,
    provinceId?: number | null,
    districtId?: number | null,
  ) {
    return platformRequest<{ match: TerritoryMatch | null }>(
      "GET",
      "/v1/platform/territories/resolve",
      {
        query: {
          country_id: countryId,
          province_id: provinceId ?? undefined,
          district_id: districtId ?? undefined,
        },
      },
    );
  },

  plateFormats() {
    return platformRequest<{ items: PlateFormat[] }>(
      "GET",
      "/v1/plate-formats",
    );
  },

  validatePlate(country: string, plate: string) {
    return platformRequest<PlateCheck>("POST", "/v1/plate-formats/validate", {
      body: { country, plate },
    });
  },

  platformPlateFormats() {
    return platformRequest<{ items: PlateFormat[] }>(
      "GET",
      "/v1/platform/plate-formats",
    );
  },

  createPlateFormat(body: PlateFormatRequest) {
    return platformRequest<PlateFormat>("POST", "/v1/platform/plate-formats", {
      body,
    });
  },

  updatePlateFormat(iso2: string, body: PlateFormatRequest) {
    return platformRequest<PlateFormat>(
      "PATCH",
      `/v1/platform/plate-formats/${enc(iso2)}`,
      { body },
    );
  },

  /** Drag-and-drop order (ISO2 codes); returns the renumbered full list. */
  reorderPlateFormats(countries: string[]) {
    return platformRequest<{ items: PlateFormat[] }>(
      "PUT",
      "/v1/platform/plate-formats/order",
      { body: { countries } },
    );
  },

  deletePlateFormat(iso2: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/platform/plate-formats/${enc(iso2)}`,
    );
  },
};
