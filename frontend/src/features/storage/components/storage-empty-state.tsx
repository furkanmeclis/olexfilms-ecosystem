"use client";

import { EmptyState } from "@/components/common/empty-state";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

export function StorageEmptyState({
  view,
  onUpload,
  canWrite,
}: {
  view: string;
  onUpload?: () => void;
  canWrite?: boolean;
}) {
  const { t } = useLocale();
  const title =
    view === "trash"
      ? t("storage.empty_trash")
      : view === "starred"
        ? t("storage.empty_starred")
        : t("storage.empty_title");
  return (
    <EmptyState
      title={title}
      description={view === "all" ? t("storage.empty_description") : undefined}
      action={
        canWrite && view === "all" && onUpload ? (
          <Button type="button" onClick={onUpload}>
            {t("storage.upload")}
          </Button>
        ) : null
      }
    />
  );
}
