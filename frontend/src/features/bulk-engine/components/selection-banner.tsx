"use client";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

type SelectionBannerProps = {
  selectedCount: number;
  total: number;
  showSelectAll: boolean;
  allMatchingSelected: boolean;
  onSelectAllMatching: () => void;
  onClearSelection: () => void;
};

export function SelectionBanner({
  selectedCount,
  total,
  showSelectAll,
  allMatchingSelected,
  onSelectAllMatching,
  onClearSelection,
}: SelectionBannerProps) {
  const { t } = useLocale();

  if (selectedCount <= 0) return null;

  return (
    <div className="bg-muted/50 flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-sm">
      <span className="text-muted-foreground">
        {allMatchingSelected
          ? t("bulk.all_matching_selected", { total })
          : t("table.selected", { count: selectedCount })}
      </span>
      {showSelectAll ? (
        <Button
          type="button"
          size="sm"
          variant="link"
          className="h-auto p-0"
          onClick={onSelectAllMatching}
        >
          {t("bulk.select_all_matching", { total })}
        </Button>
      ) : null}
      {allMatchingSelected ? (
        <Button
          type="button"
          size="sm"
          variant="link"
          className="h-auto p-0"
          onClick={onClearSelection}
        >
          {t("bulk.clear_selection")}
        </Button>
      ) : null}
    </div>
  );
}
