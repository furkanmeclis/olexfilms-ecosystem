import { describe, expect, it } from "vitest";

import {
  EMPTY_VALUE,
  createFormatter,
  formatCurrency,
  formatDate,
  formatDateTime,
  formatNumber,
  formatRelative,
  formatTime,
  isValidTimeZone,
  supportedTimeZones,
} from "@/lib/i18n/format";

const NOON_UTC = "2026-01-01T12:00:00Z";

describe("format: time zone", () => {
  it("shows a UTC 12:00 record as 16:00 for Asia/Dubai (TEC-137 kabul)", () => {
    expect(formatTime(NOON_UTC, { locale: "tr", timeZone: "Asia/Dubai" })).toBe(
      "16:00",
    );
    expect(
      formatDateTime(NOON_UTC, { locale: "tr", timeZone: "Asia/Dubai" }),
    ).toBe("01.01.2026 16:00");
  });

  it("uses the effective zone, not the machine zone", () => {
    expect(formatTime(NOON_UTC, { locale: "tr", timeZone: "UTC" })).toBe(
      "12:00",
    );
    expect(
      formatTime(NOON_UTC, { locale: "tr", timeZone: "America/New_York" }),
    ).toBe("07:00");
    // Default is Europe/Istanbul (UTC+3).
    expect(formatTime(NOON_UTC, { locale: "tr" })).toBe("15:00");
  });

  it("crosses the date line with the zone", () => {
    const lateUtc = "2026-01-01T22:30:00Z";
    expect(formatDate(lateUtc, { locale: "tr", timeZone: "Asia/Dubai" })).toBe(
      "02.01.2026",
    );
    expect(formatDate(lateUtc, { locale: "tr", timeZone: "UTC" })).toBe(
      "01.01.2026",
    );
  });

  it("falls back to the default zone for an unknown name", () => {
    expect(isValidTimeZone("Mars/Olympus")).toBe(false);
    expect(
      formatTime(NOON_UTC, { locale: "tr", timeZone: "Mars/Olympus" }),
    ).toBe("15:00");
  });

  it("returns a placeholder for missing or invalid dates", () => {
    const ctx = { locale: "tr" as const };
    expect(formatDateTime(null, ctx)).toBe(EMPTY_VALUE);
    expect(formatDateTime("not-a-date", ctx)).toBe(EMPTY_VALUE);
  });

  it("follows each language's conventions", () => {
    const dubai = { timeZone: "Asia/Dubai" };
    expect(formatTime(NOON_UTC, { locale: "de", ...dubai })).toBe("16:00");
    expect(formatTime(NOON_UTC, { locale: "en", ...dubai })).toMatch(
      /^04:00\sPM$/,
    );
    expect(formatDate(NOON_UTC, { locale: "de", ...dubai })).toBe("01.01.2026");
  });

  it("lists IANA zones for the picker", () => {
    const zones = supportedTimeZones();
    expect(zones).toContain("Asia/Dubai");
    expect(zones).toContain("Europe/Istanbul");
    expect(zones).toContain("UTC");
  });
});

describe("format: numbers and money", () => {
  it("formats numbers per locale", () => {
    expect(formatNumber(1234567.5, { locale: "tr" })).toBe("1.234.567,5");
    expect(formatNumber(1234567.5, { locale: "en" })).toBe("1,234,567.5");
    expect(formatNumber(undefined, { locale: "en" })).toBe(EMPTY_VALUE);
  });

  it("formats the organization currency", () => {
    expect(formatCurrency(1500, "TRY", { locale: "tr" })).toBe("₺1.500,00");
    expect(formatCurrency(1500, "eur", { locale: "de" })).toBe("1.500,00 €");
    expect(formatCurrency(1500, "AED", { locale: "en" })).toMatch(
      /^AED\s1,500\.00$/,
    );
  });

  it("does not throw on an invalid currency code", () => {
    expect(formatCurrency(10, "XXXX", { locale: "en" })).toBe("10.00 XXXX");
  });
});

describe("format: relative and bound formatter", () => {
  it("formats relative times", () => {
    const now = "2026-01-01T12:00:00Z";
    expect(formatRelative("2026-01-01T09:00:00Z", { locale: "en" }, now)).toBe(
      "3 hours ago",
    );
    expect(formatRelative("2026-01-01T09:00:00Z", { locale: "tr" }, now)).toBe(
      "3 saat önce",
    );
  });

  it("binds locale and zone once", () => {
    const f = createFormatter({ locale: "tr", timeZone: "Asia/Dubai" });
    expect(f.timeZone).toBe("Asia/Dubai");
    expect(f.time(NOON_UTC)).toBe("16:00");
    expect(f.currency(2, "USD")).toBe("$2,00");
  });
});
