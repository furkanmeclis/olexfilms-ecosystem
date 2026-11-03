/**
 * Why the review cannot be sent yet, told above its button. Pure (no React)
 * so node:test runs it.
 */
export type ReviewMissing = 'ratings' | 'questions' | null;

export function reviewMissing(v: { platform: number; product: number; requiredUnanswered: number }): ReviewMissing {
  // The two star questions are always required (platform_rating, product_rating).
  if (v.platform < 1 || v.product < 1) return 'ratings';
  if (v.requiredUnanswered > 0) return 'questions';
  return null;
}
