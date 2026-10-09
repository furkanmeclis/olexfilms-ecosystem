import type {
  DealerModuleRow,
  DealerStandardEntry,
  FeatureList,
  ModuleRequestRow,
  ModuleRequestSummary,
  ModuleState,
  PlatformModule,
  PlatformModulePatch,
} from "@/features/modules/types";
import { platformRequest } from "@/lib/api/platform-request";

const enc = encodeURIComponent;

export type ListDealerModulesParams = {
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
  module?: string;
  /** CSV of enabled, disabled */
  state?: string;
  /** CSV of module sources */
  source?: string;
};

/** TEC-508 request queue list contract (tenant and platform). */
export type ListModuleRequestsParams = {
  limit?: number;
  offset?: number;
  sort?: string;
  q?: string;
  /** CSV of pending, approved, rejected, cancelled */
  status?: string;
  /** CSV of module keys */
  module_key?: string;
  created_from?: string;
  created_to?: string;
};

/** Whose queue: the distributor's dealers or the center's. */
export type ModuleRequestScope = "tenant" | "platform";

type ModuleRequestPage = {
  items: ModuleRequestRow[];
  total: number;
  limit: number;
  offset: number;
};

const requestQueueBase: Record<ModuleRequestScope, string> = {
  tenant: "/v1/tenant/modules/requests",
  platform: "/v1/platform/modules/requests",
};

export const modulesService = {
  /** Modules of the active organization (menus, guards, Özellikler page). */
  list() {
    return platformRequest<FeatureList>("GET", "/v1/features");
  },

  request(key: string, note?: string) {
    return platformRequest<{
      status: string;
      recipients: number;
      request: ModuleRequestSummary;
    }>("POST", `/v1/features/${enc(key)}/request`, {
      body: note ? { note } : {},
    });
  },

  /** Withdraws the organization's pending request of a module. */
  cancelRequest(key: string) {
    return platformRequest<ModuleRequestSummary>(
      "DELETE",
      `/v1/features/${enc(key)}/request`,
    );
  },

  /** Decision queue: the distributor's dealers or the center's requests. */
  requests(scope: ModuleRequestScope, params: ListModuleRequestsParams = {}) {
    return platformRequest<ModuleRequestPage>("GET", requestQueueBase[scope], {
      query: params,
    });
  },

  decideRequest(
    scope: ModuleRequestScope,
    uuid: string,
    decision: "approve" | "reject",
    note?: string,
  ) {
    return platformRequest<ModuleRequestSummary>(
      "POST",
      `${requestQueueBase[scope]}/${enc(uuid)}/${decision}`,
      { body: note ? { note } : {} },
    );
  },

  /**
   * Dealer module matrix. With `limit`/`offset` the result is a page
   * (`total` counts every match); `state` / `source` (CSV) need `module`.
   */
  dealers(params: ListDealerModulesParams = {}) {
    return platformRequest<{
      items: DealerModuleRow[];
      total: number;
      limit: number;
      offset: number;
    }>("GET", "/v1/tenant/modules/dealers", { query: params });
  },

  /** enabled null clears the dealers' own values (back to the standard). */
  bulk(dealerUuids: string[], key: string, enabled: boolean | null) {
    return platformRequest<{ updated: number }>(
      "POST",
      "/v1/tenant/modules/dealers/bulk",
      { body: { dealer_uuids: dealerUuids, key, enabled } },
    );
  },

  dealerStandard() {
    return platformRequest<{ items: DealerStandardEntry[] }>(
      "GET",
      "/v1/tenant/modules/dealer-standard",
    );
  },

  setDealerStandard(key: string, enabled: boolean | null) {
    const path = `/v1/tenant/modules/dealer-standard/${enc(key)}`;
    return enabled === null
      ? platformRequest<{ items: DealerStandardEntry[] }>("DELETE", path)
      : platformRequest<{ items: DealerStandardEntry[] }>("PUT", path, {
          body: { enabled },
        });
  },

  platformList() {
    return platformRequest<{ items: PlatformModule[] }>(
      "GET",
      "/v1/platform/modules",
    );
  },

  platformPatch(key: string, body: PlatformModulePatch) {
    return platformRequest<PlatformModule>(
      "PATCH",
      `/v1/platform/modules/${enc(key)}`,
      { body },
    );
  },

  platformOrganization(uuid: string) {
    return platformRequest<{ organization_type: string; items: ModuleState[] }>(
      "GET",
      `/v1/platform/organizations/${enc(uuid)}/modules`,
    );
  },

  platformSetOrganization(uuid: string, key: string, enabled: boolean | null) {
    const path = `/v1/platform/organizations/${enc(uuid)}/modules/${enc(key)}`;
    return enabled === null
      ? platformRequest<ModuleState>("DELETE", path)
      : platformRequest<ModuleState>("PUT", path, { body: { enabled } });
  },
};
