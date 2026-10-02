import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Service = Schemas["Service"];
export type ServiceCreateInput = Schemas["ServiceCreateInput"];
export type ServiceUpdateInput = Schemas["ServiceUpdateInput"];
export type CustomerSummary = Schemas["CustomerSummary"];
export type CustomerWrite = Schemas["CustomerWrite"];
export type CustomerCreateInput = Schemas["CustomerCreateInput"];
export type Vehicle = Schemas["Vehicle"];
export type VehicleCreateInput = Schemas["VehicleCreateInput"];

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

const enc = encodeURIComponent;

/**
 * Service wizard API: draft services (TEC-179) and the customers / vehicles
 * it picks or creates (TEC-160). Every call goes through the BFF with the
 * active organization of the session.
 */
export const serviceWizardService = {
  getService(uuid: string) {
    return platformRequest<Service>("GET", `/v1/services/${enc(uuid)}`);
  },
  createService(body: ServiceCreateInput) {
    return platformRequest<Service>("POST", "/v1/services", { body });
  },
  updateService(uuid: string, body: ServiceUpdateInput) {
    return platformRequest<Service>("PATCH", `/v1/services/${enc(uuid)}`, {
      body,
    });
  },
  listCustomers(params: { q?: string; limit?: number; offset?: number }) {
    return platformRequest<Page<CustomerSummary>>("GET", "/v1/customers", {
      query: { status: "active", limit: 10, offset: 0, ...params },
    });
  },
  createCustomer(body: CustomerCreateInput) {
    return platformRequest<CustomerWrite>("POST", "/v1/customers", { body });
  },
  listVehicles(customerUuid: string) {
    return platformRequest<Page<Vehicle>>("GET", "/v1/vehicles", {
      query: { customer_uuid: customerUuid, limit: 50, offset: 0 },
    });
  },
  createVehicle(body: VehicleCreateInput) {
    return platformRequest<Vehicle>("POST", "/v1/vehicles", { body });
  },
};

export const serviceWizardKeys = {
  all: ["service-wizard"] as const,
  service: (uuid: string) => ["service-wizard", "service", uuid] as const,
  customers: (q: string) => ["service-wizard", "customers", q] as const,
  vehicles: (customerUuid: string) =>
    ["service-wizard", "vehicles", customerUuid] as const,
};
