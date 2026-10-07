import type { FilterOption } from "@/features/ai-admin/components/usage-columns";

/**
 * Merges filter options by value (first label wins), sorted by label.
 * The usage API has no user / model directory, so options come from the
 * rows seen so far plus any directory the caller has.
 */
export function mergeOptions(...lists: FilterOption[][]): FilterOption[] {
  const byValue = new Map<string, FilterOption>();
  for (const list of lists) {
    for (const option of list) {
      if (option.value && !byValue.has(option.value)) {
        byValue.set(option.value, option);
      }
    }
  }
  return [...byValue.values()].sort((a, b) => a.label.localeCompare(b.label));
}
