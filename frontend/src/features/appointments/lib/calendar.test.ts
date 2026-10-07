import { describe, expect, it } from "vitest";

import type { Appointment } from "@/features/appointments/services/appointments.service";
import { rescheduleBody } from "@/features/appointments/services/appointments.service";

import {
  canStartIntake,
  nextStatuses,
  parseSlotId,
  placeAppointments,
  slotId,
} from "./calendar";
import { daysRange, weekDays, zonedParts, zonedToUtc } from "./time";

const appt = (over: Partial<Appointment>): Appointment => ({
  uuid: "a-1",
  organization_id: 1,
  customer_user_id: 7,
  vehicle_id: 9,
  starts_at: "2026-10-05T07:00:00Z",
  ends_at: "2026-10-05T08:00:00Z",
  estimated_minutes: 60,
  source: "panel",
  status: "scheduled",
  note: "",
  ...over,
});

describe("time zone math (TEC-326)", () => {
  it("builds a Monday-first week", () => {
    expect(weekDays("2026-10-07")).toEqual([
      "2026-10-05",
      "2026-10-06",
      "2026-10-07",
      "2026-10-08",
      "2026-10-09",
      "2026-10-10",
      "2026-10-11",
    ]);
    expect(weekDays("2026-10-11")[0]).toBe("2026-10-05");
  });

  it("converts local wall time to UTC across DST", () => {
    // CEST (UTC+2) before 25 Oct 2026, CET (UTC+1) after.
    expect(zonedToUtc("2026-10-05", 9 * 60, "Europe/Berlin")).toBe(
      "2026-10-05T07:00:00Z",
    );
    expect(zonedToUtc("2026-10-26", 9 * 60, "Europe/Berlin")).toBe(
      "2026-10-26T08:00:00Z",
    );
    expect(zonedToUtc("2026-10-05", 9 * 60, "UTC")).toBe(
      "2026-10-05T09:00:00Z",
    );
    expect(zonedParts("2026-10-26T08:00:00Z", "Europe/Berlin")).toEqual({
      date: "2026-10-26",
      minutes: 9 * 60,
    });
  });

  it("asks whole local days of the zone", () => {
    expect(daysRange(["2026-10-05", "2026-10-06"], "Europe/Berlin")).toEqual({
      from: "2026-10-04T22:00:00Z",
      to: "2026-10-06T22:00:00Z",
    });
  });
});

describe("placeAppointments (TEC-326)", () => {
  const week = weekDays("2026-10-05");

  it("puts a late-evening UTC booking on the next day in Europe/Berlin", () => {
    // 23:30 UTC Monday = 01:30 Tuesday in Berlin (UTC+2).
    const a = appt({ starts_at: "2026-10-05T23:30:00Z" });
    const berlin = placeAppointments([a], week, "Europe/Berlin");
    const utc = placeAppointments([a], week, "UTC");
    expect(berlin["2026-10-06"]?.map((p) => p.start)).toEqual([90]);
    expect(berlin["2026-10-05"]).toEqual([]);
    expect(utc["2026-10-05"]?.map((p) => p.start)).toEqual([23 * 60 + 30]);
    expect(utc["2026-10-06"]).toEqual([]);
  });

  it("drops bookings outside the days and lanes overlapping ones", () => {
    const placed = placeAppointments(
      [
        appt({ uuid: "a", starts_at: "2026-10-05T08:00:00Z" }),
        appt({ uuid: "b", starts_at: "2026-10-05T08:30:00Z" }),
        appt({ uuid: "c", starts_at: "2026-10-05T10:00:00Z" }),
        appt({ uuid: "x", starts_at: "2026-10-20T08:00:00Z" }),
      ],
      week,
      "UTC",
    );
    const monday = placed["2026-10-05"] ?? [];
    expect(monday.map((p) => [p.appointment.uuid, p.lane, p.lanes])).toEqual([
      ["a", 0, 2],
      ["b", 1, 2],
      ["c", 0, 1],
    ]);
    expect(Object.values(placed).flat()).toHaveLength(3);
  });
});

describe("appointment rules (TEC-326)", () => {
  it("mirrors the backend status transitions", () => {
    expect(nextStatuses("scheduled")).toEqual([
      "confirmed",
      "no_show",
      "cancelled",
    ]);
    expect(nextStatuses("confirmed")).toEqual([
      "arrived",
      "no_show",
      "cancelled",
    ]);
    expect(nextStatuses("arrived")).toEqual([]);
  });

  it("starts intake only after arrival, with a vehicle and no service", () => {
    expect(canStartIntake(appt({ status: "confirmed" }))).toBe(false);
    expect(canStartIntake(appt({ status: "arrived" }))).toBe(true);
    expect(canStartIntake(appt({ status: "arrived", vehicle_id: null }))).toBe(
      false,
    );
    expect(
      canStartIntake(appt({ status: "arrived", service_uuid: "svc" })),
    ).toBe(false);
  });

  it("keeps every booking field in the reschedule body", () => {
    const a = appt({
      note: "bring keys",
      estimated_minutes: 90,
      source: "portal",
    });
    expect(rescheduleBody(a, "2026-10-06T08:00:00Z")).toEqual({
      customer_user_id: 7,
      vehicle_id: 9,
      starts_at: "2026-10-06T08:00:00Z",
      estimated_minutes: 90,
      source: "portal",
      note: "bring keys",
    });
  });

  it("round-trips slot ids", () => {
    expect(parseSlotId(slotId("2026-10-06", 600))).toEqual({
      date: "2026-10-06",
      minutes: 600,
    });
    expect(parseSlotId("a-1")).toBeNull();
  });
});
