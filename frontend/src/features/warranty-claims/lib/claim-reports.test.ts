import { describe, expect, it } from "vitest";

import {
  buildExportRequest,
  buildPeriodQuery,
  dealerFilterOptions,
  filterDealerRows,
  formatClaimRate,
  isPeriodValid,
  tabReportKind,
  toFailureRateBars,
} from "@/features/warranty-claims/lib/claim-reports";
import {
  claimReportExportPath,
  type ByDealerRow,
  type FailureRateRow,
} from "@/features/warranty-claims/services/claim-reports.service";

function failure(over: Partial<FailureRateRow> = {}): FailureRateRow {
  return {
    group: "product",
    product_uuid: "p-1",
    product_sku: "PPF-1",
    product_name: "Film PPF",
    lot_uuid: null,
    lot_code: null,
    warranty_count: 8,
    claim_count: 2,
    approved_claim_count: 1,
    claim_rate: 0.25,
    approved_rate: 0.125,
    ...over,
  };
}

function dealer(over: Partial<ByDealerRow> = {}): ByDealerRow {
  return {
    organization_uuid: "o-1",
    organization_name: "Dealer A",
    organization_type: "dealer",
    claim_count: 3,
    approved_claim_count: 2,
    rejected_claim_count: 1,
    approval_rate: 2 / 3,
    ...over,
  };
}

describe("formatClaimRate", () => {
  it("formats API fractions as Intl percentages in the locale", () => {
    const en = new Intl.NumberFormat("en", {
      style: "percent",
      maximumFractionDigits: 1,
    }).format(0.125);
    const tr = new Intl.NumberFormat("tr", {
      style: "percent",
      maximumFractionDigits: 1,
    }).format(0.125);
    expect(formatClaimRate(0.125, { locale: "en" })).toBe(en);
    expect(formatClaimRate(0.125, { locale: "en" })).toBe("12.5%");
    expect(formatClaimRate(0.125, { locale: "tr" })).toBe(tr);
    expect(formatClaimRate(0.125, { locale: "tr" })).toBe("%12,5");
    expect(formatClaimRate(2 / 3, { locale: "en" })).toBe("66.7%");
    expect(formatClaimRate(0, { locale: "en" })).toBe("0%");
  });
});

describe("period", () => {
  it("drops empty bounds and rejects from after to", () => {
    expect(buildPeriodQuery({ from: "", to: "2026-09-30" })).toEqual({
      to: "2026-09-30",
    });
    expect(isPeriodValid({ from: "2026-10-01", to: "2026-09-30" })).toBe(false);
    expect(isPeriodValid({ from: "2026-09-01", to: "2026-09-30" })).toBe(true);
    expect(isPeriodValid({ from: "2026-09-01" })).toBe(true);
  });
});

describe("export request", () => {
  it("maps each tab to its report export endpoint", () => {
    expect(claimReportExportPath(tabReportKind("product"))).toBe(
      "/v1/warranty-claims/reports/failure-rate/export",
    );
    expect(claimReportExportPath(tabReportKind("lot"))).toBe(
      "/v1/warranty-claims/reports/failure-rate/export",
    );
    expect(claimReportExportPath(tabReportKind("dealer"))).toBe(
      "/v1/warranty-claims/reports/by-dealer/export",
    );
    expect(claimReportExportPath(tabReportKind("parts"))).toBe(
      "/v1/warranty-claims/reports/parts/export",
    );
  });

  it("sends the group only for the failure-rate tabs", () => {
    expect(
      buildExportRequest("lot", { from: "2026-01-01" }, "csv", "de"),
    ).toEqual({
      format: "csv",
      locale: "de",
      query: { from: "2026-01-01", group: "lot" },
    });
    expect(buildExportRequest("dealer", {}, "xlsx", "tr")).toEqual({
      format: "xlsx",
      locale: "tr",
      query: {},
    });
  });
});

describe("failure rate bars", () => {
  it("sorts by approved rate and skips rows without claims", () => {
    const bars = toFailureRateBars([
      failure({ product_uuid: "a", product_name: "A", approved_rate: 0.1 }),
      failure({ product_uuid: "b", product_name: "B", approved_rate: 0.5 }),
      failure({ product_uuid: "c", claim_count: 0, approved_rate: 0 }),
    ]);
    expect(bars.map((b) => b.label)).toEqual(["B", "A"]);
    expect(bars[0]?.approvedPercent).toBe(50);
    expect(bars[0]?.claimPercent).toBe(25);
  });

  it("labels lot rows with the lot code", () => {
    const [bar] = toFailureRateBars([
      failure({ group: "lot", lot_uuid: "l-1", lot_code: "LOT-7" }),
    ]);
    expect(bar?.label).toBe("Film PPF · LOT-7");
  });
});

describe("dealer filter", () => {
  it("offers only the organizations of the scoped report (distributor subtree)", () => {
    // What the API returns to a distributor: itself and its own dealers.
    const rows = [
      dealer({
        organization_uuid: "dist",
        organization_name: "Distributor X",
        organization_type: "distributor",
      }),
      dealer({ organization_uuid: "d-2", organization_name: "Dealer B" }),
      dealer({ organization_uuid: "d-1", organization_name: "Dealer A" }),
    ];
    expect(dealerFilterOptions(rows)).toEqual([
      { value: "d-1", label: "Dealer A" },
      { value: "d-2", label: "Dealer B" },
      { value: "dist", label: "Distributor X" },
    ]);
    expect(
      filterDealerRows(rows, "d-2").map((r) => r.organization_uuid),
    ).toEqual(["d-2"]);
    expect(filterDealerRows(rows, "")).toHaveLength(3);
  });

  it("never offers the center itself", () => {
    expect(
      dealerFilterOptions([
        dealer({ organization_uuid: "c", organization_type: "center" }),
        dealer(),
      ]).map((o) => o.value),
    ).toEqual(["o-1"]);
  });
});
