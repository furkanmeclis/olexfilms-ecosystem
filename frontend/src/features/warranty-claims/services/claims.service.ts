import type { ServerListQuery } from "@/components/entity";
import { apiConfig } from "@/config/api";
import type {
  WarrantyClaim,
  WarrantyClaimCreateInput,
  WarrantyClaimPhoto,
  WarrantyClaimStatus,
} from "@/features/warranty-claims/lib/claims";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

export type WarrantyClaimPage = {
  items: WarrantyClaim[];
  total: number;
  limit: number;
  offset: number;
};

/**
 * GET /v1/warranty-claims params (TEC-377): one sort field (`-created_at`
 * default, `updated_at`, `claim_no`, `status`, `decided_at`), `q`, CSV
 * `status` / `organization_uuid`, `warranty_uuid`, `service_uuid`,
 * `vehicle_uuid` and the `created_from` / `created_to` days.
 */
export type WarrantyClaimServerQuery = ServerListQuery;

const enc = encodeURIComponent;

/** BFF URL of a claim photo (GET …/photos/{photo}, read scope). */
export function claimPhotoUrl(claimUuid: string, photoUuid: string) {
  return `${apiConfig.baseUrl.replace(/\/$/, "")}/v1/warranty-claims/${enc(claimUuid)}/photos/${enc(photoUuid)}`;
}

/**
 * Panel warranty claim API (TEC-335/336/339) through the BFF. Reads follow
 * the warranty_claims.read scope; the status change is checked again by
 * the backend (write / review / decide).
 */
export const claimsService = {
  list(params: WarrantyClaimServerQuery) {
    return platformRequest<WarrantyClaimPage>("GET", "/v1/warranty-claims", {
      query: params,
    });
  },
  get(uuid: string) {
    return platformRequest<WarrantyClaim>(
      "GET",
      `/v1/warranty-claims/${enc(uuid)}`,
    );
  },
  create(body: WarrantyClaimCreateInput) {
    return platformRequest<WarrantyClaim>("POST", "/v1/warranty-claims", {
      body,
    });
  },
  addPhoto(uuid: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return platformFormRequest<WarrantyClaimPhoto>(
      "POST",
      `/v1/warranty-claims/${enc(uuid)}/photos`,
      form,
    );
  },
  transition(
    uuid: string,
    status: WarrantyClaimStatus,
    rejectionReason?: string,
  ) {
    return platformRequest<WarrantyClaim>(
      "POST",
      `/v1/warranty-claims/${enc(uuid)}/status`,
      {
        body: rejectionReason
          ? { status, rejection_reason: rejectionReason }
          : { status },
      },
    );
  },
};

export const claimKeys = {
  all: ["warranty-claims"] as const,
  list: (params: WarrantyClaimServerQuery) =>
    ["warranty-claims", "list", params] as const,
  detail: (uuid: string) => ["warranty-claims", "detail", uuid] as const,
  byWarranty: (warrantyUuid: string) =>
    ["warranty-claims", "by-warranty", warrantyUuid] as const,
  portal: ["portal", "warranty-claims"] as const,
};
