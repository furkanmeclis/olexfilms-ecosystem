import { describe, expect, it } from "vitest";

import {
  formatPlate,
  matchPlate,
  normalizePlate,
} from "@/features/geo/lib/plate";

// Same expressions as the backend seed (migration 000035).
const DE = "^[A-ZÄÖÜ]{1,3}[A-Z]{1,2}[1-9][0-9]{0,3}[EH]?$";
const TR = "^(0[1-9]|[1-7][0-9]|8[01])[A-Z]{1,3}[0-9]{2,5}$";

describe("plate helpers", () => {
  it("normalizes separators and case", () => {
    expect(normalizePlate(" b-ab 12.34 ")).toBe("BAB1234");
  });

  it("matches the seeded DE and TR formats", () => {
    expect(matchPlate(DE, "B AB 1234")).toBe(true);
    expect(matchPlate(DE, "MÜ AB 12")).toBe(true);
    expect(matchPlate(DE, "B AB 0123")).toBe(false);
    expect(matchPlate(TR, "34 ABC 123")).toBe(true);
    expect(matchPlate(TR, "82 ABC 123")).toBe(false);
  });

  it("rejects empty plates and broken regexes", () => {
    expect(matchPlate(DE, "  ")).toBe(false);
    expect(matchPlate("^[A-Z$", "AB")).toBe(false);
  });

  it("groups by the input mask", () => {
    expect(formatPlate("34abc123", "99 AAA 999")).toBe("34 ABC 123");
    expect(formatPlate("ab123c", "XX-999-X")).toBe("AB-123-C");
  });
});
