"use client";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { useLocale } from "@/providers/locale-provider";

type DeleteDialogProps = {
  open: boolean;
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  isPending?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

export function DeleteDialog({
  open,
  title,
  description,
  confirmLabel,
  cancelLabel,
  isPending,
  onConfirm,
  onCancel,
}: DeleteDialogProps) {
  const { t } = useLocale();

  return (
    <ConfirmDialog
      open={open}
      title={title}
      description={description}
      confirmLabel={confirmLabel ?? t("common.delete")}
      cancelLabel={cancelLabel}
      variant="destructive"
      isPending={isPending}
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  );
}
