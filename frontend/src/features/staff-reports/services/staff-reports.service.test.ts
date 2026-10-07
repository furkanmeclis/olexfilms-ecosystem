import { afterEach, describe, expect, it, vi } from "vitest";

const request = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/platform-request", () => ({ platformRequest: request }));

import { staffReportsService } from "./staff-reports.service";

afterEach(() => request.mockReset());

describe("staffReportsService exports (TEC-349)", () => {
  it("each report export goes to its own endpoint with its own fields", async () => {
    request.mockResolvedValue({ uuid: "job", status: "queued" });
    const query = {
      from: "2026-01-01",
      to: "2026-03-31",
      as_of: "2026-03-31",
      group: "category" as const,
    };

    await staffReportsService.exportReport("pnl", "csv", query, "tr");
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/accounting/reports/pnl/export",
      {
        body: {
          format: "csv",
          from: "2026-01-01",
          to: "2026-03-31",
          group: "category",
          locale: "tr",
        },
      },
    );

    await staffReportsService.exportReport("margin", "xlsx", query);
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/accounting/reports/margin/export",
      { body: { format: "xlsx", from: "2026-01-01", to: "2026-03-31" } },
    );

    await staffReportsService.exportReport("cari-aging", "pdf", query);
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/accounting/reports/cari-aging/export",
      { body: { format: "pdf", as_of: "2026-03-31" } },
    );

    await staffReportsService.exportReport("staff-cost", "csv", {});
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/accounting/reports/staff-cost/export",
      { body: { format: "csv" } },
    );
  });

  it("reads the reports with only the set filters", async () => {
    request.mockResolvedValue({});
    await staffReportsService.pnl({ from: "2026-01-01", group: "month" });
    expect(request).toHaveBeenLastCalledWith(
      "GET",
      "/v1/accounting/reports/pnl",
      { query: { from: "2026-01-01", group: "month" } },
    );
    await staffReportsService.cariAging({ as_of: "" });
    expect(request).toHaveBeenLastCalledWith(
      "GET",
      "/v1/accounting/reports/cari-aging",
      { query: {} },
    );
  });
});

describe("staffReportsService staff", () => {
  it("reads every page of the staff list", async () => {
    const page = (n: number, offset: number) => ({
      items: Array.from({ length: n }, (_, i) => ({ uuid: `s-${offset + i}` })),
      total: 130,
      limit: 100,
      offset,
    });
    request
      .mockResolvedValueOnce(page(100, 0))
      .mockResolvedValueOnce(page(30, 100));
    const res = await staffReportsService.listStaff();
    expect(res.items).toHaveLength(130);
    expect(request).toHaveBeenNthCalledWith(1, "GET", "/v1/staff-profiles", {
      query: { limit: 100, offset: 0 },
    });
    expect(request).toHaveBeenNthCalledWith(2, "GET", "/v1/staff-profiles", {
      query: { limit: 100, offset: 100 },
    });
  });

  it("payments, history and payroll hit the staff endpoints", async () => {
    request.mockResolvedValue({ items: [], total: 0, limit: 100, offset: 0 });
    await staffReportsService.listPayments("s 1");
    expect(request).toHaveBeenLastCalledWith(
      "GET",
      "/v1/staff-profiles/s%201/payments",
      { query: { limit: 100, offset: 0 } },
    );
    await staffReportsService.createPayment("s-1", {
      type: "salary",
      period: "2026-10",
    });
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/staff-profiles/s-1/payments",
      { body: { type: "salary", period: "2026-10" } },
    );
    await staffReportsService.runPayroll("2026-10");
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/staff-payments/payroll",
      { query: { period: "2026-10" } },
    );
  });
});
