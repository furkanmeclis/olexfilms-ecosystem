import { describe, expect, it } from "vitest";

import {
  nextWindow,
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
    expect(form.monday).toEqual({
      open: true,
      windows: [{ start: "09:00", end: "18:00" }],
    });
    expect(form.tuesday).toEqual({
      open: true,
      windows: [{ start: "10:00", end: "16:00" }],
    });
    expect(form.saturday).toEqual({
      open: true,
      windows: [{ start: "09:00", end: "13:00" }],
    });
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

  it("round-trips every saved window of a day in order", () => {
    const saved = {
      monday: [
        { start: "09:00", end: "12:00" },
        { start: "13:00", end: "18:00" },
      ],
      saturday: [
        { start: "14:00", end: "16:00" },
        { start: "09:00", end: "12:00" },
        { start: "17:00", end: "19:00" },
      ],
    };
    const form = workingHoursFromApi(saved);
    expect(form.monday.windows).toHaveLength(2);
    expect(validateWorkingHours(form)).toEqual({});
    expect(workingHoursToApi(form)).toEqual(saved);
  });

  it("rejects a closing time before or at the opening time", () => {
    const form = workingHoursFromApi({
      monday: [{ start: "09:00", end: "08:00" }],
      tuesday: [{ start: "09:00", end: "09:00" }],
      wednesday: [{ start: "09:00", end: "18:00" }],
      thursday: [{ start: "9", end: "18:00" }],
      friday: [
        { start: "09:00", end: "12:00" },
        { start: "14:00", end: "13:00" },
      ],
    });
    expect(validateWorkingHours(form)).toEqual({
      monday: { windows: { 0: "appointments.settings.close_before_open" } },
      tuesday: { windows: { 0: "appointments.settings.close_before_open" } },
      thursday: { windows: { 0: "appointments.settings.hours_invalid" } },
      friday: { windows: { 1: "appointments.settings.close_before_open" } },
    });
  });

  it("rejects overlapping windows of the same day but allows touching ones", () => {
    const form = workingHoursFromApi({
      monday: [
        { start: "13:00", end: "18:00" },
        { start: "09:00", end: "13:30" },
      ],
      tuesday: [
        { start: "09:00", end: "18:00" },
        { start: "10:00", end: "11:00" },
      ],
      wednesday: [
        { start: "09:00", end: "12:00" },
        { start: "12:00", end: "18:00" },
      ],
    });
    expect(validateWorkingHours(form)).toEqual({
      monday: { day: "appointments.settings.windows_overlap" },
      tuesday: { day: "appointments.settings.windows_overlap" },
    });
  });

  it("ignores the windows of a closed day", () => {
    const form = workingHoursFromApi({});
    form.monday.windows = [{ start: "18:00", end: "09:00" }];
    expect(validateWorkingHours(form)).toEqual({});
    expect(workingHoursToApi(form)).toEqual({});
  });

  it("suggests the next window after the last one", () => {
    expect(nextWindow([{ start: "09:00", end: "12:00" }])).toEqual({
      start: "12:00",
      end: "13:00",
    });
    expect(nextWindow([{ start: "20:00", end: "23:30" }])).toEqual({
      start: "09:00",
      end: "18:00",
    });
  });
});
