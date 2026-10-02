import { describe, expect, it } from "vitest";

import { tenantNav } from "@/config/nav";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { visibleNavItems } from "@/features/nav-engine/lib/access";

import {
  buildWarrantyListQuery,
  EMPTY_WARRANTY_FILTERS,
  hasWarrantyFilters,
  pageCount,
  progressTone,
  warrantyHolderName,
  warrantyProgress,
  warrantyVehicleTitle,
} from "./warranty-list";

const START = "2026-01-01T00:00:00Z";
const END = "2027-01-01T00:00:00Z"; // 365 days

describe("warrantyProgress (TEC-191)", () => {
  it("is 0% with all days left at the start", () => {
    const p = warrantyProgress(START, END, new Date(START));
    expect(p).toEqual({
      percent: 0,
      daysLeft: 365,
      totalDays: 365,
      ended: false,
    });
  });

  it("computes the elapsed share and rounds partial days up", () => {
    const p = warrantyProgress(START, END, new Date("2026-07-02T12:00:00Z"));
    expect(p.percent).toBe(50);
    expect(p.daysLeft).toBe(183);
    expect(p.ended).toBe(false);
  });

  it("clamps before the start and after the end", () => {
    expect(warrantyProgress(START, END, new Date("2025-06-01")).percent).toBe(
      0,
    );
    const after = warrantyProgress(START, END, new Date("2027-03-01"));
    expect(after).toMatchObject({ percent: 100, daysLeft: 0, ended: true });
  });

  it("treats a broken period as ended", () => {
    expect(warrantyProgress(END, START)).toMatchObject({
      percent: 100,
      ended: true,
    });
    expect(warrantyProgress("nope", END).ended).toBe(true);
  });

  it("tones: last 30 days amber, void / ended muted", () => {
    const near = warrantyProgress(START, END, new Date("2026-12-15"));
    expect(progressTone("active", near)).toBe("warning");
    const far = warrantyProgress(START, END, new Date("2026-02-01"));
    expect(progressTone("active", far)).toBe("success");
    expect(progressTone("void", far)).toBe("muted");
    expect(progressTone("expired", far)).toBe("muted");
  });
});

describe("buildWarrantyListQuery", () => {
  const page = { limit: 20, offset: 40 };

  it("drops empty filters", () => {
    expect(buildWarrantyListQuery(EMPTY_WARRANTY_FILTERS, page)).toEqual({
      limit: 20,
      offset: 40,
    });
    expect(hasWarrantyFilters(EMPTY_WARRANTY_FILTERS)).toBe(false);
  });

  it("maps status, search, product and the ends-within preset", () => {
    const f = {
      status: "expired" as const,
      endsWithin: "all" as const,
      q: "  34 abc 12 ",
      productUuid: "p-1",
    };
    expect(buildWarrantyListQuery(f, page)).toEqual({
      limit: 20,
      offset: 40,
      status: "expired",
      q: "34 abc 12",
      product_uuid: "p-1",
    });
    expect(hasWarrantyFilters(f)).toBe(true);
  });

  it("ends within N days asks for active warranties by default", () => {
    expect(
      buildWarrantyListQuery(
        { ...EMPTY_WARRANTY_FILTERS, endsWithin: 30 },
        page,
      ),
    ).toEqual({ limit: 20, offset: 40, status: "active", days_left_max: 30 });
  });

  it("caps the search at 100 characters", () => {
    const q = buildWarrantyListQuery(
      { ...EMPTY_WARRANTY_FILTERS, q: "x".repeat(150) },
      page,
    ).q;
    expect(q).toHaveLength(100);
  });

  it("formats names and pages", () => {
    expect(
      warrantyVehicleTitle({
        vehicle: {
          uuid: "v",
          brand_name: "BMW",
          model_name: "X5",
          model_year: 2024,
          plate: null,
        },
      }),
    ).toBe("BMW X5 (2024)");
    expect(
      warrantyHolderName({
        holder: { uuid: "h", name: "Ayşe", surname: "Kaya", anonymized: false },
      }),
    ).toBe("Ayşe Kaya");
    expect(warrantyHolderName({})).toBe("");
    expect(pageCount(0, 20)).toBe(1);
    expect(pageCount(41, 20)).toBe(3);
  });
});

describe("tenant nav: warranties (TEC-191)", () => {
  function visibleIds(granted: string[]) {
    const access = {
      can: (p: string | string[]) =>
        (Array.isArray(p) ? p : [p]).every((x) => granted.includes(x)),
      canAny: (ps: string[]) => ps.some((x) => granted.includes(x)),
      org: { type: "dealer", role: "owner", features: ["services"] },
    };
    return tenantNav("acme").groups.flatMap((group) =>
      visibleNavItems(group, access).map((item) => item.id),
    );
  }

  it("links to the warranty list with warranties.read", () => {
    const item = tenantNav("acme")
      .groups.flatMap((g) => g.items)
      .find((i) => i.id === "warranties-list");
    expect(item?.href).toBe(routes.tenant.warranties.list("acme"));
    expect(visibleIds([Permission.WarrantiesRead])).toContain(
      "warranties-list",
    );
    expect(visibleIds([Permission.ServicesRead])).not.toContain(
      "warranties-list",
    );
  });
});
