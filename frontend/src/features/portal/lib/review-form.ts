import type { components } from "@/generated/api";

export type ReviewFormQuestion = components["schemas"]["ReviewFormQuestion"];
export type ReviewProduct = components["schemas"]["ReviewProduct"];
export type ReviewAnswerInput = components["schemas"]["ReviewAnswerInput"];

/** One answer row of the portal form: a question, per product when targeted. */
export type ReviewFormRow = {
  key: string;
  question: ReviewFormQuestion;
  product: ReviewProduct | null;
};

/** Answer values by row key: a 1-5 rating or a text. */
export type ReviewFormValues = Record<string, number | string>;

/**
 * Rows in question order; a `product` question repeats for each product of
 * the service (none when the service has no product).
 */
export function reviewFormRows(
  questions: readonly ReviewFormQuestion[],
  products: readonly ReviewProduct[],
): ReviewFormRow[] {
  const sorted = [...questions].sort((a, b) => a.sort_order - b.sort_order);
  const rows: ReviewFormRow[] = [];
  for (const question of sorted) {
    if (question.target === "product") {
      for (const product of products) {
        rows.push({
          key: `${question.uuid}:${product.uuid}`,
          question,
          product,
        });
      }
    } else {
      rows.push({ key: question.uuid, question, product: null });
    }
  }
  return rows;
}

function answered(row: ReviewFormRow, value: number | string | undefined) {
  if (row.question.question_type === "rating_1_5") {
    return typeof value === "number" && value >= 1 && value <= 5;
  }
  return typeof value === "string" && value.trim() !== "";
}

/** True while a required row has no answer. */
export function missingRequired(
  rows: readonly ReviewFormRow[],
  values: ReviewFormValues,
): boolean {
  return rows.some(
    (row) => row.question.is_required && !answered(row, values[row.key]),
  );
}

/** API answers: answered rows only (rating or trimmed text). */
export function reviewAnswers(
  rows: readonly ReviewFormRow[],
  values: ReviewFormValues,
): ReviewAnswerInput[] {
  const out: ReviewAnswerInput[] = [];
  for (const row of rows) {
    const value = values[row.key];
    if (!answered(row, value)) continue;
    const answer: ReviewAnswerInput = { question_uuid: row.question.uuid };
    if (row.product) answer.product_uuid = row.product.uuid;
    if (row.question.question_type === "rating_1_5") {
      answer.rating = value as number;
    } else {
      answer.text = (value as string).trim();
    }
    out.push(answer);
  }
  return out;
}
