import { describe, expect, it } from "vitest";

import {
  missingRequired,
  reviewAnswers,
  reviewFormRows,
  type ReviewFormQuestion,
} from "./review-form";

function question(
  uuid: string,
  patch: Partial<ReviewFormQuestion> = {},
): ReviewFormQuestion {
  return {
    uuid,
    question_key: uuid,
    question_type: "rating_1_5",
    target: "platform",
    is_required: false,
    sort_order: 10,
    text: uuid,
    ...patch,
  };
}

const products = [
  { uuid: "p1", sku: "A", name: "A" },
  { uuid: "p2", sku: "B", name: "B" },
];

describe("review form rows", () => {
  it("repeats product questions per product and skips them without products", () => {
    const qs = [
      question("film", { target: "product", sort_order: 20 }),
      question("staff", { target: "dealer", sort_order: 10 }),
    ];
    expect(reviewFormRows(qs, products).map((r) => r.key)).toEqual([
      "staff",
      "film:p1",
      "film:p2",
    ]);
    expect(reviewFormRows(qs, []).map((r) => r.key)).toEqual(["staff"]);
  });

  it("treats blank text as unanswered", () => {
    const rows = reviewFormRows(
      [question("note", { question_type: "text", is_required: true })],
      [],
    );
    expect(missingRequired(rows, { note: "   " })).toBe(true);
    expect(missingRequired(rows, { note: "ok" })).toBe(false);
    expect(reviewAnswers(rows, { note: "  ok " })).toEqual([
      { question_uuid: "note", text: "ok" },
    ]);
    expect(reviewAnswers(rows, { note: " " })).toEqual([]);
  });
});
