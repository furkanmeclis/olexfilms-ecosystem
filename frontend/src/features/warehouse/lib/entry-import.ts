import {
  importPlan,
  type ImportConfirmBuilder,
} from "@/features/io/lib/import-summary";

/** Confirmation copy for a barcode import bound to one draft entry. */
export const entryImportConfirmCopy: ImportConfirmBuilder = ({
  summary,
  t,
}) => {
  const plan = importPlan(summary);
  return {
    sentence: t("warehouse.entry_import.confirm_sentence", {
      linked: plan.update,
      created: plan.create,
      placed: summary?.counts?.placed ?? 0,
      skipped: plan.skipped,
    }),
    action: t("warehouse.entry_import.confirm_action", {
      count: plan.importable,
      skipped: plan.skipped,
    }),
  };
};
