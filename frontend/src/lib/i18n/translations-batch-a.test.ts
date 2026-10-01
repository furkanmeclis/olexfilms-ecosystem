import { describe, expect, it } from "vitest";

import type { AppLocale } from "@/config/i18n";
import { loadMessages, translate, translatePlural } from "@/lib/i18n/messages";

// TEC-138 batch A: these languages ship full translations.
const BATCH_A: AppLocale[] = ["de", "fr", "es", "it", "ru", "uk"];

describe("translations batch A", () => {
  it.each(BATCH_A)("%s loads its own copy instead of en", async (locale) => {
    await loadMessages(locale);
    for (const key of ["common.save", "auth.login.title", "layout.nav_users"]) {
      expect(translate(locale, key)).not.toBe(translate("en", key));
    }
    expect(
      translate(locale, "table.showing", { from: 1, to: 10, total: 42 }),
    ).toContain("42");
  });

  it("uses Slavic plural forms for ru and uk", async () => {
    await loadMessages("ru");
    await loadMessages("uk");
    expect(translatePlural("ru", "common.items", 1)).toBe("1 запись");
    expect(translatePlural("ru", "common.items", 3)).toBe("3 записи");
    expect(translatePlural("ru", "common.items", 5)).toBe("5 записей");
    expect(translatePlural("ru", "common.items", 21)).toBe("21 запись");
    expect(translatePlural("uk", "common.items", 2)).toBe("2 записи");
    expect(translatePlural("uk", "common.items", 11)).toBe("11 записів");
  });
});
