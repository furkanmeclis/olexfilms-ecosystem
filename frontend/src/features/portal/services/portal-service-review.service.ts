import { portalRequest } from "@/features/portal/lib/portal-client";
import type { components } from "@/generated/api";

/** TEC-244: review state of a portal service (review, can_review, Google link). */
export type PortalServiceReviewState =
  components["schemas"]["PortalServiceReview"];
export type PortalServiceReviewInput =
  components["schemas"]["PortalServiceReviewInput"];

/** Longest review comment the API accepts (characters). */
export const REVIEW_COMMENT_MAX = 2000;

const enc = encodeURIComponent;

/** GET / POST /v1/portal/services/{uuid}/review through the portal BFF. */
export const portalServiceReviewApi = {
  get(serviceUuid: string) {
    return portalRequest<PortalServiceReviewState>(
      `portal/services/${enc(serviceUuid)}/review`,
    );
  },
  create(serviceUuid: string, body: PortalServiceReviewInput) {
    return portalRequest<PortalServiceReviewState>(
      `portal/services/${enc(serviceUuid)}/review`,
      { method: "POST", body },
    );
  },
};

/** Only an https link is rendered as the Google review button. */
export function googleReviewUrl(url: string | null | undefined): string | null {
  const u = url?.trim();
  return u && u.startsWith("https://") ? u : null;
}
