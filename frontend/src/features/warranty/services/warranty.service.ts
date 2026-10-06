import type { ServerListQuery } from "@/components/entity";
import type { Warranty } from "@/features/warranty/lib/warranty-list";
import { platformRequest } from "@/lib/api/platform-request";

export type WarrantyPage = {
  items: Warranty[];
  total: number;
  limit: number;
  offset: number;
};

/**
 * GET /v1/warranties params (TEC-191, TEC-377): one sort field (`expiry`
 * default, `end_at`, `start_at`, `created_at`, `public_code`, `service_no`,
 * `status`, `product`, `organization`), `q`, CSV `status` /
 * `organization_uuid`, `product_uuid`, `days_left_min` / `_max` and the
 * `start_*` / `end_*` day windows.
 */
export type WarrantyServerQuery = ServerListQuery;

/** List export (TEC-377): same filters, search and sort as the list. */
export const WARRANTIES_EXPORT_PATH = "/v1/warranties/export";

const enc = encodeURIComponent;

/**
 * Panel warranty API (TEC-191) through the BFF: the list and detail follow
 * the warranties.read scope of the active organization; the void needs
 * warranties.void and a step-up (platformRequest retries after the step-up
 * dialog).
 */
export const warrantyService = {
  list(params: WarrantyServerQuery) {
    return platformRequest<WarrantyPage>("GET", "/v1/warranties", {
      query: params,
    });
  },
  get(uuid: string) {
    return platformRequest<Warranty>("GET", `/v1/warranties/${enc(uuid)}`);
  },
  void(uuid: string, reason: string) {
    return platformRequest<Warranty>(
      "POST",
      `/v1/warranties/${enc(uuid)}/void`,
      { body: { reason } },
    );
  },
};

export const warrantyKeys = {
  all: ["warranties"] as const,
  list: (params: WarrantyServerQuery) =>
    ["warranties", "list", params] as const,
  detail: (uuid: string) => ["warranties", "detail", uuid] as const,
};
