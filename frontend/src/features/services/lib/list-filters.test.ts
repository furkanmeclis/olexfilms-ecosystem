import { describe, expect, it } from "vitest";

import {
  SERVICE_STATUSES,
  dayEndIso,
  dayStartIso,
  invalidDateRange,
  pageCount,
} from "./list-filters";

describe("service list helpers (TEC-183)", () => {
  it("lists the statuses in flow order", () => {
    expect(SERVICE_STATUSES).toEqual([
      "draft",
      "pending",
      "processing",
      "ready",
      "completed",
      "cancelled",
    ]);
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

  it("flags a reversed range", () => {
    expect(invalidDateRange("2026-10-02", "2026-10-01")).toBe(true);
    expect(invalidDateRange("2026-10-01", "2026-10-01")).toBe(false);
    expect(invalidDateRange("", "2026-10-01")).toBe(false);
  });

  it("counts pages", () => {
    expect(pageCount(0, 20)).toBe(1);
    expect(pageCount(20, 20)).toBe(1);
    expect(pageCount(21, 20)).toBe(2);
  });
});
