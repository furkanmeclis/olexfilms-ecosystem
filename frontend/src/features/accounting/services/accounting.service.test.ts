import { afterEach, describe, expect, it, vi } from "vitest";

const request = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/platform-request", () => ({ platformRequest: request }));
vi.mock("@/lib/api/platform-form-request", () => ({
  platformDownloadFile: vi.fn(),
  triggerBrowserDownload: vi.fn(),
}));

import { accountingService } from "./accounting.service";

afterEach(() => request.mockReset());

describe("accountingService lists (TEC-380)", () => {
  it("passes the list params through, booleans as strings", () => {
    accountingService.listCari({
      limit: 20,
      offset: 0,
      sort: "-balance",
      active: true,
      counterparty_kind: "dealer,customer",
    });
    expect(request).toHaveBeenLastCalledWith("GET", "/v1/accounting/cari", {
      query: {
        limit: 20,
        offset: 0,
        sort: "-balance",
        active: "true",
        counterparty_kind: "dealer,customer",
      },
    });

    accountingService.listDisputes({ status: "open,rejected", q: "iade" });
    expect(request).toHaveBeenLastCalledWith("GET", "/v1/accounting/disputes", {
      query: { status: "open,rejected", q: "iade" },
    });
  });

  it("queues the ledger export with the list query", () => {
    accountingService.exportEntries(
      "xlsx",
      { direction: "income", q: "kira", sort: "-amount" },
      "tr",
    );
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/accounting/entries/export",
      {
        body: {
          format: "xlsx",
          query: { direction: "income", q: "kira", sort: "-amount" },
          locale: "tr",
        },
      },
    );
  });
});
