import { describe, expect, it } from "vitest";

import {
  localeFromAcceptLanguage,
  resolveRequestLocale,
} from "@/lib/i18n/request-locale";

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

describe("Accept-Language fallback (TEC-142)", () => {
  it.each([
    ["ar", "ar"],
    ["de-AT,de;q=0.9,en;q=0.8", "de"],
    ["ja,fr-CA;q=0.7,en;q=0.5", "fr"],
    ["en;q=0.4, ru;q=0.9", "ru"],
    ["es;q=0.8, it;q=0.8", "es"],
    ["zh_CN", "zh-CN"],
    ["zh-Hans-CN;q=0.9", "zh-CN"],
    ["AZ", "az"],
    ["uk-UA ; q=0.9 , *;q=0.1", "uk"],
    ["ar;q=0, el", "el"],
    ["*", null],
    ["ja,ko;q=0.9", null],
    ["", null],
    ["en;q=abc, bg", "bg"],
  ])("%s -> %s", (header, want) => {
    expect(localeFromAcceptLanguage(header)).toBe(want);
  });

  it("uses Accept-Language only without a valid locale cookie", () => {
    expect(resolveRequestLocale(jar({}), "ar,en;q=0.5")).toEqual({
      locale: "ar",
      dir: "rtl",
      timeZone: "Europe/Istanbul",
    });
    expect(resolveRequestLocale(jar({ NEXT_LOCALE: "de" }), "ar")).toEqual({
      locale: "de",
      dir: "ltr",
      timeZone: "Europe/Istanbul",
    });
    expect(
      resolveRequestLocale(jar({ NEXT_LOCALE: "<script>" }), "ar").locale,
    ).toBe("ar");
    expect(resolveRequestLocale(jar({}), "ja, ko").locale).toBe("tr");
    expect(resolveRequestLocale(jar({}), null).locale).toBe("tr");
  });
});
