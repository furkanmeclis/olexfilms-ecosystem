import { describe, expect, it } from "vitest";

import type { PhotoAngle } from "@/features/photo-standard/services/photo-standard.service";
import { ApiError } from "@/lib/api/errors";

import {
  MAX_PHOTO_BYTES,
  canSeeIntakeLocation,
  mapLink,
  missingAnglesFromError,
  overrideRows,
  overridesBody,
  reorderPatches,
  resetOverride,
  setOverride,
  validatePhotoFile,
} from "./photo-standard";

const file = (size: number, type = "image/jpeg") =>
  ({ size, type, name: "x.jpg" }) as File;

const angle = (key: string, over: Partial<PhotoAngle> = {}): PhotoAngle => ({
  uuid: `a-${key}`,
  key,
  name: { tr: key },
  hint: {},
  required: true,
  sort_order: 10,
  active: true,
  ...over,
});

describe("validatePhotoFile", () => {
  it("refuses files over 12 MB and accepts exactly 12 MB", () => {
    expect(validatePhotoFile(file(MAX_PHOTO_BYTES + 1))).toBe("too_large");
    expect(validatePhotoFile(file(MAX_PHOTO_BYTES))).toBeNull();
  });

  it("refuses non-images but accepts a typeless (iOS HEIC) file", () => {
    expect(validatePhotoFile(file(10, "application/pdf"))).toBe("not_image");
    expect(validatePhotoFile(file(10, ""))).toBeNull();
    expect(validatePhotoFile(file(10, "image/heic"))).toBeNull();
  });
});

describe("missingAnglesFromError", () => {
  it("reads the intake_photos.<key> details of a 422", () => {
    const error = new ApiError({
      status: 422,
      code: "PHOTO_STANDARD_INCOMPLETE",
      message: "m",
      details: [
        { field: "intake_photos.front", code: "missing" },
        { field: "intake_photos.rear", code: "missing" },
      ],
    });
    expect(missingAnglesFromError(error)).toEqual(["front", "rear"]);
  });

  it("falls back to data.missing_angles and ignores other errors", () => {
    const error = new ApiError({
      status: 422,
      code: "PHOTO_STANDARD_INCOMPLETE",
      message: "m",
      body: {
        success: false,
        error: { code: "PHOTO_STANDARD_INCOMPLETE", message: "m" },
        data: { missing_angles: ["left"] },
      } as never,
    });
    expect(missingAnglesFromError(error)).toEqual(["left"]);
    expect(
      missingAnglesFromError(
        new ApiError({ status: 422, code: "CONTRACT_REQUIRED", message: "m" }),
      ),
    ).toBeNull();
    expect(missingAnglesFromError(new Error("x"))).toBeNull();
  });
});

describe("canSeeIntakeLocation (KVKK)", () => {
  const base = {
    isSuperAdmin: false,
    orgType: "dealer",
    orgUuid: "d1",
    orgRoles: ["dealer_staff"],
    serviceOrgUuid: "d1",
  };

  it("hides the location from dealer staff", () => {
    expect(canSeeIntakeLocation(base)).toBe(false);
  });

  it("shows it to the service dealer's owner, the center and super admins", () => {
    expect(canSeeIntakeLocation({ ...base, orgRoles: ["dealer_owner"] })).toBe(
      true,
    );
    expect(
      canSeeIntakeLocation({
        ...base,
        orgRoles: ["dealer_owner"],
        serviceOrgUuid: "other",
      }),
    ).toBe(false);
    expect(canSeeIntakeLocation({ ...base, orgType: "center" })).toBe(true);
    expect(canSeeIntakeLocation({ ...base, isSuperAdmin: true })).toBe(true);
    expect(
      canSeeIntakeLocation({
        ...base,
        orgType: "distributor",
        orgRoles: ["distributor_owner"],
      }),
    ).toBe(false);
  });

  it("builds a map link only for a valid position", () => {
    expect(mapLink("41.0", "29.0")).toContain("mlat=41&mlon=29");
    expect(mapLink(undefined, "29")).toBeNull();
    expect(mapLink("x", "29")).toBeNull();
  });
});

describe("override rows", () => {
  it("shows the center default for untouched angles and sends only overrides", () => {
    const rows = overrideRows([
      angle("front", {
        required: true,
        default_required: true,
        default_hidden: false,
        overridden: false,
      }),
      angle("rear", {
        required: false,
        hidden: true,
        default_required: true,
        default_hidden: false,
        overridden: true,
      }),
    ]);
    expect(rows[0]).toMatchObject({ overridden: false, required: true });
    expect(rows[1]).toMatchObject({ overridden: true, hidden: true });
    expect(overridesBody("o1", rows)).toEqual({
      organization_uuid: "o1",
      overrides: [{ angle_key: "rear", required: false, hidden: true }],
    });
  });

  it("hiding clears required; reset returns to the center default", () => {
    const [row] = overrideRows([
      angle("front", { default_required: true, overridden: false }),
    ]);
    const hidden = setOverride(row, "hidden", true);
    expect(hidden).toMatchObject({
      overridden: true,
      hidden: true,
      required: false,
    });
    expect(resetOverride(hidden)).toMatchObject({
      overridden: false,
      hidden: false,
      required: true,
    });
  });
});

describe("reorderPatches", () => {
  it("renumbers moved angles by 10", () => {
    const a = angle("a", { sort_order: 10 });
    const b = angle("b", { sort_order: 20 });
    expect(
      reorderPatches([b, a]).map((p) => [p.angle.key, p.sort_order]),
    ).toEqual([
      ["b", 10],
      ["a", 20],
    ]);
  });
});
