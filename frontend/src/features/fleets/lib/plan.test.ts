import { describe, expect, it } from "vitest";

import {
  countByDay,
  dayInZone,
  lastClosedPeriods,
  moveToDay,
  parsePreferredTimes,
  rowWarnings,
  unscheduledVehicles,
} from "@/features/fleets/lib/plan";

const rows = [
  { vehicle_uuid: "a", starts_at: "2026-10-12T06:00:00Z" },
  { vehicle_uuid: "b", starts_at: "2026-10-12T07:00:00Z" },
  { vehicle_uuid: "c", starts_at: "2026-10-12T22:30:00Z" },
];

describe("fleet plan helpers (TEC-477)", () => {
  it("groups appointments by the day of the dealer's time zone", () => {
    expect(dayInZone("2026-10-12T22:30:00Z", "Europe/Istanbul")).toBe(
      "2026-10-13",
    );
    expect(countByDay(rows, "UTC").get("2026-10-12")).toBe(3);
    expect(countByDay(rows, "Europe/Istanbul").get("2026-10-12")).toBe(2);
  });

  it("moves an appointment to another day keeping its wall-clock time", () => {
    expect(
      moveToDay("2026-10-12T06:00:00Z", "2026-10-15", "Europe/Istanbul"),
    ).toBe("2026-10-15T06:00:00.000Z");
    expect(moveToDay("2026-10-12T06:00:00Z", "", "UTC")).toBe(
      "2026-10-12T06:00:00Z",
    );
  });

  it("flags a day over the daily limit and the server warnings of the row", () => {
    const warnings = [
      { vehicle_uuid: "a", date: "2026-10-12", code: "X", message: "x" },
    ];
    expect(rowWarnings(rows[0], rows, warnings, 2, "UTC")).toEqual([
      { kind: "server", code: "X", message: "x" },
      { kind: "capacity", day: "2026-10-12", count: 3, limit: 2 },
    ]);
    expect(rowWarnings(rows[1], rows, warnings, 3, "UTC")).toEqual([]);
    expect(rowWarnings(rows[1], rows, [], undefined, "UTC")).toEqual([]);
  });

  it("lists the vehicles the preview could not place", () => {
    expect(unscheduledVehicles(["a", "b", "x"], rows)).toEqual(["x"]);
  });

  it("parses the preferred hours", () => {
    expect(parsePreferredTimes(" 09:00, 14:30 ")).toEqual(["09:00", "14:30"]);
    expect(parsePreferredTimes("")).toEqual([]);
    expect(parsePreferredTimes("9am")).toBeNull();
  });

  it("proposes the last closed month and quarter", () => {
    expect(lastClosedPeriods(new Date(2026, 9, 8))).toEqual({
      month: "2026-09",
      quarter: "2026-Q3",
    });
    expect(lastClosedPeriods(new Date(2026, 0, 5))).toEqual({
      month: "2025-12",
      quarter: "2025-Q4",
    });
  });
});
