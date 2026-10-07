import type { ServerListQuery } from "@/components/entity";
import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Service = Schemas["Service"];
export type ServiceProfit = Schemas["ServiceProfit"];
export type ServiceIncomeInput = Schemas["ServiceIncomeInput"];
export type ServiceIncomeResult = Schemas["ServiceIncomeResult"];
export type ServiceIncomePaymentMethod = Schemas["ServiceIncomePaymentMethod"];
export const SERVICE_INCOME_METHODS: ServiceIncomePaymentMethod[] = [
  "cash",
  "card",
  "cari",
];
export type ServiceCreateInput = Schemas["ServiceCreateInput"];
export type ServiceUpdateInput = Schemas["ServiceUpdateInput"];
export type CustomerSummary = Schemas["CustomerSummary"];
export type CustomerWrite = Schemas["CustomerWrite"];
export type CustomerCreateInput = Schemas["CustomerCreateInput"];
export type Vehicle = Schemas["Vehicle"];
export type VehicleCreateInput = Schemas["VehicleCreateInput"];
export type ServiceItem = Schemas["ServiceItem"];
export type ServiceItemInput = Schemas["ServiceItemInput"];
export type ServiceStockUnit = Schemas["ServiceStockUnit"];
export type ServiceStatus = Schemas["ServiceStatus"];
export type ServiceStatusLog = Schemas["ServiceStatusLog"];
export type ServiceImage = Schemas["ServiceImage"];
export type ServiceWarranty = Schemas["ServiceWarranty"];

/**
 * GET /v1/services params (TEC-183, TEC-377): limit / offset, one sort field
 * (`service_no`, `status`, `created_at`, `updated_at`, `completed_at`,
 * `plate`, `organization`), `q`, CSV `status` / `organization_uuid` and the
 * `created_*` / `completed_*` day windows.
 */
export type ServiceListQuery = ServerListQuery;

/** List export (TEC-377): same filters, search and sort as the list. */
export const SERVICES_EXPORT_PATH = "/v1/services/export";

export type StockUnitQuery = {
  barcode?: string;
  q?: string;
  limit?: number;
  offset?: number;
};

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
  /** Service list in the services.read scope (TEC-183). */
  listServices(params: ServiceListQuery) {
    return platformRequest<Page<Service>>("GET", "/v1/services", {
      query: params,
    });
  },
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
  /** Stock picker of the service organization (TEC-180, q: TEC-182). */
  listStockUnits(uuid: string, params: StockUnitQuery) {
    return platformRequest<{ items: ServiceStockUnit[] }>(
      "GET",
      `/v1/services/${enc(uuid)}/stock-units`,
      { query: { limit: 20, offset: 0, ...params } },
    );
  },
  addItem(uuid: string, body: ServiceItemInput) {
    return platformRequest<Service>("POST", `/v1/services/${enc(uuid)}/items`, {
      body,
    });
  },
  removeItem(uuid: string, item: string) {
    return platformRequest<Service>(
      "DELETE",
      `/v1/services/${enc(uuid)}/items/${enc(item)}`,
    );
  },
  /**
   * Income of a completed service (TEC-343): cash (cash account), card
   * (bank account) or cari (the customer's cari, no account). 409 when one
   * is already recorded; a warranty re-apply service answers a warning.
   */
  recordIncome(uuid: string, body: ServiceIncomeInput) {
    return platformRequest<ServiceIncomeResult>(
      "POST",
      `/v1/services/${enc(uuid)}/income`,
      { body },
    );
  },
  /** Reverses the recorded income (append-only reversal rows). */
  deleteIncome(uuid: string, reason?: string) {
    return platformRequest<ServiceIncomeResult>(
      "DELETE",
      `/v1/services/${enc(uuid)}/income`,
      { body: reason ? { reason } : {} },
    );
  },
  transition(uuid: string, status: ServiceStatus, note?: string) {
    return platformRequest<Service>(
      "POST",
      `/v1/services/${enc(uuid)}/transitions`,
      { body: note ? { status, note } : { status } },
    );
  },
};

/** Browser URL of a service image (the API url goes through the BFF). */
export function serviceImageSrc(url: string): string {
  return `${apiConfig.baseUrl.replace(/\/$/, "")}${url}`;
}

export const serviceWizardKeys = {
  all: ["service-wizard"] as const,
  list: (params: ServiceListQuery) =>
    ["service-wizard", "list", params] as const,
  service: (uuid: string) => ["service-wizard", "service", uuid] as const,
  customers: (q: string) => ["service-wizard", "customers", q] as const,
  vehicles: (customerUuid: string) =>
    ["service-wizard", "vehicles", customerUuid] as const,
  stock: (uuid: string, params: StockUnitQuery) =>
    ["service-wizard", "stock", uuid, params] as const,
  categories: ["service-wizard", "categories"] as const,
};
