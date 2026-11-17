import type {
  StockEntry,
  StockEntryArrivalReport,
  StockEntryDecision,
} from "@/features/warehouse/services/warehouse.service";

type T = (key: string, params?: Record<string, string | number>) => string;

/** A damaged line of the receiving decision (TEC-1225). */
export type DamagedChoice = {
  barcode: string;
  decision: "accepted" | "excluded";
  reason: string;
  attachmentUuids: string[];
};

/** Extra barcode decision. */
export type ExtraChoice = "added" | "rejected";

/**
 * Whether the arrival check applies at confirm (mirror of the backend): a
 * container entry, or the arrival scan was used.
 */
export function arrivalChecked(
  entry: StockEntry,
  report?: StockEntryArrivalReport | null,
): boolean {
  if (entry.container_no) return true;
  if ((entry.scanned_count ?? 0) > 0) return true;
  if ((entry.lines ?? []).some((l) => Boolean(l.scanned_at))) return true;
  return (report?.extras.length ?? 0) > 0;
}

/** Barcodes of the extras that the backend does not know (only "rejected"). */
export function unknownExtras(report?: StockEntryArrivalReport | null) {
  return new Set(
    (report?.extras ?? []).filter((x) => !x.product).map((x) => x.barcode),
  );
}

/**
 * The confirm decisions: every missing line (not damaged) is left out,
 * every extra takes its choice, every damaged line its decision. `pending`
 * lists what still needs a choice or is incomplete (damage reason/photo).
 */
export function buildDecisions(
  report: StockEntryArrivalReport | null | undefined,
  checked: boolean,
  extras: Record<string, ExtraChoice | undefined>,
  damaged: DamagedChoice[],
): { decisions: StockEntryDecision[]; pending: string[] } {
  const decisions: StockEntryDecision[] = [];
  const pending: string[] = [];
  const damagedSet = new Set(damaged.map((d) => d.barcode.toUpperCase()));
  if (checked) {
    for (const line of report?.missing ?? []) {
      if (damagedSet.has(line.barcode.toUpperCase())) continue;
      decisions.push({
        barcode: line.barcode,
        kind: "missing",
        decision: "left_out",
      });
    }
  }
  for (const x of report?.extras ?? []) {
    const choice = extras[x.barcode];
    if (!choice) {
      pending.push(x.barcode);
      continue;
    }
    decisions.push({ barcode: x.barcode, kind: "extra", decision: choice });
  }
  for (const d of damaged) {
    if (!d.reason.trim() || d.attachmentUuids.length === 0) {
      pending.push(d.barcode);
      continue;
    }
    decisions.push({
      barcode: d.barcode,
      kind: "damaged",
      decision: d.decision,
      reason: d.reason.trim(),
      attachment_uuids: d.attachmentUuids,
    });
  }
  return { decisions, pending };
}

/** Details of a timeline step as a short suffix ("ABC-1 · ×4 · A-01-02"). */
export function timelineDetail(
  details: Record<string, unknown> | undefined | null,
  t: T,
): string {
  if (!details) return "";
  const parts: string[] = [];
  const str = (k: string) =>
    typeof details[k] === "string" || typeof details[k] === "number"
      ? String(details[k])
      : "";
  if (str("barcode")) parts.push(str("barcode"));
  if (str("count") && Number(details.count) > 1) parts.push(`×${str("count")}`);
  if (str("location")) parts.push(str("location"));
  if (str("kind")) {
    const key = `warehouse.receiving.attachment_kind.${str("kind")}`;
    const label = t(key);
    parts.push(label === key ? str("kind") : label);
  }
  if (str("filename")) parts.push(str("filename"));
  if (str("reason")) parts.push(str("reason"));
  return parts.join(" · ");
}

/** Translated label of a movement type that blocks an undo. */
export function movementLabel(type: string, t: T): string {
  const key = `warehouse.receiving.movement.${type}`;
  const label = t(key);
  return label === key ? type : label;
}
