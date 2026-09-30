"use client";

import { DeleteDialog } from "@/components/dialogs/delete-dialog";
import { useLocale } from "@/providers/locale-provider";

type EntityDeleteDialogProps = {
  open: boolean;
  entityLabel: string;
  softDelete?: boolean;
  isPending?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

/**
 * Confirmation dialog for entity soft/hard delete.
 */
export function EntityDeleteDialog({
  open,
  entityLabel,
  softDelete = true,
  isPending,
  onConfirm,
  onCancel,
}: EntityDeleteDialogProps) {
  const { t } = useLocale();

  return (
    <DeleteDialog
      open={open}
      title={t("entity.delete_title", { name: entityLabel })}
      description={
        softDelete
          ? t("entity.delete_soft_description", { name: entityLabel })
          : t("entity.delete_description", { name: entityLabel })
      }
      isPending={isPending}
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  );
}
