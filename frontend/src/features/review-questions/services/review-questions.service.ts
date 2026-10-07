import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

/** TEC-353: admin review question (platform "Değerlendirme soruları"). */
export type ReviewQuestion = components["schemas"]["ReviewQuestion"];
export type ReviewQuestionInput = components["schemas"]["ReviewQuestionInput"];
export type ReviewQuestionLocale =
  components["schemas"]["ReviewQuestionLocale"];
export type ReviewQuestionType = components["schemas"]["ReviewQuestionType"];
export type ReviewQuestionTarget =
  components["schemas"]["ReviewQuestionTarget"];

export const REVIEW_QUESTION_TYPES: readonly ReviewQuestionType[] = [
  "rating_1_5",
  "text",
];
export const REVIEW_QUESTION_TARGETS: readonly ReviewQuestionTarget[] = [
  "platform",
  "dealer",
  "product",
];

export const TYPE_LABEL_KEYS: Record<ReviewQuestionType, string> = {
  rating_1_5: "services.review_questions.type.rating_1_5",
  text: "services.review_questions.type.text",
};
export const TARGET_LABEL_KEYS: Record<ReviewQuestionTarget, string> = {
  platform: "services.review_questions.target.platform",
  dealer: "services.review_questions.target.dealer",
  product: "services.review_questions.target.product",
};

/** Longest question text the API accepts per language (characters). */
export const REVIEW_QUESTION_TEXT_MAX = 500;

const enc = encodeURIComponent;

/** /v1/platform/review-questions (reviews.questions.manage). */
export const reviewQuestionsService = {
  list() {
    return platformRequest<{ items: ReviewQuestion[] }>(
      "GET",
      "/v1/platform/review-questions",
    );
  },
  create(body: ReviewQuestionInput) {
    return platformRequest<ReviewQuestion>(
      "POST",
      "/v1/platform/review-questions",
      { body },
    );
  },
  update(uuid: string, body: ReviewQuestionInput) {
    return platformRequest<ReviewQuestion>(
      "PATCH",
      `/v1/platform/review-questions/${enc(uuid)}`,
      { body },
    );
  },
  putLocale(uuid: string, locale: string, text: string) {
    return platformRequest<ReviewQuestionLocale>(
      "PUT",
      `/v1/platform/review-questions/${enc(uuid)}/locales/${enc(locale)}`,
      { body: { text } },
    );
  },
};
