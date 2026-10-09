import type { FeatureListItem, ModuleState } from "@/features/modules/types";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Same cap as the backend (`note` maxLength 1000). */
export const MODULE_REQUEST_NOTE_MAX = 1000;

/** Error key of a request / decision note, or null when it is valid. */
export function requestNoteError(note: string): string | null {
  return note.trim().length > MODULE_REQUEST_NOTE_MAX
    ? "modules.requests.note_too_long"
    : null;
}

/**
 * Price line of a module (TEC-509): on by default → free with no service
 * record; a module bundle on sale → its price and period; paid with nothing
 * on sale → contact us; otherwise (a free add-on the admin switches) free.
 */
export type ModulePriceKind = "free_default" | "paid" | "contact" | "free";

export function modulePriceKind(
  item: Pick<
    FeatureListItem,
    "free_default" | "price" | "contact_for_price" | "paid"
  >,
): ModulePriceKind {
  if (item.free_default) return "free_default";
  if (item.price) return "paid";
  if (item.contact_for_price || item.paid) return "contact";
  return "free";
}

export function modulePriceText(
  t: Translate,
  currency: (amount: number, code: string) => string,
  item: Pick<
    FeatureListItem,
    "free_default" | "price" | "contact_for_price" | "paid"
  >,
): string {
  const kind = modulePriceKind(item);
  if (kind === "paid" && item.price) {
    const price = currency(Number(item.price.amount), item.price.currency);
    return t(`modules.price.paid_${item.price.recurrence}`, { price });
  }
  return t(`modules.price.${kind}`);
}

/**
 * What the Özellikler page offers for a module: nothing for core or open
 * modules, "off at the level above" (no request) when the parent lacks it,
 * withdraw for a pending request, otherwise a request.
 */
export type ModuleRequestAction =
  "none" | "upstream_closed" | "pending" | "request";

export function moduleRequestAction(
  item: Pick<ModuleState, "level" | "enabled" | "upstream_enabled"> & {
    request?: FeatureListItem["request"];
  },
): ModuleRequestAction {
  if (item.level === "core" || item.enabled) return "none";
  if (!item.upstream_enabled) return "upstream_closed";
  if (item.request?.status === "pending") return "pending";
  return "request";
}
