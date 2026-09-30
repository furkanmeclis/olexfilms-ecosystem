"use client";

import type {
  BulkActionMeta,
  BulkResource,
  SelectionScope,
} from "@/features/bulk-engine/types";
import { resolveBulkActionsWithIcons } from "@/features/bulk-engine/lib/bulk-action-icons";
import { BulkActionMenu } from "@/features/bulk-engine/components/bulk-action-menu";
import { SelectionBanner } from "@/features/bulk-engine/components/selection-banner";
import type { ResourceCapabilities } from "@/features/io/types";

type ResourceBulkToolbarProps = {
  resource: BulkResource;
  bulkActions?: BulkActionMeta[];
  capabilities?: ResourceCapabilities;
  selection: SelectionScope;
  selectedCount: number;
  total: number;
  showSelectAllBanner: boolean;
  onSelectAllMatching: () => void;
  onClearSelection: () => void;
  onComplete?: () => void;
};

export function ResourceBulkToolbar({
  resource,
  bulkActions,
  capabilities,
  selection,
  selectedCount,
  total,
  showSelectAllBanner,
  onSelectAllMatching,
  onClearSelection,
  onComplete,
}: ResourceBulkToolbarProps) {
  const resolvedActions = resolveBulkActionsWithIcons(
    resource,
    bulkActions ?? [],
  );

  if (!capabilities?.bulk || resolvedActions.length === 0) return null;

  return (
    <div className="flex w-full flex-col gap-2">
      <SelectionBanner
        selectedCount={selectedCount}
        total={total}
        showSelectAll={showSelectAllBanner}
        allMatchingSelected={selection.mode === "all"}
        onSelectAllMatching={onSelectAllMatching}
        onClearSelection={onClearSelection}
      />
      <div className="flex flex-wrap items-center gap-2">
        <BulkActionMenu
          resource={resource}
          actions={resolvedActions}
          scope={selection}
          selectedCount={selectedCount}
          onComplete={onComplete}
        />
      </div>
    </div>
  );
}
