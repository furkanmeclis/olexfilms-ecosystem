import type {
  DealerModuleRow,
  DealerStandardEntry,
  FeatureList,
  ModuleState,
  PlatformModule,
  PlatformModulePatch,
} from "@/features/modules/types";
import { platformRequest } from "@/lib/api/platform-request";

const enc = encodeURIComponent;

export const modulesService = {
  /** Modules of the active organization (menus, guards, Özellikler page). */
  list() {
    return platformRequest<FeatureList>("GET", "/v1/features");
  },

  request(key: string, note?: string) {
    return platformRequest<{ status: string; recipients: number }>(
      "POST",
      `/v1/features/${enc(key)}/request`,
      { body: note ? { note } : {} },
    );
  },

  dealers() {
    return platformRequest<{ items: DealerModuleRow[] }>(
      "GET",
      "/v1/tenant/modules/dealers",
    );
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
