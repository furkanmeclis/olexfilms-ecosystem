import type { components, operations } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type GlorianConnection = components["schemas"]["GlorianConnection"];
export type GlorianConnectionInput =
  components["schemas"]["PutGlorianConnectionRequest"];
export type GlorianTestResult = components["schemas"]["GlorianTestResult"];
export type GlorianSyncRun = components["schemas"]["GlorianSyncRun"];
export type GlorianSyncRunKind = GlorianSyncRun["kind"];
export type GlorianSyncRunStatus = GlorianSyncRun["status"];
export type GlorianSyncKind =
  components["schemas"]["TriggerGlorianSyncRequest"]["kind"];
export type GlorianSyncQueued = components["schemas"]["GlorianSyncQueued"];
export type GlorianOutbound = components["schemas"]["GlorianOutbound"];
export type GlorianOutboundState = GlorianOutbound["state"];

export type GlorianSyncRunFilter = NonNullable<
  operations["listGlorianSyncRuns"]["parameters"]["query"]
>;

/**
 * Errors are not toasted globally: the page maps the Glorian codes to its
 * own texts and shows an unconfigured connection (404) in place.
 */
const SILENT = { silent: true } as const;

/** Glorian hub admin API (TEC-273) behind the BFF. */
export const glorianService = {
  async get() {
    return unwrap<GlorianConnection>(
      await apiClient.GET("/v1/platform/integrations/glorian"),
    );
  },

  async save(body: GlorianConnectionInput) {
    return unwrap<GlorianConnection>(
      await apiClient.PUT("/v1/platform/integrations/glorian", { body }),
      SILENT,
    );
  },

  async test() {
    return unwrap<GlorianTestResult>(
      await apiClient.POST("/v1/platform/integrations/glorian/test"),
      SILENT,
    );
  },

  async listSyncRuns(query: GlorianSyncRunFilter = {}) {
    const res = await unwrap<{ items: GlorianSyncRun[] }>(
      await apiClient.GET("/v1/platform/integrations/glorian/sync-runs", {
        params: { query },
      }),
      SILENT,
    );
    return res.items;
  },

  async getSyncRun(uuid: string) {
    return unwrap<GlorianSyncRun>(
      await apiClient.GET(
        "/v1/platform/integrations/glorian/sync-runs/{uuid}",
        { params: { path: { uuid } } },
      ),
      SILENT,
    );
  },

  async triggerSync(kind: GlorianSyncKind) {
    return unwrap<GlorianSyncQueued>(
      await apiClient.POST("/v1/platform/integrations/glorian/sync-runs", {
        body: { kind },
      }),
      SILENT,
    );
  },

  async listOutbounds(state: GlorianOutboundState = "held") {
    const res = await unwrap<{ items: GlorianOutbound[] }>(
      await apiClient.GET("/v1/platform/integrations/glorian/outbounds", {
        params: { query: { state } },
      }),
      SILENT,
    );
    return res.items;
  },

  async replayOutbound(uuid: string) {
    return unwrap<GlorianOutbound>(
      await apiClient.POST(
        "/v1/platform/integrations/glorian/outbounds/{uuid}/replay",
        { params: { path: { uuid } } },
      ),
      SILENT,
    );
  },

  async reconcile() {
    return unwrap<GlorianSyncRun>(
      await apiClient.POST("/v1/platform/integrations/glorian/reconcile"),
      SILENT,
    );
  },
};
