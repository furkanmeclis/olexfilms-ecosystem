import { describe, expect, it } from "vitest";

import {
  extractServiceNo,
  isValidServiceNo,
} from "@/features/services/lib/service-no";

describe("extractServiceNo", () => {
  it.each([
    ["https://olexfilms.app/warranty/1EVMEEI9", "1EVMEEI9"],
    ["  https://olexfilms.app/warranty/DS12AB34CD/pdf ", "DS12AB34CD"],
    ["http://olexfilms.app/warranty/AB-12_cd?lang=tr", "AB-12_cd"],
    ["olexfilms.app/warranty/ABCD1234", "ABCD1234"],
    ["https://panel.example.test/Warranty/X%2D1234#top", "X-1234"],
    [" DS12AB34CD ", "DS12AB34CD"],
    ["2025D2333D014-CC-210", "2025D2333D014-CC-210"],
  ])("%s -> %s", (input, want) => {
    expect(extractServiceNo(input)).toBe(want);
  });
});

describe("isValidServiceNo", () => {
  it("accepts 4..32 URL-safe characters", () => {
    expect(isValidServiceNo("DS12AB34CD")).toBe(true);
    expect(isValidServiceNo("card_1")).toBe(true);
    expect(isValidServiceNo("abc")).toBe(false);
    expect(isValidServiceNo("2025 3207")).toBe(false);
    expect(isValidServiceNo("1EVMEEİ9")).toBe(false);
    expect(isValidServiceNo("A".repeat(33))).toBe(false);
  });
});
