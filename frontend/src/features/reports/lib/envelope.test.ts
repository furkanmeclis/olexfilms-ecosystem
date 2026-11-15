import { describe, expect, it } from "vitest";

import {
  asV2,
  rankingPointHref,
  reportGridColumns,
  reportHref,
  reportKpiTiles,
  splitSeries,
} from "@/features/reports/lib/envelope";

const UUID = "0b9c4c1e-0000-4000-8000-0000000000f1";

describe("splitSeries", () => {
  it("drops the cards series and splits dates from categories", () => {
    const split = splitSeries([
      {
        key: "cards",
        label: "Cards",
        points: [{ key: "a", label: "A", value: 1 }],
      },
      {
        key: "created",
        label: "Created",
        points: [
          { key: "2026-11-01", label: "1 Nov", value: 3, previous: 2 },
          { key: "2026-11-02", label: "2 Nov", value: 4, previous: 1 },
        ],
      },
      {
        key: "status",
        label: "Status",
        points: [{ key: "completed", label: "Completed", value: 9 }],
      },
      { key: "empty", label: "Empty", points: [] },
    ]);
    expect(split.trends.map((s) => s.key)).toEqual(["created"]);
    expect(split.categories.map((s) => s.key)).toEqual(["status"]);
  });
});

describe("reportGridColumns", () => {
  it("bridges percent ratios and duration hours to the kit units", () => {
    const cols = reportGridColumns({
      columns: [
        { key: "share", label: "Share", type: "percent" },
        { key: "prep", label: "Prep", type: "duration" },
        { key: "name", label: "Name", type: "string" },
      ],
    });
    const row = { share: 0.25, prep: "1.5", name: "x" };
    expect(cols[0]?.accessor?.(row)).toBe(25);
    expect(cols[1]?.accessor?.(row)).toBe(5400);
    expect(cols[1]?.accessor?.({ prep: null })).toBeNull();
    expect(cols[2]?.type).toBe("text");
    expect(cols[2]?.accessor).toBeUndefined();
  });
});

describe("reportHref", () => {
  it("opens the filtered list for organizations and the detail for reports", () => {
    const org = { kind: "organization", uuid: UUID };
    expect(reportHref(org, "acme", "orders.by_buyer")).toBe(
      `/t/acme/orders?buyer_org_uuid=${UUID}`,
    );
    expect(reportHref(org, "acme", "network.summary")).toBe(
      `/t/acme/services?organization_uuid=${UUID}`,
    );
    expect(
      reportHref({ kind: "report", report: "stock.movement" }, "acme", "x"),
    ).toBe("/t/acme/reports/stock.movement");
    expect(reportHref({ kind: "service", uuid: UUID }, "acme", "x")).toBe(
      `/t/acme/services/${UUID}`,
    );
    expect(reportHref(null, "acme", "x")).toBeNull();
  });

  it("links ranking points only for uuid keys", () => {
    expect(rankingPointHref("services.by_org", UUID, "acme")).toContain(
      "organization_uuid=",
    );
    expect(rankingPointHref("services.top_products", UUID, "acme")).toBe(
      `/t/acme/catalog/products/${UUID}`,
    );
    expect(
      rankingPointHref("services.top_brands", "bmw", "acme"),
    ).toBeUndefined();
  });
});

describe("reportKpiTiles / asV2", () => {
  it("reads period_info and applies the link rules to KPIs", () => {
    const env = asV2({
      report: "network.summary",
      period: "30d",
      period_info: {
        from: "2026-10-17",
        to: "2026-11-15",
        granularity: "week",
      },
      kpis: [
        {
          key: "active",
          label: "Active",
          unit: "count",
          value: 4,
          previous: 3,
          delta_pct: 33.3,
          good: "up",
          spark: [1, 2],
          link: { kind: "report", report: "dealers.performance" },
        },
      ],
    });
    expect(typeof env.period === "object" && env.period?.granularity).toBe(
      "week",
    );
    const [tile] = reportKpiTiles(env, "acme");
    expect(tile?.href).toBe("/t/acme/reports/dealers.performance");
    expect(tile?.deltaPct).toBe(33.3);
  });
});
