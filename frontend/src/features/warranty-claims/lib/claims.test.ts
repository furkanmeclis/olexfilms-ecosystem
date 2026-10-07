import { describe, expect, it } from "vitest";

import {
  CLAIM_PHOTO_MAX_BYTES,
  canSubmitClaim,
  checkClaimPhoto,
  claimActions,
  isLiveClaim,
  latestClaimByWarranty,
  type PortalClaimStatus,
} from "./claims";

describe("canSubmitClaim", () => {
  it("needs parts, a description and at least one photo", () => {
    const base = { parts: ["body_kaput"], description: "Bubbles", photos: 1 };
    expect(canSubmitClaim(base)).toBe(true);
    expect(canSubmitClaim({ ...base, photos: 0 })).toBe(false);
    expect(canSubmitClaim({ ...base, parts: [] })).toBe(false);
    expect(canSubmitClaim({ ...base, description: "   " })).toBe(false);
  });
});

describe("checkClaimPhoto", () => {
  it("accepts JPEG / PNG / WebP up to 12 MB", () => {
    expect(checkClaimPhoto({ size: 1000, type: "image/jpeg" })).toBe("ok");
    expect(
      checkClaimPhoto({ size: CLAIM_PHOTO_MAX_BYTES, type: "image/webp" }),
    ).toBe("ok");
    expect(
      checkClaimPhoto({ size: CLAIM_PHOTO_MAX_BYTES + 1, type: "image/png" }),
    ).toBe("too_large");
    expect(checkClaimPhoto({ size: 10, type: "application/pdf" })).toBe(
      "bad_type",
    );
  });
});

describe("claimActions", () => {
  const all = { write: true, review: true, decide: true };

  it("never offers approve / reject to a dealer", () => {
    const dealer = { write: true, review: false, decide: false };
    for (const status of ["open", "dealer_review", "center_review"] as const) {
      expect(claimActions(status, dealer, "dealer").decide).toBe(false);
    }
    expect(claimActions("open", dealer, "dealer").submitReview).toBe(true);
    expect(claimActions("dealer_review", dealer, "dealer").forward).toBe(false);
  });

  it("lets the distributor forward and the center decide", () => {
    expect(claimActions("dealer_review", all, "distributor").forward).toBe(
      true,
    );
    expect(claimActions("open", all, "distributor").forward).toBe(true);
    expect(claimActions("open", all, "center").forward).toBe(false);
    expect(claimActions("center_review", all, "center").decide).toBe(true);
    expect(claimActions("approved", all, "center")).toEqual({
      submitReview: false,
      forward: false,
      decide: false,
    });
  });
});

describe("isLiveClaim", () => {
  it("treats rejected and closed claims as finished", () => {
    expect(isLiveClaim("open")).toBe(true);
    expect(isLiveClaim("approved")).toBe(true);
    expect(isLiveClaim("rejected")).toBe(false);
    expect(isLiveClaim("closed")).toBe(false);
  });
});

describe("latestClaimByWarranty", () => {
  it("keeps the newest claim per warranty", () => {
    const items: PortalClaimStatus[] = [
      {
        uuid: "c1",
        warranty_uuid: "w1",
        status: "rejected",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-02T00:00:00Z",
      },
      {
        uuid: "c2",
        warranty_uuid: "w1",
        status: "open",
        created_at: "2026-02-01T00:00:00Z",
        updated_at: "2026-02-01T00:00:00Z",
      },
      {
        uuid: "c3",
        status: "open",
        created_at: "2026-02-01T00:00:00Z",
        updated_at: "2026-02-01T00:00:00Z",
      },
    ];
    const map = latestClaimByWarranty(items);
    expect(map.size).toBe(1);
    expect(map.get("w1")?.uuid).toBe("c2");
  });
});
