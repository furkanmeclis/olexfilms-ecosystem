import type {
  StockEntry,
  StockEntryLine,
  StockEntryStatus,
} from "@/features/warehouse/services/warehouse.service";

export const ENTRY_STATUSES: StockEntryStatus[] = [
  "draft",
  "confirmed",
  "cancelled",
  "undone",
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
