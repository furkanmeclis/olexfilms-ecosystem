import { describe, expect, it } from "vitest";

import {
  brandLogoUrl,
  logoVersion,
  vehicleImageProblem,
  VEHICLE_IMAGE_LIMITS,
} from "./images";

const UUID = "0b5c4a39-6a43-4d47-9a3f-1f3a2b4c5d6e";

describe("vehicleImageProblem", () => {
  it("refuses SVG by type or extension", () => {
    expect(
      vehicleImageProblem(
        { name: "bmw.svg", type: "image/svg+xml", size: 100 },
        "logo",
      ),
    ).toBe("svg");
    expect(
      vehicleImageProblem({ name: "BMW.SVG", type: "", size: 100 }, "logo"),
    ).toBe("svg");
  });

  it("accepts JPEG, PNG and WebP within the limit", () => {
    for (const type of ["image/jpeg", "image/png", "image/webp"]) {
      expect(
        vehicleImageProblem({ name: "x", type, size: 1024 }, "logo"),
      ).toBeNull();
    }
  });

  it("refuses other types and oversized files", () => {
    expect(
      vehicleImageProblem(
        { name: "x.gif", type: "image/gif", size: 1 },
        "hero",
      ),
    ).toBe("type");
    expect(
      vehicleImageProblem(
        {
          name: "x.png",
          type: "image/png",
          size: VEHICLE_IMAGE_LIMITS.logo + 1,
        },
        "logo",
      ),
    ).toBe("size");
    expect(
      vehicleImageProblem(
        {
          name: "x.png",
          type: "image/png",
          size: VEHICLE_IMAGE_LIMITS.logo + 1,
        },
        "hero",
      ),
    ).toBeNull();
  });
});

describe("brandLogoUrl", () => {
  it("builds the fixed public logo URL", () => {
    expect(brandLogoUrl(UUID)).toBe(`/brand-logos/${UUID}`);
    expect(brandLogoUrl(UUID, "a1b2")).toBe(`/brand-logos/${UUID}?v=a1b2`);
  });

  it("reads the cache-buster of a backend logo_url", () => {
    expect(logoVersion(`/brand-logos/${UUID}?v=a1b2`)).toBe("a1b2");
    expect(logoVersion(`/brand-logos/${UUID}`)).toBeNull();
    expect(logoVersion(null)).toBeNull();
  });
});
