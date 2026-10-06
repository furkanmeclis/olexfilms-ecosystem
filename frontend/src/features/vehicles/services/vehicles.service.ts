import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

export type Vehicle = components["schemas"]["Vehicle"];

/**
 * GET /v1/vehicles params (TEC-371): `q` matches a plate / VIN prefix or
 * the car brand and model name; `car_brand_uuid`, `car_model_uuid` and
 * `organization_uuid` are CSV; `sort` one of plate, brand, model,
 * model_year, created_at (`-` desc).
 */
export type VehicleListQuery = {
  q?: string;
  car_brand_uuid?: string;
  car_model_uuid?: string;
  organization_uuid?: string;
  customer_uuid?: string;
  sort?: string;
  limit: number;
  offset: number;
};

export type VehicleListPage = {
  items: Vehicle[];
  total: number;
  limit: number;
  offset: number;
};

/** Tenant vehicle list (TEC-372): vehicles of the customers in scope. */
export const vehiclesService = {
  list(params: VehicleListQuery) {
    return platformRequest<VehicleListPage>("GET", "/v1/vehicles", {
      query: params,
    });
  },
};

export const vehicleListKeys = {
  all: ["vehicles", "list"] as const,
  list: (params: VehicleListQuery) => ["vehicles", "list", params] as const,
};
