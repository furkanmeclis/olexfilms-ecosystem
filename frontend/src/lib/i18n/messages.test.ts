import { describe, expect, it } from "vitest";

import { SUPPORTED_LOCALES } from "@/config/i18n";
import { catalogLoaders } from "@/lib/i18n/catalog-loaders";
import {
  isLocaleLoaded,
  loadMessages,
  pluralCategory,
  registerMessages,
  translate,
  translatePlural,
} from "@/lib/i18n/messages";

describe("messages", () => {
  it("has a lazy loader for every supported language", () => {
    expect(Object.keys(catalogLoaders).sort()).toEqual(
      [...SUPPORTED_LOCALES].sort(),
    );
  });

  it("bundles en only; other languages load on demand", async () => {
    expect(isLocaleLoaded("en")).toBe(true);
    expect(isLocaleLoaded("tr")).toBe(false);
    const tr = await loadMessages("tr");
    expect(isLocaleLoaded("tr")).toBe(true);
    expect(tr.common).toBeDefined();
    expect(translate("tr", "common.save")).not.toBe(
      translate("en", "common.save"),
    );
  });

  it("falls back to en for a language without translations yet", async () => {
    await loadMessages("ar");
    expect(translate("ar", "common.save")).toBe(translate("en", "common.save"));
    // Not loaded at all: still en, never the raw key.
    expect(translate("ru", "common.save")).toBe(translate("en", "common.save"));
  });

  it("returns the key when no language has it", () => {
    expect(translate("en", "common.__nope__")).toBe("common.__nope__");
  });

  it("interpolates {{params}}", () => {
    registerMessages("bg", { test: { greet: "Здравей, {{ name }}!" } });
    expect(translate("bg", "test.greet", { name: "Ali" })).toBe(
      "Здравей, Ali!",
    );
  });

  it("uses CLDR plural categories (Intl.PluralRules)", () => {
    expect(pluralCategory("en", 1)).toBe("one");
    expect(pluralCategory("ru", 3)).toBe("few");
    expect(pluralCategory("ru", 5)).toBe("many");
    expect(pluralCategory("ar", 0)).toBe("zero");
    expect(pluralCategory("ar", 2)).toBe("two");
    expect(pluralCategory("ar", 11)).toBe("many");
  });

  it("picks the plural form of the language, then falls back", () => {
    registerMessages("uk", {
      test: {
        files_one: "{{count}} файл",
        files_few: "{{count}} файли",
        files_many: "{{count}} файлів",
        files_other: "{{count}} файлу",
      },
    });
    expect(translatePlural("uk", "test.files", 1)).toBe("1 файл");
    expect(translatePlural("uk", "test.files", 3)).toBe("3 файли");
    expect(translatePlural("uk", "test.files", 5)).toBe("5 файлів");
    // A language without the key uses en's own category.
    registerMessages("el", { test: {} });
    expect(translatePlural("el", "test.files", 2)).toBe("test.files");
  });
});
