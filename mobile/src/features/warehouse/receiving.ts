import type { StockEntry, StockEntryArrivalReport, StockEntryDecision } from './api';

/** A damaged line of the receiving decision (TEC-1225). */
export type DamagedChoice = {
  barcode: string;
  decision: 'accepted' | 'excluded';
  reason: string;
  attachmentUuids: string[];
};

export type ExtraChoice = 'added' | 'rejected';

/** The arrival check applies to a container entry or once the arrival scan was used (mirror of the backend). */
export function arrivalChecked(entry: StockEntry, report?: StockEntryArrivalReport | null): boolean {
  if (entry.container_no) return true;
  if ((entry.scanned_count ?? 0) > 0) return true;
  if ((entry.lines ?? []).some((l) => Boolean(l.scanned_at))) return true;
  return (report?.extras.length ?? 0) > 0;
}

/** Extras the backend does not know: they can only be rejected. */
export function unknownExtras(report?: StockEntryArrivalReport | null) {
  return new Set((report?.extras ?? []).filter((x) => !x.product).map((x) => x.barcode));
}

/**
 * Confirm decisions: missing lines (not damaged) are left out, extras take
 * their choice, damaged lines their decision. `pending` lists what still
 * needs a choice, a reason or a photo.
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
        kind: 'missing',
        decision: 'left_out',
      });
    }
  }
  for (const x of report?.extras ?? []) {
    const choice = extras[x.barcode];
    if (!choice) {
      pending.push(x.barcode);
      continue;
    }
    decisions.push({ barcode: x.barcode, kind: 'extra', decision: choice });
  }
  for (const d of damaged) {
    if (!d.reason.trim() || d.attachmentUuids.length === 0) {
      pending.push(d.barcode);
      continue;
    }
    decisions.push({
      barcode: d.barcode,
      kind: 'damaged',
      decision: d.decision,
      reason: d.reason.trim(),
      attachment_uuids: d.attachmentUuids,
    });
  }
  return { decisions, pending };
}

/** YYYY-MM-DD, a real calendar day. */
export function isIsoDate(v: string) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(v)) return false;
  const d = new Date(`${v}T00:00:00Z`);
  return !Number.isNaN(d.getTime()) && d.toISOString().slice(0, 10) === v;
}

/** Timeline step details as a short suffix ("ABC-1 · ×4 · A-01"). */
export function timelineDetail(details: Record<string, unknown> | undefined | null, kindLabel: (k: string) => string): string {
  if (!details) return '';
  const str = (k: string) => (typeof details[k] === 'string' || typeof details[k] === 'number' ? String(details[k]) : '');
  const parts: string[] = [];
  if (str('barcode')) parts.push(str('barcode'));
  if (str('count') && Number(details.count) > 1) parts.push(`×${str('count')}`);
  if (str('location')) parts.push(str('location'));
  if (str('kind')) parts.push(kindLabel(str('kind')));
  if (str('filename')) parts.push(str('filename'));
  if (str('reason')) parts.push(str('reason'));
  return parts.join(' · ');
}
