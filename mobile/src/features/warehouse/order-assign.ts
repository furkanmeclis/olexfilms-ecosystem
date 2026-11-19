/**
 * Order preparation rules of the assign scanner, free of React so node:test
 * runs them. No `@/` imports.
 */

export type AssignItem = {
  uuid: string;
  product: { uuid: string; unit_type: string };
  meters?: string | number | null;
  quantity?: number | null;
  assigned: string | number;
  /** TEC-1238: box (quantity) or meter (meters) line. */
  sale_measure?: string | null;
  /** TEC-1238: set on the meter line of a box product (cut pieces carry it). */
  line_credits?: number | null;
};

/** A roll_meter line: a roll is assigned whole or split by the typed meters. */
export const isRollItem = (i: AssignItem) =>
  i.product.unit_type === "roll_meter";
/** A line counted in meters: roll_meter products and meter lines of box products. */
export const isMeterItem = (i: AssignItem) =>
  isRollItem(i) || i.sale_measure === "meter";
/**
 * TEC-1241: the meter line of a box product. It is picked by cutting one
 * piece from an open roll or a closed box (box opening confirmed).
 */
export const isCutItem = (i: AssignItem) =>
  i.sale_measure === "meter" && i.line_credits != null;
export const requiredOf = (i: AssignItem) =>
  Number(isMeterItem(i) ? (i.meters ?? 0) : (i.quantity ?? 0));
export const isCovered = (i: AssignItem) =>
  Number(i.assigned) >= requiredOf(i) - 1e-9;

/** Every line has what it asks for (an order without lines is not complete). */
export function allCovered(items: readonly AssignItem[]): boolean {
  return items.length > 0 && items.every(isCovered);
}

/**
 * Line a scanned unit of `productUuid` goes to: the first open line of that
 * product. `not_on_order` when no line has the product, `covered` when every
 * line of it is already full (nothing is sent then).
 */
export function assignTarget<T extends AssignItem>(
  items: readonly T[],
  productUuid: string | null | undefined,
  opened?: boolean,
):
  | { kind: "line"; item: T }
  | { kind: "not_on_order" }
  | { kind: "covered"; item: T } {
  const lines = items.filter((i) => i.product.uuid === productUuid);
  if (!lines.length) return { kind: "not_on_order" };
  const open = lines.filter((i) => !isCovered(i));
  if (!open.length) return { kind: "covered", item: lines[0] };
  // TEC-1241: one product may have a box and a meter line (K10). An open
  // roll or a cut piece (it has meters) goes to the meter line; a closed
  // box to the box line first, else it is opened for the meter line.
  if (opened !== undefined && open.length > 1) {
    const meter = open.find((i) => i.sale_measure === "meter");
    const box = open.find((i) => i.sale_measure !== "meter");
    const pick = (opened ? meter : box) ?? open[0];
    return { kind: "line", item: pick };
  }
  return { kind: "line", item: open[0] };
}

/** What the cut confirmation tells (TEC-1241), as i18n keys with values. */
export type CutPreviewLike = {
  closed_box: boolean;
  whole: boolean;
  remnant: boolean;
  partial: boolean;
  source_meters: string;
  cut_meters: string;
  remaining_meters: string;
  missing_meters: string;
  min_cut_meters?: string | null;
  /** TEC-1250 (K7): extra meters when the whole roll / box goes. */
  excess_meters?: string | null;
  /** TEC-1250 (19.11): free | charge, the seller's setting for them. */
  excess_billing?: string | null;
};

