import { describe, expect, it } from "vitest";

import type { ReviewQuestion } from "@/features/review-questions/services/review-questions.service";

import {
  changedLocales,
  nextSortOrder,
  questionText,
  reorderPatches,
} from "./review-questions";

const at = "2026-10-01T00:00:00Z";

function q(
  uuid: string,
  sort: number,
  locales: [string, string][] = [],
): ReviewQuestion {
  return {
    uuid,
    question_key: `key_${uuid}`,
    question_type: "rating_1_5",
    target: "platform",
    is_required: false,
    is_active: true,
    sort_order: sort,
    locales: locales.map(([locale, text]) => ({
      locale,
      text,
      updated_at: at,
    })),
    created_at: at,
    updated_at: at,
  };
}

describe("review question helpers", () => {
  it("renumbers a dragged list in steps of 10, moved rows only", () => {
    expect(reorderPatches([q("b", 20), q("a", 10), q("c", 30)])).toEqual([
      { uuid: "b", sort_order: 10 },
      { uuid: "a", sort_order: 20 },
    ]);
    expect(reorderPatches([q("a", 10), q("b", 20)])).toEqual([]);
  });

  it("appends a new question after the largest sort order", () => {
    expect(nextSortOrder([])).toBe(10);
    expect(nextSortOrder([q("a", 10), q("b", 25)])).toBe(30);
  });

  it("picks the UI language, then Turkish, English, any, else the key", () => {
    const both = q("a", 10, [
      ["en", "Staff?"],
      ["tr", "Personel?"],
    ]);
    expect(questionText(both, "de")).toBe("Personel?");
    expect(questionText(both, "en")).toBe("Staff?");
    expect(questionText(q("b", 10, [["ar", "؟"]]), "de")).toBe("؟");
    expect(questionText(q("c", 10), "tr")).toBe("key_c");
  });

  it("PUTs only non-empty texts that changed", () => {
    const stored = q("a", 10, [
      ["tr", "Personel?"],
      ["en", "Staff?"],
    ]).locales;
    expect(
      changedLocales(stored, { tr: " Personel? ", en: "Team?", de: "  " }),
    ).toEqual([{ locale: "en", text: "Team?" }]);
  });
});
