import type {
  Warranty,
  WarrantyListQuery,
} from "@/features/warranty/lib/warranty-list";
import { platformRequest } from "@/lib/api/platform-request";

export type WarrantyPage = {
  items: Warranty[];
  total: number;
  limit: number;
  offset: number;
};

const enc = encodeURIComponent;

/**
 * Panel warranty API (TEC-191) through the BFF: the list and detail follow
 * the warranties.read scope of the active organization; the void needs
 * warranties.void and a step-up (platformRequest retries after the step-up
 * dialog).
 */
export const warrantyService = {
  list(params: WarrantyListQuery) {
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
  list: (params: WarrantyListQuery) => ["warranties", "list", params] as const,
  detail: (uuid: string) => ["warranties", "detail", uuid] as const,
};
