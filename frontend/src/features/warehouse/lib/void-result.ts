// TEC-774: a reserved label already on an open transfer or an order line is
// not voided; tell the user which barcodes and why instead of only counting
// them as rejected.

type VoidItem = { barcode: string; voided: boolean; reason?: string };

const HELD_REASONS = ["in_transfer", "in_order"] as const;

export function voidHeldWarnings(
  items: VoidItem[],
  t: (key: string, values?: Record<string, string | number>) => string,
): string[] {
  return HELD_REASONS.flatMap((reason) => {
    const barcodes = items
      .filter((item) => !item.voided && item.reason === reason)
      .map((item) => item.barcode);
    if (barcodes.length === 0) return [];
    return [
      t(`warehouse.barcodes.void_held_${reason}`, {
        n: barcodes.length,
        barcodes: barcodes.join(", "),
      }),
    ];
  });
}
