import { describe, expect, it } from "vitest";

import { permissions } from "@/config/permissions";
import {
  groupByCategory,
  reportCategory,
  visibleSpecialReports,
  reportQueryFor,
} from "@/features/reports/lib/catalog";

describe("reportCategory", () => {
  it("maps explicit and prefixed keys", () => {
    expect(reportCategory("overview")).toBe("general");
    expect(reportCategory("services.trend")).toBe("services");
    expect(reportCategory("services.by_org")).toBe("network");
    expect(reportCategory("orders.fulfillment")).toBe("warehouse");
    expect(reportCategory("stock.movement")).toBe("warehouse");
    expect(reportCategory("network.summary")).toBe("network");
    expect(reportCategory("unknown.thing")).toBe("other");
  });
});

describe("groupByCategory", () => {
  it("keeps the category order and drops empty ones", () => {
    const groups = groupByCategory(
      ["stock.summary", "services.trend", "overview", "services.top_brands"],
      reportCategory,
    );
    expect(groups.map((g) => g.category)).toEqual([
      "general",
      "services",
      "warehouse",
    ]);
    expect(groups[1]?.items).toEqual(["services.trend", "services.top_brands"]);
  });
});

describe("visibleSpecialReports", () => {
  it("needs the permission, the module and the organization type", () => {
    const can = (p: string) =>
      p === permissions.accounting.read ||
      p === permissions.warrantyClaims.read;
    expect(visibleSpecialReports(can, "dealer", null)).toEqual([]);
    const keys = (orgType: string) =>
      visibleSpecialReports(can, orgType, [
        "accounting",
        "warranty_claims",
        "performance",
      ]).map((r) => r.key);
    expect(keys("dealer")).toEqual(["accounting.reports"]);
    expect(keys("center")).toEqual([
      "accounting.reports",
      "warranty_claims.reports",
    ]);
  });
});

describe("reportQueryFor", () => {
  it("drops the period params of snapshot reports", () => {
    const q = {
      period: "30d",
      compare: "previous",
      range_from: "2026-01-01",
      limit: 10,
    };
    expect(reportQueryFor("stock.summary", q)).toEqual({ limit: 10 });
    expect(reportQueryFor("activities.recent", q)).toEqual({ limit: 10 });
  });
  it("keeps them for periodic reports", () => {
    const q = { period: "30d", compare: "previous" };
    expect(reportQueryFor("services.trend", q)).toBe(q);
  });
});
