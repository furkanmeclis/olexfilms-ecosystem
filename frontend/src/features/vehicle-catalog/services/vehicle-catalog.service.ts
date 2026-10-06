import type { components } from "@/generated/api";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type VehicleBrand = Schemas["VehicleBrand"];
export type VehicleModel = Schemas["VehicleModel"];
export type VehicleBrandInput = Schemas["VehicleBrandInput"];
export type VehicleModelInput = Schemas["VehicleModelInput"];
export type VehicleModelFacets = Schemas["VehicleModelFacets"];

export type VehiclePage<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/**
 * List query (TEC-369): paging, single-field `sort`, `q` and the column
 * filters, passed through as query params. Brands: `active`, `has_logo`;
 * models also `body_type` / `powertrain` (CSV) and `year_min` / `year_max`.
 */
export type VehicleListParams = {
  limit: number;
  offset: number;
  q?: string;
  sort?: string;
  /** "true" / "false"; undefined lists both (super_admin only). */
  active?: string;
} & Record<string, string | number | undefined>;

export type VehicleModelListParams = VehicleListParams & {
  brand_uuid: string;
};

/**
 * TEC-149 vehicle catalog. Reads go through `/v1/vehicle-catalog/*`
 * (vehicle_catalog.read), writes through `/v1/platform/vehicle-catalog/*`
 * (vehicle_catalog.write, super_admin only).
 */
export const vehicleCatalogService = {
  listBrands(params: VehicleListParams) {
    return platformRequest<VehiclePage<VehicleBrand>>(
      "GET",
      "/v1/vehicle-catalog/brands",
      { query: params },
    );
  },
  getBrand(uuid: string) {
    return platformRequest<VehicleBrand>(
      "GET",
      `/v1/vehicle-catalog/brands/${encodeURIComponent(uuid)}`,
    );
  },
  createBrand(body: VehicleBrandInput) {
    return platformRequest<VehicleBrand>(
      "POST",
      "/v1/platform/vehicle-catalog/brands",
      { body },
    );
  },
  updateBrand(uuid: string, body: VehicleBrandInput) {
    return platformRequest<VehicleBrand>(
      "PATCH",
      `/v1/platform/vehicle-catalog/brands/${encodeURIComponent(uuid)}`,
      { body },
    );
  },
  deleteBrand(uuid: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/platform/vehicle-catalog/brands/${encodeURIComponent(uuid)}`,
    );
  },
  uploadBrandImage(uuid: string, kind: "logo" | "hero", file: File) {
    const form = new FormData();
    form.append(kind, file);
    return platformFormRequest<VehicleBrand>(
      "PUT",
      `/v1/platform/vehicle-catalog/brands/${encodeURIComponent(uuid)}/${kind}`,
      form,
    );
  },
  deleteBrandImage(uuid: string, kind: "logo" | "hero") {
    return platformRequest<VehicleBrand>(
      "DELETE",
      `/v1/platform/vehicle-catalog/brands/${encodeURIComponent(uuid)}/${kind}`,
    );
  },
  listModels(params: VehicleModelListParams) {
    return platformRequest<VehiclePage<VehicleModel>>(
      "GET",
      "/v1/vehicle-catalog/models",
      { query: params },
    );
  },
  /** Distinct body_type / powertrain values with counts (TEC-369). */
  modelFacets(params: { brand_uuid?: string; active?: string }) {
    return platformRequest<VehicleModelFacets>(
      "GET",
      "/v1/vehicle-catalog/models/facets",
      { query: params },
    );
  },
  createModel(body: VehicleModelInput) {
    return platformRequest<VehicleModel>(
      "POST",
      "/v1/platform/vehicle-catalog/models",
      { body },
    );
  },
  updateModel(uuid: string, body: VehicleModelInput) {
    return platformRequest<VehicleModel>(
      "PATCH",
      `/v1/platform/vehicle-catalog/models/${encodeURIComponent(uuid)}`,
      { body },
    );
  },
  deleteModel(uuid: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/platform/vehicle-catalog/models/${encodeURIComponent(uuid)}`,
    );
  },
  uploadModelHero(uuid: string, file: File) {
    const form = new FormData();
    form.append("hero", file);
    return platformFormRequest<VehicleModel>(
      "PUT",
      `/v1/platform/vehicle-catalog/models/${encodeURIComponent(uuid)}/hero`,
      form,
    );
  },
  deleteModelHero(uuid: string) {
    return platformRequest<VehicleModel>(
      "DELETE",
      `/v1/platform/vehicle-catalog/models/${encodeURIComponent(uuid)}/hero`,
    );
  },
};
