import { describe, expect, it } from "vitest";

import {
  LATIN_ONLY,
  intlLocale,
  localizeDigits,
  toLatinDigits,
} from "@/lib/i18n/digits";
import {
  createFormatter,
  formatDate,
  formatNumber,
  formatPercent,
} from "@/lib/i18n/format";

describe("Arabic digits (TEC-1009)", () => {
  it("formats Arabic with Arabic-Indic digits by default", () => {
    const ctx = { locale: "ar" as const, timeZone: "UTC" };
    expect(formatNumber(1234.5, ctx)).toBe("١٬٢٣٤٫٥");
    expect(formatPercent(0.5, ctx)).toMatch(/٥٠/);
    expect(formatDate("2026-10-26T12:00:00Z", ctx, "short")).toMatch(/٢٠٢٦/);
    const f = createFormatter(ctx);
    expect(f.digits).toBe("arab");
    expect(f.intlLocale).toBe("ar-u-nu-arab");
    expect(f.money(12)).toBe("١٢٫٠٠");
  });

  it("uses Latin digits when the user picked them", () => {
    const ctx = {
      locale: "ar" as const,
      timeZone: "UTC",
      digits: "latn" as const,
    };
    expect(formatNumber(1234, ctx)).toBe("1,234");
    expect(createFormatter(ctx).intlLocale).toBe("ar-u-nu-latn");
  });

  it("leaves the other languages alone", () => {
    expect(intlLocale("tr")).toBe("tr");
    expect(formatNumber(1234, { locale: "tr" })).toBe("1.234");
  });

  it("normalizes typed Arabic-Indic and Persian digits to Latin", () => {
    expect(toLatinDigits("٣٤ ABC ١٢")).toBe("34 ABC 12");
    expect(toLatinDigits("۱۲۳٫۵")).toBe("123٫5");
    expect(toLatinDigits("WBA0123")).toBe("WBA0123");
    expect(toLatinDigits("MH 01 AB १२३४")).toBe("MH 01 AB 1234");
  });

  it("keeps Latin digits with Indian grouping for Hindi (TEC-1248)", () => {
    expect(intlLocale("hi")).toBe("hi-IN-u-nu-latn");
    expect(formatNumber(1234567.5, { locale: "hi" })).toMatch(
      /^12,34,567(\.5)?$/,
    );
  });

  it("localizes hand-built text only for Arabic digits", () => {
    expect(localizeDigits("3 / 5", "ar")).toBe("٣ / ٥");
    expect(localizeDigits("3 / 5", "ar", "latn")).toBe("3 / 5");
    expect(localizeDigits("3 / 5", "tr")).toBe("3 / 5");
  });

  it("lists the identifiers that keep Latin digits", () => {
    expect([...LATIN_ONLY]).toEqual([
      "plate",
      "vin",
      "barcode",
      "service_no",
      "phone",
      "code",
      "id",
      "url",
    ]);
  });
});
