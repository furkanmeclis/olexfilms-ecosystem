import { describe, expect, it } from "vitest";

import {
  clusterDealers,
  deltaTone,
  formatMetric,
  metricColor,
  metricModuleOff,
  rankingCsv,
  recentMonths,
  regionRadius,
  targetAchievement,
} from "@/features/performance/lib/performance";
import { layoutInput } from "@/features/performance/lib/report-layout";
import type {
  PerformanceDealerPoint,
  PerformanceTarget,
} from "@/features/performance/services/performance.service";

const format = {
  number: (v: number) => `n(${v})`,
  percent: (v: number) => `${Math.round(v * 1000) / 10}%`,
  currency: (v: number, c: string) => `${v} ${c}`,
};

function point(uuid: string, lat: number, lng: number): PerformanceDealerPoint {
  return {
    uuid,
    code: uuid,
    name: uuid,
    country_iso2: "TR",
    latitude: lat,
    longitude: lng,
    showcase_url: "",
  };
}

function target(partial: Partial<PerformanceTarget>): PerformanceTarget {
  return {
    metric: "services_count",
    period_start: "2026-09-01",
    period_end: "2026-09-30",
    achievement_pct: "80",
    ...partial,
  } as PerformanceTarget;
}

describe("performance lib", () => {
  it("reports a metric as module off only when its module is disabled", () => {
    expect(metricModuleOff("review_avg", ["performance"])).toBe(true);
    expect(metricModuleOff("review_avg", ["performance", "reviews"])).toBe(
      false,
    );
    // No module behind the metric, or features still unknown.
    expect(metricModuleOff("services_count", [])).toBe(false);
    expect(metricModuleOff("waste_ratio", null)).toBe(false);
  });

  it("formats ratios, money and counts; no value is a dash", () => {
    expect(formatMetric("warranty_start_rate", { value: "0.25" }, format)).toBe(
      "25%",
    );
    expect(
      formatMetric("order_volume", { value: "1500", currency: "TRY" }, format),
    ).toBe("1500 TRY");
    expect(formatMetric("services_count", { value: "12.00" }, format)).toBe(
      "n(12)",
    );
    expect(formatMetric("review_avg", { value: "" }, format)).toBe("—");
    expect(formatMetric("review_avg", null, format)).toBe("—");
  });

  it("treats a rising overdue or waste metric as a bad change", () => {
    expect(deltaTone("services_count", "12.5")).toBe("up");
    expect(deltaTone("services_count", "-3")).toBe("down");
    expect(deltaTone("waste_ratio", "10")).toBe("down");
    expect(deltaTone("cari_overdue_days", "-10")).toBe("up");
    expect(deltaTone("services_count", null)).toBe("flat");
  });

  it("averages the achievement of the targets covering the month", () => {
    const targets = [
      target({ achievement_pct: "80" }),
      target({ achievement_pct: "120", period_end: "2026-12-31" }),
      target({ metric: "order_volume", achievement_pct: "10" }),
      target({ period_start: "2026-10-01", period_end: "2026-10-31" }),
    ];
    expect(targetAchievement(targets, "services_count", "2026-09")).toBe(100);
    expect(targetAchievement(targets, "services_count", "2026-08")).toBeNull();
  });

  it("lists the last 12 months newest first", () => {
    const months = recentMonths(new Date(2026, 0, 15), 12);
    expect(months[0]).toBe("2026-01");
    expect(months[1]).toBe("2025-12");
    expect(months).toHaveLength(12);
  });

  it("clusters dealers in the same grid cell", () => {
    const clusters = clusterDealers(
      [
        point("a", 41.01, 29.01),
        point("b", 41.2, 29.3),
        point("c", 39.9, 32.8),
      ],
      0.5,
    );
    expect(clusters).toHaveLength(2);
    const pair = clusters.find((c) => c.dealers.length === 2)!;
    expect(pair.id.startsWith("cluster:")).toBe(true);
    expect(pair.lat).toBeCloseTo(41.105);
    expect(clusters.find((c) => c.dealers.length === 1)!.id).toBe("dealer:c");
  });

  it("sizes circles by dealer count and colors by metric", () => {
    expect(regionRadius(0, 10)).toBe(6);
    expect(regionRadius(10, 10)).toBe(30);
    expect(regionRadius(1, 100)).toBeLessThan(regionRadius(50, 100));
    expect(metricColor("services_count", 10, 0, 10)).toBe("#16a34a");
    expect(metricColor("waste_ratio", 10, 0, 10)).toBe("#dc2626");
    expect(metricColor("services_count", null, 0, 10)).toBe("#94a3b8");
  });

  it("writes the ranking CSV with distributor, province and metrics", () => {
    const csv = rankingCsv([
      {
        rank: 1,
        name: 'Oto "Kuzey"',
        type: "dealer",
        distributor: { uuid: "d", name: "Ege" },
        province_name: "İzmir",
        metrics: { services_count: { value: "5.00" } },
      },
    ]);
    const [header, line] = csv.trim().split("\n");
    expect(
      header.startsWith(
        "rank,organization,type,distributor,province,services_count",
      ),
    ).toBe(true);
    expect(line.startsWith('1,"Oto ""Kuzey""",dealer,Ege,İzmir,5.00')).toBe(
      true,
    );
  });

  it("sends the layout in display order with null periods", () => {
    expect(
      layoutInput([
        { id: "b", report: "orders.trend", period: "90d", granularity: null },
        { id: "a", report: "overview", period: null, granularity: null },
      ]),
    ).toEqual({
      version: 1,
      widgets: [
        { id: "b", report: "orders.trend", period: "90d", granularity: null },
        { id: "a", report: "overview", period: null, granularity: null },
      ],
    });
  });
});
