import type { EntityKind } from "@/config/routes";
import { ENTRY_SOURCE_TYPES } from "@/features/accounting/services/accounting.service";

/**
 * TEC-1218: ledger `source_type` → the record kind `source_uuid` names
 * (backend posting sources). Types whose uuid points to a record without a
 * tenant page (manual rows, staff payments, cari transfers, subscription
 * periods, product sales, purchases, cash/bank openings) are left out and
 * render as plain text.
 */
export const ENTRY_SOURCE_KINDS: Partial<Record<string, EntityKind>> = {
  order: "order",
  service: "service",
  service_income: "service",
  transfer: "transfer",
  stock_transfer: "transfer",
  stock_return: "transfer",
  warranty_claim: "warranty_claim",
  opening_balance: "cari",
};

/** Record kind of a ledger source, or null when it has no detail page. */
export function entrySourceKind(
  sourceType: string | null | undefined,
): EntityKind | null {
  if (!sourceType) return null;
  return ENTRY_SOURCE_KINDS[sourceType] ?? null;
}

/** i18n key of a known source type, or null (render the raw value). */
export function entrySourceLabelKey(
  sourceType: string | null | undefined,
): string | null {
  if (!sourceType) return null;
  return (ENTRY_SOURCE_TYPES as readonly string[]).includes(sourceType)
    ? `accounting.sources.${sourceType}`
    : null;
}

/** First block of a uuid, used as a short record label. */
export function shortId(uuid: string | null | undefined): string {
  return uuid ? uuid.split("-")[0] : "";
}
