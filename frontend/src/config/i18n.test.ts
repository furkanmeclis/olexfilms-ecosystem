import { describe, expect, it } from "vitest";

import {
  LOCALE_NAMES,
  SUPPORTED_LOCALES,
  i18nConfig,
  localeDir,
  normalizeLocale,
} from "@/config/i18n";

describe("i18n config", () => {
  it("supports the 13 K10 languages with zh-CN in BCP-47 form", () => {
    expect(SUPPORTED_LOCALES).toEqual([
      "tr",
      "en",
      "bg",
      "de",
      "el",
      "uk",
      "ru",
      "fr",
      "es",
      "it",
      "zh-CN",
      "az",
      "ar",
    ]);
    for (const l of SUPPORTED_LOCALES) expect(LOCALE_NAMES[l]).toBeTruthy();
  });

  it("is RTL for Arabic only", () => {
    expect(i18nConfig.rtlLocales).toEqual(["ar"]);
    expect(SUPPORTED_LOCALES.filter((l) => localeDir(l) === "rtl")).toEqual([
      "ar",
    ]);
  });

  it("normalizes spellings like the backend", () => {
    expect(normalizeLocale("zh_CN")).toBe("zh-CN");
    expect(normalizeLocale("zh-cn")).toBe("zh-CN");
    expect(normalizeLocale("zh")).toBe("zh-CN");
    expect(normalizeLocale("tr-TR")).toBe("tr");
    expect(normalizeLocale(" AR-ae ")).toBe("ar");
    expect(normalizeLocale("ja")).toBeNull();
    expect(normalizeLocale("")).toBeNull();
    expect(normalizeLocale(undefined)).toBeNull();
  });
});
