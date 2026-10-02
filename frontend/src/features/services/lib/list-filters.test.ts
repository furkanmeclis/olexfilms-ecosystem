import { describe, expect, it } from "vitest";

import {
  EMPTY_SERVICE_FILTERS,
  buildServiceListQuery,
  dayEndIso,
  dayStartIso,
  invalidDateRange,
  pageCount,
} from "./list-filters";

const pg = { limit: 20, offset: 40 };

describe("service list filters (TEC-183)", () => {
  it("leaves empty filters out", () => {
    expect(buildServiceListQuery(EMPTY_SERVICE_FILTERS, pg)).toEqual(pg);
    expect(
      buildServiceListQuery({ ...EMPTY_SERVICE_FILTERS, q: "   " }, pg),
    ).toEqual(pg);
  });

  it("maps status and trims the search to 100 characters", () => {
    const q = buildServiceListQuery(
      { ...EMPTY_SERVICE_FILTERS, status: "ready", q: ` ${"x".repeat(120)} ` },
      pg,
    );
    expect(q.status).toBe("ready");
    expect(q.q).toHaveLength(100);
  });

  it("turns days into local-day ISO bounds (to is exclusive)", () => {
    expect(Date.parse(dayStartIso("2026-02-28") ?? "")).toBe(
      new Date(2026, 1, 28).getTime(),
    );
    expect(Date.parse(dayEndIso("2026-02-28") ?? "")).toBe(
      new Date(2026, 2, 1).getTime(),
    );
    expect(dayStartIso("2026-02-28")).toMatch(/Z$/);
    expect(dayStartIso("2026-02-28")).not.toMatch(/\.\d{3}Z$/);
    expect(dayStartIso("")).toBeUndefined();
    expect(dayStartIso("28.02.2026")).toBeUndefined();
  });

  it("sends a single bound and drops a reversed range", () => {
    const from = buildServiceListQuery(
      { ...EMPTY_SERVICE_FILTERS, from: "2026-10-01" },
      pg,
    );
    expect(from.created_from).toBeDefined();
    expect(from.created_to).toBeUndefined();

    expect(invalidDateRange("2026-10-02", "2026-10-01")).toBe(true);
    expect(invalidDateRange("2026-10-01", "2026-10-01")).toBe(false);
    expect(invalidDateRange("", "2026-10-01")).toBe(false);
    const reversed = buildServiceListQuery(
      { ...EMPTY_SERVICE_FILTERS, from: "2026-10-02", to: "2026-10-01" },
      pg,
    );
    expect(reversed.created_from).toBeUndefined();
    expect(reversed.created_to).toBeUndefined();
  });

  it("counts pages", () => {
    expect(pageCount(0, 20)).toBe(1);
    expect(pageCount(20, 20)).toBe(1);
    expect(pageCount(21, 20)).toBe(2);
  });
});
