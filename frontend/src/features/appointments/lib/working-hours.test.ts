import { describe, expect, it } from "vitest";

import {
  validateWorkingHours,
  workingHoursFromApi,
  workingHoursToApi,
} from "./working-hours";

describe("working hours form (TEC-326)", () => {
  it("reads full, short and ISO weekday keys", () => {
    const form = workingHoursFromApi({
      monday: [{ start: "09:00", end: "18:00" }],
      tue: [{ start: "10:00", end: "16:00" }],
      "6": [{ start: "09:00", end: "13:00" }],
    });
    expect(form.monday).toEqual({ open: true, start: "09:00", end: "18:00" });
    expect(form.tuesday).toEqual({ open: true, start: "10:00", end: "16:00" });
    expect(form.saturday).toEqual({ open: true, start: "09:00", end: "13:00" });
    expect(form.sunday.open).toBe(false);
  });

  it("writes only open days with full keys", () => {
    const form = workingHoursFromApi({
      mon: [{ start: "09:00", end: "18:00" }],
    });
    expect(workingHoursToApi(form)).toEqual({
      monday: [{ start: "09:00", end: "18:00" }],
    });
  });

  it("rejects a closing time before or at the opening time", () => {
    const form = workingHoursFromApi({
      monday: [{ start: "09:00", end: "08:00" }],
      tuesday: [{ start: "09:00", end: "09:00" }],
      wednesday: [{ start: "09:00", end: "18:00" }],
      thursday: [{ start: "9", end: "18:00" }],
    });
    expect(validateWorkingHours(form)).toEqual({
      monday: "appointments.settings.close_before_open",
      tuesday: "appointments.settings.close_before_open",
      thursday: "appointments.settings.hours_invalid",
    });
  });
});
