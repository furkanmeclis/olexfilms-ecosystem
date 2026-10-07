import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type WarrantyClaim = Schemas["WarrantyClaim"];
export type WarrantyClaimStatus = Schemas["WarrantyClaimStatus"];
export type WarrantyClaimEvent = Schemas["WarrantyClaimEvent"];
export type WarrantyClaimPhoto = Schemas["WarrantyClaimPhoto"];
export type WarrantyClaimCreateInput = Schemas["WarrantyClaimCreateInput"];
export type CoverageReason =
  Schemas["WarrantyClaimCoverageCheck"]["reasons"][number];
export type PortalClaimStatus = Schemas["PortalWarrantyClaimStatus"];

/** Flow order (open → closed), same as the backend status sort. */
export const CLAIM_STATUSES: readonly WarrantyClaimStatus[] = [
  "open",
  "dealer_review",
  "center_review",
  "approved",
  "rejected",
  "reapplied",
  "closed",
];

/** A claim that still blocks a new one on the same warranty. */
export function isLiveClaim(status: WarrantyClaimStatus): boolean {
  return status !== "rejected" && status !== "closed";
}

export function claimStatusTone(
  status: WarrantyClaimStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "approved":
    case "reapplied":
      return "success";
    case "rejected":
      return "danger";
    case "dealer_review":
    case "center_review":
      return "warning";
    default:
      return "default";
  }
}

/** Photo limits of POST /v1/warranty-claims/{uuid}/photos. */
export const CLAIM_PHOTO_MAX_BYTES = 12 * 1024 * 1024;
export const CLAIM_PHOTO_TYPES = ["image/jpeg", "image/png", "image/webp"];

export type PhotoCheck = "ok" | "too_large" | "bad_type";

export function checkClaimPhoto(file: { size: number; type: string }) {
  if (!CLAIM_PHOTO_TYPES.includes(file.type)) return "bad_type" as const;
  if (file.size > CLAIM_PHOTO_MAX_BYTES) return "too_large" as const;
  return "ok" as const;
}

/**
 * "Submit for review" needs at least one part, a description and at least
 * one photo (the backend rejects dealer_review without photos, 422
 * CLAIM_PHOTO_REQUIRED).
 */
export function canSubmitClaim(input: {
  parts: readonly string[];
  description: string;
  photos: number;
}): boolean {
  return (
    input.parts.length > 0 &&
    input.description.trim() !== "" &&
    input.photos > 0
  );
}

export type ClaimGrants = {
  write: boolean;
  review: boolean;
  decide: boolean;
};

export type ClaimActions = {
  /** open → dealer_review (dealer / distributor that owns the claim). */
  submitReview: boolean;
  /** dealer_review (or a distributor's own open claim) → center_review. */
  forward: boolean;
  /** center_review → approved / rejected (center only). */
  decide: boolean;
};

/**
 * Actions the caller may take on the claim. Mirrors the backend flow:
 * dealer_review needs warranty_claims.write, forwarding to the center
 * warranty_claims.review (distributor), the decision
 * warranty_claims.decide (center). Dealers never get approve / reject.
 */
export function claimActions(
  status: WarrantyClaimStatus,
  grants: ClaimGrants,
  orgType: string | undefined,
): ClaimActions {
  return {
    submitReview: status === "open" && grants.write,
    forward:
      grants.review &&
      (status === "dealer_review" ||
        (status === "open" && orgType === "distributor")),
    decide: status === "center_review" && grants.decide,
  };
}

/** Latest portal claim status per warranty uuid (newest created first). */
export function latestClaimByWarranty(
  items: readonly PortalClaimStatus[],
): Map<string, PortalClaimStatus> {
  const out = new Map<string, PortalClaimStatus>();
  for (const item of items) {
    if (!item.warranty_uuid) continue;
    const prev = out.get(item.warranty_uuid);
    if (!prev || item.created_at > prev.created_at) {
      out.set(item.warranty_uuid, item);
    }
  }
  return out;
}
