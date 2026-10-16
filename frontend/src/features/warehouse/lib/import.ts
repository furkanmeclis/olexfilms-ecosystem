import type { ImportConfirmBuilder } from "@/features/io/lib/import-summary";

/** Preview counts are unique nodes, while exists counts skipped file rows. */
export const warehouseLocationConfirmCopy: ImportConfirmBuilder = ({
  summary,
  t,
}) => ({
  sentence: t("warehouse.import.confirm", {
    rooms: summary?.counts?.rooms ?? 0,
    aisles: summary?.counts?.aisles ?? 0,
    shelves: summary?.counts?.shelves ?? 0,
    bins: summary?.counts?.bins ?? 0,
    existing: summary?.counts?.exists ?? 0,
  }),
  action: t("warehouse.import.confirm_action"),
});
