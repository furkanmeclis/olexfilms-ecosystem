import type { components, operations } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type GlorianConnection = components["schemas"]["GlorianConnection"];
export type GlorianConnectionInput =
  components["schemas"]["PutGlorianConnectionRequest"];
export type GlorianTestResult = components["schemas"]["GlorianTestResult"];
export type GlorianPackageInput =
  components["schemas"]["CreateGlorianConnectionPackageRequest"];
export type GlorianConnectionPackage =
  components["schemas"]["GlorianConnectionPackage"];
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
export type GlorianOutboundFilter = NonNullable<
  operations["listGlorianOutbounds"]["parameters"]["query"]
>;
export type GlorianReplaySkip = components["schemas"]["GlorianReplaySkip"];
export type GlorianLocationMap = components["schemas"]["GlorianLocationMap"];
export type GlorianLocationMapInput =
  components["schemas"]["GlorianLocationMapInput"];
export type GlorianMapWarehouse = components["schemas"]["GlorianMapWarehouse"];
export type GlorianMapLocation = components["schemas"]["GlorianMapLocation"];
export type GlorianLocationMapFilter = NonNullable<
  operations["listGlorianLocationMaps"]["parameters"]["query"]
>;

export type GlorianDealer = components["schemas"]["GlorianDealer"];
export type GlorianDealerMatchType = GlorianDealer["match_type"];
export type GlorianDealerOrganization =
  components["schemas"]["GlorianDealerOrganization"];
export type GlorianDealerFilter = NonNullable<
  operations["listGlorianDealers"]["parameters"]["query"]
>;

/** List envelope of the sync runs / outbounds (TEC-367). */
export type GlorianPage<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

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

  /** TEC-1265: new API key sealed in an encrypted connection package. */
  async createPackage(body: GlorianPackageInput) {
    return unwrap<GlorianConnectionPackage>(
      await apiClient.POST(
        "/v1/platform/integrations/glorian/connection-package",
        { body },
      ),
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

  /** One page of sync runs (sort, CSV kind/status, started range). */
  async syncRunsPage(query: GlorianSyncRunFilter = {}) {
    return unwrap<GlorianPage<GlorianSyncRun>>(
      await apiClient.GET("/v1/platform/integrations/glorian/sync-runs", {
        params: { query },
      }),
      SILENT,
    );
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

  /** One page of outbounds (no `state` = every state; CSV, q, sort). */
  async outboundsPage(query: GlorianOutboundFilter = {}) {
    return unwrap<GlorianPage<GlorianOutbound>>(
      await apiClient.GET("/v1/platform/integrations/glorian/outbounds", {
        params: { query },
      }),
      SILENT,
    );
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

  /** Replay several held / failed outbounds (1..100 uuids). */
  async replayOutbounds(uuids: string[]) {
    return unwrap<{
      queued: GlorianOutbound[];
      skipped: GlorianReplaySkip[];
    }>(
      await apiClient.POST(
        "/v1/platform/integrations/glorian/outbounds/replay",
        { body: { uuids } },
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

  /** One page of remote location maps (q, sort, paging; TEC-769). */
  async locationMapsPage(query: GlorianLocationMapFilter = {}) {
    return unwrap<GlorianPage<GlorianLocationMap>>(
      await apiClient.GET("/v1/platform/integrations/glorian/location-maps", {
        params: { query },
      }),
      SILENT,
    );
  },

  async createLocationMap(body: GlorianLocationMapInput) {
    return unwrap<GlorianLocationMap>(
      await apiClient.POST("/v1/platform/integrations/glorian/location-maps", {
        body,
      }),
      SILENT,
    );
  },

  async updateLocationMap(uuid: string, body: GlorianLocationMapInput) {
    return unwrap<GlorianLocationMap>(
      await apiClient.PUT(
        "/v1/platform/integrations/glorian/location-maps/{uuid}",
        { params: { path: { uuid } }, body },
      ),
      SILENT,
    );
  },

  async deleteLocationMap(uuid: string) {
    await unwrap<unknown>(
      await apiClient.DELETE(
        "/v1/platform/integrations/glorian/location-maps/{uuid}",
        { params: { path: { uuid } } },
      ),
      SILENT,
    );
  },

  /** Warehouses of the connection's organization (map targets). */
  async mapWarehouses() {
    const res = await unwrap<{ items: GlorianMapWarehouse[] }>(
      await apiClient.GET(
        "/v1/platform/integrations/glorian/location-maps/warehouses",
      ),
      SILENT,
    );
    return res.items;
  },

  /** Up to 50 active bins of a warehouse, searched by full code / name. */
  async mapLocations(warehouseUuid: string, q?: string) {
    const res = await unwrap<{ items: GlorianMapLocation[] }>(
      await apiClient.GET(
        "/v1/platform/integrations/glorian/location-maps/warehouses/{uuid}/locations",
        {
          params: {
            path: { uuid: warehouseUuid },
            query: q ? { q } : {},
          },
        },
      ),
      SILENT,
    );
    return res.items;
  },

  /** One page of hub dealers with their organization match (TEC-1265). */
  async dealersPage(query: GlorianDealerFilter = {}) {
    return unwrap<GlorianPage<GlorianDealer>>(
      await apiClient.GET("/v1/platform/integrations/glorian/dealers", {
        params: { query },
      }),
      SILENT,
    );
  },

  /** Up to 50 organizations of the brand (dealer link picker). */
  async dealerOrganizations(q?: string) {
    const res = await unwrap<{ items: GlorianDealerOrganization[] }>(
      await apiClient.GET(
        "/v1/platform/integrations/glorian/dealers/organizations",
        { params: { query: q ? { q } : {} } },
      ),
      SILENT,
    );
    return res.items;
  },

  /** Manual link of a dealer (null clears it). */
  async linkDealer(uuid: string, organizationUuid: string | null) {
    return unwrap<GlorianDealer>(
      await apiClient.PUT(
        "/v1/platform/integrations/glorian/dealers/{uuid}/link",
        {
          params: { path: { uuid } },
          body: { organization_uuid: organizationUuid },
        },
      ),
      SILENT,
    );
  },

  /** An organization's Glorian dealer code (null clears it). */
  async setDealerCode(organizationUuid: string, code: string | null) {
    return unwrap<GlorianDealerOrganization>(
      await apiClient.PUT(
        "/v1/platform/integrations/glorian/organizations/{uuid}/dealer-code",
        {
          params: { path: { uuid: organizationUuid } },
          body: { glorian_dealer_code: code },
        },
      ),
      SILENT,
    );
  },
};
