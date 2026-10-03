/**
 * TEC-213: recognises a barcode, plate or VIN typed into the palette so the
 * matching result group moves to the top.
 */
export type QueryKind = "barcode" | "plate" | "vin";

/** Stock unit barcode: `PREFIX-00000001` (stock barcodes) or EAN/GTIN digits. */
const BARCODE = /^[A-Z][A-Z0-9]{1,11}-\d{4,}$|^\d{8,14}$/;
/** Turkish plate: province 01–81, 1–3 letters, 2–5 digits. */
const PLATE = /^(0[1-9]|[1-7]\d|8[01])\s*[A-ZÇĞİÖŞÜ]{1,3}\s*\d{2,5}$/;
/** VIN: 17 characters without I, O, Q. */
const VIN = /^[A-HJ-NPR-Z0-9]{17}$/;

export function detectQueryKind(raw: string): QueryKind | null {
  const q = raw.trim().toLocaleUpperCase("tr");
  if (!q) return null;
  if (VIN.test(q) && /\d/.test(q) && /[A-Z]/.test(q)) return "vin";
  if (PLATE.test(q)) return "plate";
  if (BARCODE.test(q)) return "barcode";
  return null;
}

/** Group spec that goes first for a recognised query kind. */
const FIRST_GROUP: Record<QueryKind, string> = {
  barcode: "stock_units",
  plate: "vehicles",
  vin: "vehicles",
};

/**
 * Moves the group matching the query kind to the top; the other groups
 * keep the server order (stable).
 */
export function prioritizeGroups<T extends { spec: string }>(
  groups: T[],
  kind: QueryKind | null,
): T[] {
  if (!kind) return groups;
  const first = FIRST_GROUP[kind];
  const top = groups.filter((g) => g.spec === first);
  if (top.length === 0) return groups;
  return [...top, ...groups.filter((g) => g.spec !== first)];
}
