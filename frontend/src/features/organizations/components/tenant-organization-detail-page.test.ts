import { describe, expect, it } from "vitest";

import { visibleCustomerRelatedTabs } from "@/features/customers/components/customer-related-tabs";

import { visibleOrganizationTabs } from "./tenant-organization-detail-page";

const canOf = (list: string[]) => (slug: string) => list.includes(slug);

describe("visibleOrganizationTabs (TEC-1217)", () => {
  it("shows every tab to a reader with accounting and price permissions", () => {
    const can = canOf([
      "orders.read",
      "services.read",
      "warranties.read",
      "stock.read",
      "accounting.read",
      "tasks.read",
    ]);
    expect(visibleOrganizationTabs(can, true)).toEqual([
      "summary",
      "children",
      "orders",
      "services",
      "warranties",
      "stock",
      "cari",
      "tasks",
    ]);
  });

  it("hides cari from warehouse roles (no price permission)", () => {
    const can = canOf(["orders.read", "stock.read", "accounting.read"]);
    expect(visibleOrganizationTabs(can, false)).toEqual([
      "summary",
      "children",
      "orders",
      "stock",
    ]);
  });
});

describe("visibleCustomerRelatedTabs (TEC-1217)", () => {
  it("lists only tabs with a ready filter and the read permission", () => {
    const kinds = (list: string[]) =>
      visibleCustomerRelatedTabs(canOf(list)).map((tab) => tab.kind);
    expect(kinds(["services.read", "warranties.read"])).toContain("service");
    expect(kinds([])).toEqual([]);
  });
});