/** TEC-1250 (K7): the extra meters of a whole roll; 0 when none. */
export function excessOf(v: { excess_meters?: string | null }): number {
  const n = Number(v.excess_meters ?? 0);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

/**
 * TEC-1250 (19.11): the key of an extra meters note — free, or added to
 * the invoice when the seller charges them (frozen on the line).
 */
export function excessKey(
  v: { excess_billing?: string | null },
  free: string,
  charge: string,
): string {
  return v.excess_billing === "charge" ? charge : free;
}

export type CutNote = {
  key: string;
  values: Record<string, string>;
  tone: "warning" | "info";
};

/**
 * The notes of the cut confirmation. A warning needs attention; an info
 * (TEC-1250, K7: the whole roll goes, the extra meters free) is confirmed
 * with the same single step.
 */
export function cutWarnings(p: CutPreviewLike): CutNote[] {
  const out: CutNote[] = [];
  if (p.closed_box) {
    out.push({
      key: p.whole ? "orders.cut.box_whole" : "orders.cut.box_open",
      values: {
        cut: p.cut_meters,
        rest: p.remaining_meters,
        box: p.source_meters,
      },
      tone: "warning",
    });
  }
  if (excessOf(p) > 0) {
    out.push({
      key: excessKey(
        p,
        "orders.cut.whole_excess",
        "orders.cut.whole_excess_charge",
      ),
      values: {
        excess: p.excess_meters ?? "",
        min: p.min_cut_meters ?? "",
        whole: p.source_meters,
      },
      tone: "info",
    });
  }
  if (p.remnant)
    out.push({
      key: "orders.cut.remnant",
      values: { rest: p.remaining_meters, min: p.min_cut_meters ?? "" },
      tone: "warning",
    });
  if (p.partial)
    out.push({
      key: "orders.cut.partial_short",
      values: { cut: p.cut_meters, missing: p.missing_meters },
      tone: "warning",
    });
  return out;
}

/** The confirm button of a preview: cut, or assign the whole roll / box (TEC-1250). */
export function confirmKey(
  p: Pick<CutPreviewLike, "closed_box" | "whole"> | null | undefined,
): string {
  if (p?.closed_box)
    return p.whole ? "orders.cut.confirm_box_whole" : "orders.cut.confirm_box";
  return p?.whole ? "orders.cut.confirm_whole" : "orders.cut.confirm";
}

/** Typed piece meters ("3,5" -> "3.5"); '' when empty or invalid. */
export function pieceMeters(raw: string): string {
  const v = raw.trim().replace(",", ".");
  return /^\d{1,8}(\.\d{1,2})?$/.test(v) && Number(v) > 0 ? v : "";
}

/** Typed piece credits: null when empty, NaN when invalid (0–1000). */
export function pieceCredits(raw: string): number | null {
  const v = raw.trim();
  if (!v) return null;
  return /^\d{1,4}$/.test(v) && Number(v) <= 1000 ? Number(v) : Number.NaN;
}

/**
 * The assign body of a confirmed cut: the box opening, the partial piece
 * (meters, note, credits) and the retry key.
 */
export function cutAssignBody(
  barcode: string,
  p: Pick<CutPreviewLike, "closed_box" | "partial">,
  partial: { meters: string; note: string; credits: string } | null,
  key: string,
): {
  barcode: string;
  idempotency_key: string;
  open_box?: boolean;
  meters?: string;
  partial?: boolean;
  note?: string;
  credits?: number;
} {
  const body: ReturnType<typeof cutAssignBody> = {
    barcode,
    idempotency_key: key,
  };
  if (p.closed_box) body.open_box = true;
  if (partial) {
    const m = pieceMeters(partial.meters);
    if (m) body.meters = m;
    const c = pieceCredits(partial.credits);
    if (c != null && !Number.isNaN(c)) body.credits = c;
  }
  if (p.partial) {
    body.partial = true;
    body.note = partial?.note.trim() ?? "";
  }
  return body;
}

/** A partial shipment rest shorter than the minimum cut (TEC-1241 warning). */
export function restBelowMinCut(
  i: AssignItem & { min_cut_meters?: string | null },
): boolean {
  if (i.sale_measure !== "meter" || !i.min_cut_meters) return false;
  const rest = missingOf(i);
  return rest > 1e-9 && rest < Number(i.min_cut_meters) - 1e-9;
}

/** Amount a line still needs (pieces or meters), never below 0. */
export const missingOf = (i: AssignItem) =>
  Math.max(0, requiredOf(i) - Number(i.assigned));

/**
 * Quantity sent for a scanned fixed barcode (TEC-1048): what the user set on
 * the stepper, or, while the stepper is untouched, what the line still needs
 * capped at what the barcode holds. A fixed barcode line (e.g. 10 squeegees)
 * is then prepared with one scan instead of ten.
 */
export function fixedAssignQuantity(
  item: AssignItem,
  chosen: number,
  touched: boolean,
  onHand?: number | null,
): number {
  if (touched) return Math.max(1, Math.round(chosen));
  const need = Math.max(1, Math.ceil(missingOf(item)));
  return onHand != null && onHand > 0 ? Math.min(need, onHand) : need;
}

type FixedSuggestion = {
  unit_uuid: string;
  unit_kind: string;
  quantity: number | null;
};

/**
 * Fixed barcode suggestions of a line as apply input (TEC-1048: the product
 * code of a fixed-barcode product was scanned). With a cap (the stepper was
 * set) the quantities are cut down to it; serial and roll rows are dropped.
 */
export function fixedSuggestionUnits(
  suggestions: readonly FixedSuggestion[],
  cap: number | null,
): { unit_uuid: string; quantity: number }[] {
  let left = cap ?? Number.POSITIVE_INFINITY;
  const out: { unit_uuid: string; quantity: number }[] = [];
  for (const s of suggestions) {
    if (s.unit_kind !== "fixed" || !s.quantity || left <= 0) continue;
    const quantity = Math.min(s.quantity, left);
    out.push({ unit_uuid: s.unit_uuid, quantity });
    left -= quantity;
  }
  return out;
}

/** A roll scanned with the meters box empty assigns the whole roll: asked first. */
export function asksWholeRoll(item: AssignItem, meters: string): boolean {
  return isRollItem(item) && meters.trim() === "";
}

/**
 * TEC-1228: package count (1–9999), desi and weight typed in the ship sheet
 * (comma or dot, at most two decimals; empty = not given). null when one is
 * invalid.
 */
export function parseShipment(
  packages: string,
  desi: string,
  weight: string,
): { package_count?: number; desi?: number; weight_kg?: number } | null {
  const out: { package_count?: number; desi?: number; weight_kg?: number } = {};
  const p = packages.trim();
  if (p) {
    if (!/^\d{1,4}$/.test(p) || Number(p) < 1) return null;
    out.package_count = Number(p);
  }
  for (const [key, raw] of [
    ["desi", desi],
    ["weight_kg", weight],
  ] as const) {
    const v = raw.trim().replace(",", ".");
    if (!v) continue;
    if (!/^\d+(\.\d{1,2})?$/.test(v)) return null;
    out[key] = Number(v);
  }
  return out;
}
