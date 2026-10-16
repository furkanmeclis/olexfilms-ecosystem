import { Permission } from "@/config/permissions";
import type {
  StockEntry,
  StockEntryLine,
  StockEntryMode,
  StockEntryStatus,
} from "@/features/warehouse/services/warehouse.service";

export const ENTRY_STATUSES: StockEntryStatus[] = [
  "draft",
  "confirmed",
  "cancelled",
  "undone",
];

/** Entry modes (TEC-204); import entries come from the stock import. */
export const ENTRY_MODES: StockEntryMode[] = [
  "with_existing",
  "generate_new",
  "import",
];

export function entryStatusTone(
  status: StockEntryStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "confirmed":
      return "success";
    case "draft":
      return "warning";
    case "cancelled":
    case "undone":
      return "danger";
    default:
      return "default";
  }
}

/** Lines still waiting for a location. */
export function unplacedLines(entry: StockEntry): StockEntryLine[] {
  return (entry.lines ?? []).filter((l) => !l.location);
}

/**
 * Lines a "place" sends: the selected ones, else every unplaced line (so a
 * placed line keeps its location), else nothing (the server then moves
 * every line).
 */
export function linesToPlace(entry: StockEntry, selected: string[]): string[] {
  if (selected.length > 0) return selected;
  return unplacedLines(entry).map((l) => l.uuid);
}

/** Confirm is possible once every line has a location. */
export function canConfirm(entry: StockEntry): boolean {
  const lines = entry.lines ?? [];
  return (
    entry.status === "draft" &&
    lines.length > 0 &&
    lines.every((l) => Boolean(l.location))
  );
}

/**
 * Stock import from Excel (TEC-759): center only (K14) with stock.import,
 * the stock module on and tenant.imports.read (the wizard previews,
 * confirms and undoes on /v1/tenant/imports).
 */
export function canImportStock(
  access: { isCenter: boolean; can: (permission: string) => boolean },
  stockModule: boolean,
): boolean {
  return (
    access.isCenter &&
    stockModule &&
    access.can(Permission.StockImport) &&
    access.can(Permission.TenantImportsRead)
  );
}

/**
 * Lines a partial import undo left in stock (they moved after the import),
 * or null when the import entry was not partially undone.
 */
export function importLinesKept(entry: StockEntry): number | null {
  if (entry.mode !== "import" || entry.import_status !== "partially_undone") {
    return null;
  }
  return (entry.lines ?? []).filter((l) => !l.undone).length;
}
