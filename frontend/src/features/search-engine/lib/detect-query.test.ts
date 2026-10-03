import { describe, expect, it } from "vitest";

import { detectQueryKind, prioritizeGroups } from "./detect-query";
import { resolveSearchHitHref } from "./hit-href";

describe("detectQueryKind (TEC-213)", () => {
  it.each([
    ["34ABC123", "plate"],
    ["34 abc 1234", "plate"],
    ["06 t 55", "plate"],
    ["OLX-00000001", "barcode"],
    ["8690000000017", "barcode"],
    ["WVWZZZ1JZXW000001", "vin"],
    ["ahmet", null],
    ["99ABC123", null],
    ["", null],
  ])("%s -> %s", (q, kind) => {
    expect(detectQueryKind(q)).toBe(kind);
  });

  it("moves the matching group first and keeps the rest in order", () => {
    const groups = [
      { spec: "customers" },
      { spec: "vehicles" },
      { spec: "stock_units" },
    ];
    expect(prioritizeGroups(groups, "barcode").map((g) => g.spec)).toEqual([
      "stock_units",
      "customers",
      "vehicles",
    ]);
    expect(prioritizeGroups(groups, null)).toBe(groups);
  });
});

describe("resolveSearchHitHref (TEC-213)", () => {
  it("maps record links into the tenant shell", () => {
    expect(resolveSearchHitHref("/customers/c1", "bayi-a")).toBe(
      "/t/bayi-a/customers/c1",
    );
    expect(resolveSearchHitHref("/warranties/w1", "bayi-a")).toBe(
      "/t/bayi-a/warranties/w1",
    );
    expect(resolveSearchHitHref("/stock/units/OLX-1", "bayi-a")).toBe(
      "/t/bayi-a/stock?barcode=OLX-1",
    );
    expect(resolveSearchHitHref("/organizations/o1", "bayi-a")).toBe(
      "/t/bayi-a/stock?organization=o1",
    );
    expect(resolveSearchHitHref("/platform/users/u1", "bayi-a")).toBe(
      "/platform/users/u1",
    );
    expect(resolveSearchHitHref("/customers/c1", null)).toBe("/customers/c1");
  });
});
