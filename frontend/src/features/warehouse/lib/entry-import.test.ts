import { describe, expect, it } from "vitest";

import {
  importPlan,
  importStatusCounts,
} from "@/features/io/lib/import-summary";
import type { ImportPreviewSummary } from "@/features/io/types";
import { entryImportConfirmCopy } from "@/features/warehouse/lib/entry-import";
import en from "@/locales/en/warehouse.json";
import tr from "@/locales/tr/warehouse.json";

const summary: ImportPreviewSummary = {
  total: 8,
  valid: 5,
  invalid: 3,
  counts: { link: 3, new: 2, placed: 4, duplicate: 2, conflict: 1 },
  rows: [],
  errors: [],
};

function translator(catalog: Record<string, string>) {
  return (key: string, params?: Record<string, string | number>) =>
    (catalog[key.replace(/^warehouse\./, "")] ?? key).replace(
      /{{(\w+)}}/g,
      (_, name: string) => String(params?.[name] ?? ""),
    );
}

describe("entry barcode import confirmation", () => {
  it("reports linked, new, placed and skipped rows in both primary languages", () => {
    expect(importPlan(summary).importable).toBe(5);
    expect(
      importStatusCounts(summary).map(({ status }) => status),
    ).not.toContain("placed");
    const stubPlural = () => "";
    const enCopy = entryImportConfirmCopy({
      summary,
      t: translator(en),
      tp: stubPlural,
    });
    expect(enCopy.sentence).toContain("3 barcodes will be linked");
    expect(enCopy.sentence).toContain("2 new units");
    expect(enCopy.sentence).toContain("4 placed in bins");
    expect(enCopy.sentence).toContain("3 rows will be skipped");
    expect(enCopy.action).toContain("5 matching rows");
    const trCopy = entryImportConfirmCopy({
      summary,
      t: translator(tr),
      tp: stubPlural,
    });
    expect(trCopy.sentence).toContain("3 barkod bağlanacak");
    expect(trCopy.action).toContain("5 satırı içeri al");
  });
});
