import type {
  WarehouseTransfer,
  WarehouseTransferStatus,
} from "@/features/warehouse/services/warehouse.service";

/** Warehouse transfer states (TEC-205): draft → in_transit → completed. */
export const TRANSFER_STATUSES: WarehouseTransferStatus[] = [
  "draft",
  "in_transit",
  "completed",
  "cancelled",
];

export function transferStatusTone(
  status: WarehouseTransferStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "completed":
      return "success";
    case "draft":
    case "in_transit":
      return "warning";
    case "cancelled":
      return "danger";
    default:
      return "default";
  }
}

/** Lines still waiting for a target location. */
export function untargetedLines(t: WarehouseTransfer) {
  return (t.lines ?? []).filter((l) => !l.target_location);
}

/** Lines may be added or removed only while the transfer is a draft. */
export function canEditLines(t: WarehouseTransfer): boolean {
  return t.status === "draft";
}

/** Target locations can be set on a draft or in-transit transfer. */
export function canPlace(t: WarehouseTransfer): boolean {
  return (
    (t.status === "draft" || t.status === "in_transit") &&
    (t.lines ?? []).length > 0
  );
}

/** Ship needs a draft with at least one line. */
export function canShip(t: WarehouseTransfer): boolean {
  return t.status === "draft" && (t.lines ?? []).length > 0;
}

/**
 * Receiving without a scanned location needs an in-transit transfer whose
 * every line has a target, its own or the transfer's default location;
 * otherwise the backend answers WAREHOUSE_TRANSFER_UNPLACED and the
 * receiver scans a location for the rest.
 */
export function canCompleteWithoutLocation(t: WarehouseTransfer): boolean {
  return (
    t.status === "in_transit" &&
    (Boolean(t.to_location) || untargetedLines(t).length === 0)
  );
}

export function canCancel(t: WarehouseTransfer): boolean {
  return t.status === "draft" || t.status === "in_transit";
}
