import { describe, expect, it } from "vitest";

import { periodRange } from "@/features/overview/lib/period-range";

const NOW = new Date(2026, 10, 15, 14, 0);

describe("periodRange", () => {
  it("resolves presets and the previous window", () => {
    expect(periodRange({ period: "30d", compare: "previous" }, NOW)).toEqual({
      from: "2026-10-17",
      to: "2026-11-15",
      previous: { from: "2026-09-17", to: "2026-10-16" },
    });
    expect(periodRange({ period: "12m" }, NOW)).toEqual({
      from: "2025-12-01",
      to: "2026-11-15",
      previous: null,
    });
  });

  it("uses custom ranges and the year comparison", () => {
    expect(
      periodRange(
        { range_from: "2026-01-01", range_to: "2026-01-31", compare: "year" },
        NOW,
      ),
    ).toEqual({
      from: "2026-01-01",
      to: "2026-01-31",
      previous: { from: "2025-01-01", to: "2025-01-31" },
    });
  });
});
