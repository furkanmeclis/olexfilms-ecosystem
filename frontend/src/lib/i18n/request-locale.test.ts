import { describe, expect, it } from "vitest";

import { resolveRequestLocale } from "@/lib/i18n/request-locale";

const jar =
  (cookies: Record<string, string>) =>
  (name: string): string | undefined =>
    cookies[name];

describe("resolveRequestLocale (SSR <html lang dir> from cookies)", () => {
  it("renders Arabic RTL from the NEXT_LOCALE cookie (reload stays RTL)", () => {
    expect(resolveRequestLocale(jar({ NEXT_LOCALE: "ar" }))).toEqual({
      locale: "ar",
      dir: "rtl",
      timeZone: "Europe/Istanbul",
    });
  });

  it("uses the time zone cookie when it is a valid IANA name", () => {
    expect(
      resolveRequestLocale(
        jar({ NEXT_LOCALE: "zh_CN", NEXT_TIMEZONE: "Asia/Dubai" }),
      ),
    ).toEqual({ locale: "zh-CN", dir: "ltr", timeZone: "Asia/Dubai" });
  });

  it("falls back to the defaults for missing or tampered cookies", () => {
    expect(resolveRequestLocale(jar({}))).toEqual({
      locale: "tr",
      dir: "ltr",
      timeZone: "Europe/Istanbul",
    });
    expect(
      resolveRequestLocale(
        jar({ NEXT_LOCALE: "<script>", NEXT_TIMEZONE: "Nowhere/City" }),
      ),
    ).toEqual({ locale: "tr", dir: "ltr", timeZone: "Europe/Istanbul" });
  });
});
