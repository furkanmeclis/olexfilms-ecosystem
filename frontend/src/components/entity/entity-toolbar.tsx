"use client";

import { Filter, RefreshCw } from "lucide-react";
import type { ReactNode } from "react";

import { EntityCreateButton } from "@/components/entity/entity-create-button";
import { ToolbarIconButton } from "@/components/tables/toolbar-icon-button";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

type EntityToolbarProps = {
  onRefresh?: () => void;
  refreshDisabled?: boolean;
  /**
   * Prefer EntityPage `actions` + EntityCreateButton for list create CTAs.
   * Kept for demos / legacy toolbar placement.
   */
  onCreate?: () => void;
  createLabel?: string;
  createPermission?: string | string[];
  onFiltersOpen?: () => void;
  filtersActive?: boolean;
  filtersLabel?: string;
  children?: ReactNode;
  className?: string;
};

/**
 * Extra toolbar actions for entity list pages (refresh / filters / optional create).
 * Wire into DataTable `toolbar` slot — search/columns/density stay in DataTableToolbar.
 */
export function EntityToolbar({
  onRefresh,
  refreshDisabled,
  onCreate,
  createLabel,
  createPermission,
  onFiltersOpen,
  filtersActive,
  filtersLabel,
  children,
  className,
}: EntityToolbarProps) {
  const { t } = useLocale();

  return (
    <div className={cn("flex flex-wrap items-center gap-2", className)}>
      {onRefresh ? (
        <ToolbarIconButton
          label={t("entity.refresh")}
          onClick={onRefresh}
          disabled={refreshDisabled}
        >
          <RefreshCw className="size-4" />
        </ToolbarIconButton>
      ) : null}

      {onFiltersOpen ? (
        <ToolbarIconButton
          label={filtersLabel ?? t("entity.filters")}
          onClick={onFiltersOpen}
          variant={filtersActive ? "secondary" : "outline"}
        >
          <Filter className="size-4" />
        </ToolbarIconButton>
      ) : null}

      {children}

      {onCreate ? (
        <EntityCreateButton
          onClick={onCreate}
          label={createLabel}
          permission={createPermission}
        />
      ) : null}
    </div>
  );
}
