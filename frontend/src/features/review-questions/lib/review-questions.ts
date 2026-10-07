import type { ReviewQuestion } from "@/features/review-questions/services/review-questions.service";

/** Gap between two consecutive sort orders after a drag-and-drop. */
export const SORT_STEP = 10;

/**
 * Sort order PATCHes for a dragged list: row i gets (i + 1) * SORT_STEP;
 * only rows whose order changes are sent.
 */
export function reorderPatches(
  rows: readonly ReviewQuestion[],
): { uuid: string; sort_order: number }[] {
  const out: { uuid: string; sort_order: number }[] = [];
  rows.forEach((row, index) => {
    const sortOrder = (index + 1) * SORT_STEP;
    if (row.sort_order !== sortOrder) {
      out.push({ uuid: row.uuid, sort_order: sortOrder });
    }
  });
  return out;
}

/** Next free sort order for a new question (appended at the end). */
export function nextSortOrder(rows: readonly ReviewQuestion[]): number {
  const max = rows.reduce((acc, row) => Math.max(acc, row.sort_order), 0);
  return Math.ceil((max + 1) / SORT_STEP) * SORT_STEP;
}

/** Question text in the UI language, else Turkish, English, any; else the key. */
export function questionText(question: ReviewQuestion, locale: string): string {
  const byLocale = new Map(
    question.locales.map((loc) => [loc.locale, loc.text]),
  );
  return (
    byLocale.get(locale) ??
    byLocale.get("tr") ??
    byLocale.get("en") ??
    question.locales[0]?.text ??
    question.question_key
  );
}

/**
 * Locale texts to PUT after saving the form: non-empty (trimmed) texts that
 * differ from the stored ones. The API has no locale delete, so an emptied
 * tab keeps its stored text.
 */
export function changedLocales(
  stored: ReviewQuestion["locales"],
  texts: Record<string, string>,
): { locale: string; text: string }[] {
  const current = new Map(stored.map((loc) => [loc.locale, loc.text]));
  return Object.entries(texts)
    .map(([locale, text]) => ({ locale, text: text.trim() }))
    .filter(({ locale, text }) => text && current.get(locale) !== text);
}
