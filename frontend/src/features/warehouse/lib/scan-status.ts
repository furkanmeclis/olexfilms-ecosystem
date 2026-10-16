import type { WarehouseScanResult } from "@/features/warehouse/services/warehouse.service";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Translate a constrained scan result with the actual matched entity. */
export function scanStatusMessage(
  result: WarehouseScanResult,
  expect: string,
  t: Translate,
): string {
  switch (result.status) {
    case "type_mismatch":
      return t("warehouse.scan_status.type_mismatch", {
        actual: t(`warehouse.scan_status.type.${result.type}`),
        expected: t(`warehouse.scan_status.type.${expect}`),
      });
    case "not_bin":
      return t("warehouse.scan_status.not_bin");
    case "warehouse_mismatch":
      return t("warehouse.scan_status.warehouse_mismatch", {
        warehouse:
          result.location?.path[0]?.name ??
          result.unit?.location?.path[0]?.name ??
          "?",
      });
    case "void":
      return t("warehouse.scan_status.void");
    default:
      return t("warehouse.scan_status.unknown");
  }
}
