import type { components } from "@/generated/api";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type CustomerSummary = Schemas["CustomerSummary"];
export type CustomerDetail = Schemas["CustomerDetail"];
export type CustomerWrite = Schemas["CustomerWrite"];
export type CustomerCreateInput = Schemas["CustomerCreateInput"];
export type CustomerUpdateInput = Schemas["CustomerUpdateInput"];
export type CustomerUpgradeInput = Schemas["CustomerUpgradeInput"];
export type CustomerUpgrade = Schemas["CustomerUpgrade"];
export type CustomerAnonymizeResult = Schemas["CustomerAnonymizeResult"];
export type CustomerType = Schemas["CustomerType"];
export type CustomerStatus = CustomerSummary["status"];
export type Vehicle = Schemas["Vehicle"];
export type VehicleCreateInput = Schemas["VehicleCreateInput"];
export type VehicleUpdateInput = Schemas["VehicleUpdateInput"];
export type ExportJob = Schemas["ExportJob"];
export type DataExportFormat = Schemas["CustomerDataExportInput"]["format"];

/** GET /v1/customers filters (TEC-163). */
export type CustomerListQuery = {
  q?: string;
  status?: CustomerStatus;
  limit: number;
  offset: number;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

export type DealerOption = { uuid: string; name: string };

const enc = encodeURIComponent;

/**
 * Customers and their vehicles (TEC-159..161) through the BFF with the
 * active organization. Writes go through platformRequest, so a
 * STEP_UP_REQUIRED answer (anonymize, data export) opens the step-up dialog
 * and retries (withStepUpRetry).
 */
export const customersService = {
  list(params: CustomerListQuery) {
    return platformRequest<Page<CustomerSummary>>("GET", "/v1/customers", {
      query: params,
    });
  },
  get(uuid: string) {
    return platformRequest<CustomerDetail>("GET", `/v1/customers/${enc(uuid)}`);
  },
  create(body: CustomerCreateInput) {
    return platformRequest<CustomerWrite>("POST", "/v1/customers", { body });
  },
  update(uuid: string, body: CustomerUpdateInput) {
    return platformRequest<CustomerWrite>(
      "PATCH",
      `/v1/customers/${enc(uuid)}`,
      { body },
    );
  },
  upgrade(uuid: string, body: CustomerUpgradeInput) {
    return platformRequest<CustomerUpgrade>(
      "POST",
      `/v1/customers/${enc(uuid)}/upgrade-to-dealer`,
      { body },
    );
  },
  anonymize(uuid: string) {
    return platformRequest<CustomerAnonymizeResult>(
      "POST",
      `/v1/customers/${enc(uuid)}/anonymize`,
    );
  },
  requestDataExport(uuid: string, format: DataExportFormat, locale?: string) {
    return platformRequest<ExportJob>(
      "POST",
      `/v1/customers/${enc(uuid)}/data-export`,
      { body: locale ? { format, locale } : { format } },
    );
  },
  getDataExport(uuid: string) {
    return platformRequest<ExportJob>(
      "GET",
      `/v1/customer-data-exports/${enc(uuid)}`,
    );
  },
  async downloadDataExport(job: ExportJob) {
    const { blob, filename } = await platformDownloadFile(
      `/v1/customer-data-exports/${enc(job.uuid)}/download`,
    );
    triggerBrowserDownload(
      blob,
      filename ?? `customer-data-${job.uuid}.${job.format}`,
    );
  },
  /** Dealers in the organizations.read scope (upgrade target picker). */
  async listDealers(): Promise<DealerOption[]> {
    const data = await platformRequest<{
      items: { uuid: string; name: string }[];
    }>("GET", "/v1/tenant/organizations", {
      query: { type: "dealer", limit: 200 },
    });
    return (data.items ?? []).map((o) => ({ uuid: o.uuid, name: o.name }));
  },
  listVehicles(customerUuid: string) {
    return platformRequest<Page<Vehicle>>("GET", "/v1/vehicles", {
      query: { customer_uuid: customerUuid, limit: 100, offset: 0 },
    });
  },
  createVehicle(body: VehicleCreateInput) {
    return platformRequest<Vehicle>("POST", "/v1/vehicles", { body });
  },
  updateVehicle(uuid: string, body: VehicleUpdateInput) {
    return platformRequest<Vehicle>("PATCH", `/v1/vehicles/${enc(uuid)}`, {
      body,
    });
  },
};

export const customerKeys = {
  all: ["customers"] as const,
  list: (params: CustomerListQuery) => ["customers", "list", params] as const,
  detail: (uuid: string) => ["customers", "detail", uuid] as const,
  vehicles: (uuid: string) => ["customers", "vehicles", uuid] as const,
  dealers: ["customers", "dealers"] as const,
};
