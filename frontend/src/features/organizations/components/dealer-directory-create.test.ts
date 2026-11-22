import { describe, expect, it } from "vitest";

import { canCreateDealer } from "./dealer-directory-page";

// TEC-1267: "Yeni bayi" on the dealer directory follows the settings rule.
describe("canCreateDealer", () => {
  it("opens for a distributor and the center with organizations.write", () => {
    expect(canCreateDealer("distributor", true)).toBe(true);
    expect(canCreateDealer("center", true)).toBe(true);
  });
  it("stays closed for a dealer or without the permission", () => {
    expect(canCreateDealer("dealer", true)).toBe(false);
    expect(canCreateDealer("distributor", false)).toBe(false);
    expect(canCreateDealer(undefined, true)).toBe(false);
  });
});
