import { unitBarcode } from "@/features/warehouse/lib/scan";
import type {
  WarehouseOrderPlacement,
  WarehouseOrderPlacementUnit,
} from "@/features/warehouse/services/warehouse.service";

/** The order fields the shelving panel looks at. */
export type PlacementOrder = {
  status: string;
  role: string;
  buyer: { type: string };
};

/**
 * Whether an order shows the "place on shelf" panel (TEC-765): only the
 * buyer side of a received order, only for a center or distributor buyer
 * (K12: a dealer has no locations) and only with warehouse.write.
 */
export function showOrderPlacement(
  order: PlacementOrder,
  orgType: string | null | undefined,
  canWrite: boolean,
): boolean {
  const fullWarehouse = orgType === "center" || orgType === "distributor";
  return (
    canWrite &&
    fullWarehouse &&
    order.role === "buyer" &&
    order.buyer.type === orgType &&
    order.status === "received"
  );
}

/** Serial units of the order still waiting for a shelf. */
export function unplacedUnits(
  p: WarehouseOrderPlacement,
): WarehouseOrderPlacementUnit[] {
  return p.units.filter((u) => u.unplaced);
}

export type UnitScan =
  | { kind: "select"; unit: WarehouseOrderPlacementUnit }
  | { kind: "already_selected"; unit: WarehouseOrderPlacementUnit }
  | { kind: "placed"; barcode: string }
  | { kind: "fixed"; barcode: string }
  | { kind: "not_on_order"; barcode: string };

/**
 * What a scanned unit code means for the order: a waiting unit to select,
 * one already shelved, a fixed barcode (never placed here) or a code that
 * is not on the order. Barcodes compare case-insensitively like the server.
 */
export function classifyUnitScan(
  p: WarehouseOrderPlacement,
  code: string,
  selected: readonly string[],
): UnitScan {
  const barcode = unitBarcode(code);
  const key = barcode.toUpperCase();
  const unit = p.units.find((u) => u.barcode.toUpperCase() === key);
  if (unit) {
    if (!unit.unplaced) return { kind: "placed", barcode: unit.barcode };
    return selected.includes(unit.unit_uuid)
      ? { kind: "already_selected", unit }
      : { kind: "select", unit };
  }
  const fixed = p.fixed.find((u) => u.barcode.toUpperCase() === key);
  if (fixed) return { kind: "fixed", barcode: fixed.barcode };
  return { kind: "not_on_order", barcode };
}

/** Barcodes of the selected units, in the order list's order. */
export function selectedBarcodes(
  p: WarehouseOrderPlacement,
  selected: readonly string[],
): string[] {
  return unplacedUnits(p)
    .filter((u) => selected.includes(u.unit_uuid))
    .map((u) => u.barcode);
}
