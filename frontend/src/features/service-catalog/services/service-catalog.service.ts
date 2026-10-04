import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type ServiceCatalogCategory = Schemas["ServiceCatalogCategory"];
export type ServiceCatalogRecurrence = Schemas["ServiceCatalogRecurrence"];
export type ServiceCatalogItem = Schemas["ServiceCatalogItem"];
export type ServiceCatalogItemInput = Schemas["ServiceCatalogItemInput"];
export type ServiceCatalogItemPatch = Schemas["ServiceCatalogItemPatch"];
export type ServiceCatalogOverride = Schemas["ServiceCatalogOverride"];
export type ServiceCatalogOverrideInput =
  Schemas["ServiceCatalogOverrideInput"];
export type PlatformModule = Schemas["PlatformModule"];
export type ContractTemplate = Schemas["ContractTemplate"] & {
  id?: number;
};

export type DistributorOption = { uuid: string; name: string };

const enc = encodeURIComponent;

export const serviceCatalogKeys = {
  all: ["service-catalog"] as const,
  platform: () => [...serviceCatalogKeys.all, "platform"] as const,
  visible: () => [...serviceCatalogKeys.all, "visible"] as const,
  templates: () => [...serviceCatalogKeys.all, "contract-templates"] as const,
  distributors: () => [...serviceCatalogKeys.all, "distributors"] as const,
};

export const SERVICE_CATALOG_CATEGORIES = [
  "advertising",
  "training",
  "setup",
  "software",
  "module_bundle",
  "other",
] as const satisfies readonly ServiceCatalogCategory[];

export const SERVICE_CATALOG_RECURRENCES = [
  "one_time",
  "monthly",
  "yearly",
] as const satisfies readonly ServiceCatalogRecurrence[];

export const serviceCatalogService = {
  listPlatform() {
    return platformRequest<{ items: ServiceCatalogItem[] }>(
      "GET",
      "/v1/platform/service-catalog",
    );
  },

  listVisible() {
    return platformRequest<{ items: ServiceCatalogItem[] }>(
      "GET",
      "/v1/service-catalog",
    );
  },

  create(body: ServiceCatalogItemInput) {
    return platformRequest<ServiceCatalogItem>(
      "POST",
      "/v1/platform/service-catalog",
      { body },
    );
  },

  patch(uuid: string, body: ServiceCatalogItemPatch) {
    return platformRequest<ServiceCatalogItem>(
      "PATCH",
      `/v1/platform/service-catalog/${enc(uuid)}`,
      { body },
    );
  },

  delete(uuid: string) {
    return platformRequest<ServiceCatalogItem>(
      "DELETE",
      `/v1/platform/service-catalog/${enc(uuid)}`,
    );
  },

  setModules(uuid: string, modules: string[]) {
    return platformRequest<ServiceCatalogItem>(
      "PUT",
      `/v1/platform/service-catalog/${enc(uuid)}/modules`,
      { body: { modules } },
    );
  },

  getOverride(uuid: string, orgUuid: string) {
    return platformRequest<ServiceCatalogOverride>(
      "GET",
      `/v1/platform/service-catalog/${enc(uuid)}/overrides/${enc(orgUuid)}`,
    );
  },

  putOverride(
    uuid: string,
    orgUuid: string,
    body: ServiceCatalogOverrideInput,
  ) {
    return platformRequest<ServiceCatalogOverride>(
      "PUT",
      `/v1/platform/service-catalog/${enc(uuid)}/overrides/${enc(orgUuid)}`,
      { body },
    );
  },

  deleteOverride(uuid: string, orgUuid: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/platform/service-catalog/${enc(uuid)}/overrides/${enc(orgUuid)}`,
    );
  },

  listContractTemplates() {
    return platformRequest<{ items: ContractTemplate[] }>(
      "GET",
      "/v1/platform/contract-templates",
      { query: { kind: "service_sale" } },
    );
  },

  async listDistributors(): Promise<DistributorOption[]> {
    const data = await platformRequest<{
      items: { uuid: string; name: string }[];
    }>("GET", "/v1/tenant/organizations", {
      query: { type: "distributor", limit: 200 },
    });
    return (data.items ?? []).map((o) => ({ uuid: o.uuid, name: o.name }));
  },
};
